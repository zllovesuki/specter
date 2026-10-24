package waf

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.miragespace.co/specter/config"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/util"

	"go.uber.org/zap"
)

type activeProtection struct {
	settings    *protocol.WAFSettings
	engine      *Engine
	unavailable bool
}

// Manager consumes shared operator intent and owns this server's rules cache.
// Request admission reads an immutable snapshot; update I/O does not hold up
// requests using the working generation.
type Manager struct {
	store    *config.Store
	dir      string
	logger   *zap.Logger
	resolver func(context.Context, string) (*protocol.WAFPolicy, error)
	source   *ruleSource
	wake     chan struct{}
	active   atomic.Pointer[activeProtection]
	jobs     sync.WaitGroup

	compileMu sync.Mutex
	rejected  map[string]error

	mu        sync.Mutex
	status    Status
	token     string
	cache     cacheState
	configErr error
	updateErr error
	nextCheck time.Time
	closed    bool
	cancel    context.CancelFunc
}

// NewManager reads persisted configuration before application handlers can run.
// Invalid configuration remains inspectable through the operator API; a cold
// consumer rejects application requests instead of interpreting it as Off.
// The resolver supplies validated hostname policies.
func NewManager(store *config.Store, dir string, logger *zap.Logger, resolver func(context.Context, string) (*protocol.WAFPolicy, error)) (*Manager, error) {
	if store == nil || strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("WAF needs shared configuration and a cache directory")
	}
	if err := util.MkdirAllDurable(dir, 0700); err != nil {
		return nil, err
	}
	source, err := officialRuleSource()
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	m := &Manager{store: store, dir: dir, logger: logger, resolver: resolver, source: source, wake: make(chan struct{}, 1)}
	m.active.Store(&activeProtection{settings: DefaultSettings()})
	m.reloadSettings()
	return m, nil
}

// Run polls the existing local configuration replica and checks upstream daily.
// A separate bounded update job leaves configuration polling responsive.
func (m *Manager) Run(ctx context.Context) {
	m.mu.Lock()
	if m.cancel != nil || m.closed {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()
	defer cancel()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		m.reloadSettings()
		m.beginUpdate(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-m.wake:
		}
	}
}

func (m *Manager) Wake() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	if m.cancel != nil {
		m.cancel()
	}
	active := *m.active.Load()
	active.unavailable = true
	m.active.Store(&active)
	m.mu.Unlock()
	m.jobs.Wait()
	if active.engine != nil {
		return active.engine.Close()
	}
	return nil
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.active.Load()
	status := m.status
	status.Enabled = active.settings.Enabled
	status.Ready = !active.unavailable && (!active.settings.Enabled || active.engine != nil)
	status.ActiveVersion, status.PreviousVersion = m.cache.Active, m.cache.Previous
	status.EngineVersion = EngineVersion
	status.Channel, status.PinnedVersion = active.settings.Channel.String(), active.settings.PinnedVersion
	if err := errors.Join(m.configErr, m.updateErr); err != nil {
		status.LastError = err.Error()
	}
	return status
}

func (m *Manager) Wrap(next http.Handler, hostname func(*http.Request) (string, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		active := m.active.Load()
		if active.unavailable || (active.settings.Enabled && active.engine == nil) {
			http.Error(w, "Web protection configuration is unavailable", http.StatusServiceUnavailable)
			return
		}
		if !active.settings.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		host, err := hostname(r)
		if err != nil {
			http.Error(w, "Invalid application hostname", http.StatusBadRequest)
			return
		}
		policy := DefaultPolicy()
		if m.resolver != nil {
			policy, err = m.resolver(r.Context(), host)
		}
		if err != nil {
			http.Error(w, "Web protection policy is unavailable", http.StatusServiceUnavailable)
			return
		}
		path := r.URL.EscapedPath()
		if path == "" {
			path = "/"
		}
		active.engine.ServeHTTP(w, r, next, host, EffectiveMode(policy, active.settings.DefaultMode), DisabledRules(policy, path))
	})
}

func (m *Manager) reloadSettings() {
	record, found, err := m.store.Get(ConfigName)
	settings := DefaultSettings()
	if err == nil && found {
		settings, err = DecodeSettings(record)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	active := *m.active.Load()
	if err != nil {
		m.status.Configured, m.configErr = found, err
		if m.token == "" {
			active.unavailable = true
			m.active.Store(&active)
		}
		return
	}
	if m.token == record.Token() && m.status.Configured == found && m.configErr == nil {
		return
	}
	m.token, m.status.Configured = record.Token(), found
	m.configErr = nil
	if settings.Enabled && active.engine == nil {
		active.engine, m.cache, m.updateErr = m.restoreRules(settings)
	}
	if !settings.Enabled || (active.engine != nil && settingsSatisfied(settings, m.cache.Active)) {
		m.status.AppliedVersion = m.token
	}
	active.settings, active.unavailable = settings, settings.Enabled && active.engine == nil
	m.active.Store(&active)
	m.nextCheck = time.Time{}
}

func (m *Manager) loadRelease(version string) (*ruleRelease, error) {
	release, err := m.source.cached(m.dir, version)
	if err == nil || version != bundledVersion {
		return release, err
	}
	rules, err := bundledRules()
	if err != nil {
		return nil, err
	}
	return &ruleRelease{version: bundledVersion, rules: rules}, nil
}

func (m *Manager) restoreRules(settings *protocol.WAFSettings) (*Engine, cacheState, error) {
	stored, stateErr := readCacheState(m.dir)
	versions := []string{settings.PinnedVersion, stored.Active, stored.Previous, bundledVersion}
	seen := make(map[string]bool)
	var failures []error
	if stateErr != nil {
		failures = append(failures, stateErr)
	}
	for _, version := range versions {
		if version == "" || seen[version] {
			continue
		}
		seen[version] = true
		release, err := m.loadRelease(version)
		if err == nil {
			var engine *Engine
			engine, err = m.compileRelease(release)
			if err == nil {
				state := cacheState{Active: version}
				if version == stored.Active {
					state.Previous = stored.Previous
				} else if version == settings.PinnedVersion && stored.Active != "" {
					state.Previous = stored.Active
				}
				if state != stored {
					if err := saveCacheState(m.dir, state); err != nil {
						failures = append(failures, err)
					}
				}
				return engine, state, errors.Join(failures...)
			}
		}
		failures = append(failures, fmt.Errorf("load CRS %s: %w", version, err))
	}
	return nil, cacheState{}, errors.Join(failures...)
}

func (m *Manager) beginUpdate(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.active.Load()
	if m.closed || m.status.Updating || m.configErr != nil || !active.settings.Enabled || time.Now().Before(m.nextCheck) {
		return
	}
	settings, token, current := active.settings, m.token, m.cache.Active
	m.status.Updating = true
	checkedAt := time.Now().UTC()
	m.status.LastCheck = &checkedAt
	m.nextCheck = time.Now().Add(24*time.Hour + time.Duration(rand.Int64N(int64(2*time.Hour))) - time.Hour)
	m.jobs.Add(1)
	go func() {
		defer m.jobs.Done()
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		release, engine, err := m.prepareUpdate(ctx, settings, current)
		m.finishUpdate(ctx, token, release, engine, err)
	}()
}

func (m *Manager) prepareUpdate(ctx context.Context, settings *protocol.WAFSettings, current string) (*ruleRelease, *Engine, error) {
	version := settings.PinnedVersion
	if version == "" {
		var err error
		version, err = m.source.latest(ctx, settings.Channel)
		if err != nil {
			return nil, nil, err
		}
		// A channel switch can deliberately lower the release number. Within an
		// unchanged eligible line, stale discovery must not roll working rules back.
		if releaseInChannel(current, settings.Channel) && compareReleases(version, current) < 0 {
			return nil, nil, nil
		}
	}
	if version == current {
		return nil, nil, nil
	}
	release, err := m.loadRelease(version)
	if err != nil {
		release, err = m.source.download(ctx, version)
	}
	if err != nil {
		return nil, nil, err
	}
	engine, err := m.compileRelease(release)
	return release, engine, err
}

func (m *Manager) compileRelease(release *ruleRelease) (*Engine, error) {
	m.compileMu.Lock()
	defer m.compileMu.Unlock()
	if err := m.rejected[release.version]; err != nil {
		return nil, err
	}
	engine, err := NewEngine(release.rules, release.version, DefaultBodyLimit, m.logger)
	if err != nil {
		// Coraza 3.8.1 does not release memoizer ownership on constructor failure.
		// Signed release versions are immutable, so reject one only once per
		// process while continuing discovery and retries for download failures.
		err = fmt.Errorf("CRS %s compilation rejected until restart: %w", release.version, err)
		if m.rejected == nil {
			m.rejected = make(map[string]error)
		}
		m.rejected[release.version] = err
	}
	return engine, err
}

func (m *Manager) finishUpdate(ctx context.Context, token string, release *ruleRelease, engine *Engine, updateErr error) {
	var old *Engine
	defer func() {
		if engine != nil {
			_ = engine.Close()
		}
		if old != nil {
			_ = old.Close()
		}
	}()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.Updating = false
	if m.closed || errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	if ctx.Err() != nil {
		updateErr = ctx.Err()
	}
	// Re-read the store after all download/compilation work, including when the
	// ordinary poll has not observed a just-saved pin or disable yet.
	record, found, err := m.store.Get(ConfigName)
	if err == nil && (!found || record.Token() != token || m.token != token) {
		m.nextCheck = time.Time{}
		m.Wake()
		return
	}
	if err != nil {
		updateErr = err
	}
	active := *m.active.Load()
	if updateErr == nil && release != nil {
		state := cacheState{Active: release.version, Previous: m.cache.Active}
		if len(release.archive) != 0 {
			updateErr = saveRelease(m.dir, release)
		}
		if updateErr == nil {
			updateErr = saveCacheState(m.dir, state)
		}
		if updateErr == nil {
			old, active.engine, engine = active.engine, engine, nil
			active.unavailable = false
			m.cache = state
			m.active.Store(&active)
			pruneCache(m.dir, state)
		}
	}
	m.updateErr = updateErr
	if updateErr != nil {
		m.nextCheck = time.Now().Add(time.Hour)
		m.logger.Warn("WAF rules update failed; keeping working rules", zap.Error(updateErr))
		return
	}
	checkedAt := time.Now().UTC()
	m.status.LastSuccessfulCheck = &checkedAt
	if active.engine != nil && settingsSatisfied(active.settings, m.cache.Active) {
		m.status.AppliedVersion = token
	}
}

func settingsSatisfied(settings *protocol.WAFSettings, version string) bool {
	if settings.PinnedVersion != "" {
		return version == settings.PinnedVersion
	}
	return releaseInChannel(version, settings.Channel)
}
