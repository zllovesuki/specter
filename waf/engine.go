package waf

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.miragespace.co/specter/spec/protocol"

	corazaconf "github.com/corazawaf/coraza-coreruleset/coraza/v3"
	"github.com/corazawaf/coraza/v3"
	"github.com/corazawaf/coraza/v3/experimental"
	"github.com/corazawaf/coraza/v3/types"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

const (
	EngineVersion = "3.8.1"
	// DefaultBodyLimit bounds inspection, not the size of a forwarded upload.
	DefaultBodyLimit = 16 << 10
	maxVariants      = 32
	maxInspections   = 32
	bodyReadTimeout  = 15 * time.Second
)

type RuleInfo struct {
	ID         int    `json:"id"`
	Category   string `json:"category"`
	Excludable bool   `json:"excludable"`
}

type engineVariant struct {
	waf  coraza.WAF
	used uint64
}

// Engine owns one immutable rules release and a bounded set of compiled policy
// variants. The cache is shared by requests with identical effective exceptions.
type Engine struct {
	rules     fs.FS
	version   string
	bodyLimit int
	config    string
	logger    *zap.Logger
	catalog   map[int]RuleInfo
	slots     chan struct{}

	mu       sync.Mutex
	variants map[string]engineVariant
	clock    uint64
	closed   bool
}

// NewEngine expects the official CRS archive layout: crs-setup.conf.example and
// rules/*.conf, including the data files referenced by those rules.
func NewEngine(rules fs.FS, version string, bodyLimit int, logger *zap.Logger) (*Engine, error) {
	if bodyLimit <= 0 || bodyLimit > 1<<20 {
		return nil, errors.New("WAF inspection limit must be between 1 byte and 1 MiB")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	config, err := fs.ReadFile(corazaconf.FS, "@coraza.conf-recommended")
	if err != nil {
		return nil, fmt.Errorf("load Coraza configuration: %w", err)
	}
	e := &Engine{
		rules: rules, version: version, bodyLimit: bodyLimit,
		config: string(config), logger: logger,
		catalog: make(map[int]RuleInfo), slots: make(chan struct{}, maxInspections),
		variants: make(map[string]engineVariant),
	}
	for _, mode := range []protocol.WAFPolicy_Mode{protocol.WAFPolicy_BLOCK, protocol.WAFPolicy_MONITOR} {
		waf, err := e.compile(mode, nil, mode == protocol.WAFPolicy_BLOCK)
		if err != nil {
			e.Close()
			return nil, err
		}
		e.variants[mode.String()] = engineVariant{waf: waf}
	}
	if len(e.catalog) == 0 {
		e.Close()
		return nil, errors.New("CRS did not load any request detection rules")
	}
	return e, nil
}

func (e *Engine) compile(mode protocol.WAFPolicy_Mode, disabled []int, catalog bool) (coraza.WAF, error) {
	engineMode := "On"
	if mode == protocol.WAFPolicy_MONITOR {
		engineMode = "DetectionOnly"
	}
	cfg := coraza.NewWAFConfig().WithRootFS(e.rules).
		WithDirectives(e.config).
		WithDirectives(engineProfile).
		WithDirectivesFromFile("crs-setup.conf.example").
		WithDirectivesFromFile("rules/*.conf").
		WithDirectives(tunnelProfile).
		WithDirectives("SecRuleEngine " + engineMode).
		WithRequestBodyLimit(e.bodyLimit).
		WithRequestBodyInMemoryLimit(e.bodyLimit)
	for _, id := range disabled {
		cfg = cfg.WithDirectives("SecRuleRemoveById " + strconv.Itoa(id))
	}
	if catalog {
		cfg = experimental.WAFConfigWithRuleObserver(cfg, func(rule types.RuleMetadata) {
			if eligibleRule(rule) {
				e.catalog[rule.ID()] = RuleInfo{ID: rule.ID(), Category: ruleCategory(rule.Tags()), Excludable: true}
			}
		})
	}
	waf, err := coraza.NewWAF(cfg)
	if err != nil {
		return nil, fmt.Errorf("compile CRS %s: %w", e.version, err)
	}
	return waf, nil
}

func eligibleRule(rule types.RuleMetadata) bool {
	id := rule.ID()
	if id < 911000 || id >= 949000 || id == 911100 || id == 920420 || rule.SecMark() != "" {
		return false
	}
	if rule.Phase() != types.PhaseRequestHeaders && rule.Phase() != types.PhaseRequestBody {
		return false
	}
	tags := rule.Tags()
	return slices.Contains(tags, "OWASP_CRS") && slices.ContainsFunc(tags, func(tag string) bool {
		return strings.HasPrefix(tag, "attack-")
	})
}

func (e *Engine) variant(mode protocol.WAFPolicy_Mode, disabled []int) (coraza.WAF, error) {
	ids := make([]int, 0, len(disabled))
	for _, id := range disabled {
		if _, ok := e.catalog[id]; ok {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	var key strings.Builder
	key.WriteString(mode.String())
	for _, id := range ids {
		key.WriteByte(':')
		key.WriteString(strconv.Itoa(id))
	}
	k := key.String()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.clock++
	if cached, ok := e.variants[k]; ok {
		cached.used = e.clock
		e.variants[k] = cached
		return cached.waf, nil
	}
	waf, err := e.compile(mode, ids, false)
	if err != nil {
		return nil, err
	}
	if e.closed {
		// A manager may retire this generation after a request loaded its pointer.
		// Closing only releases memoized ownership; this WAF remains usable.
		closeWAF(waf)
		return waf, nil
	}
	if len(e.variants) >= maxVariants {
		var oldestKey string
		oldest := ^uint64(0)
		for key, entry := range e.variants {
			if key != protocol.WAFPolicy_BLOCK.String() && key != protocol.WAFPolicy_MONITOR.String() && entry.used < oldest {
				oldestKey, oldest = key, entry.used
			}
		}
		closeWAF(e.variants[oldestKey].waf)
		delete(e.variants, oldestKey)
	}
	e.variants[k] = engineVariant{waf: waf, used: e.clock}
	return waf, nil
}

func closeWAF(waf coraza.WAF) {
	if closer, ok := waf.(experimental.WAFCloser); ok {
		_ = closer.Close()
	}
}

// Close retires the cache without interrupting requests holding this generation.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.closed {
		e.closed = true
		for _, entry := range e.variants {
			closeWAF(entry.waf)
		}
	}
	return nil
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.Handler, hostname string, mode protocol.WAFPolicy_Mode, disabledIDs []int) {
	if mode == protocol.WAFPolicy_OFF {
		next.ServeHTTP(w, r)
		return
	}
	if mode != protocol.WAFPolicy_BLOCK && mode != protocol.WAFPolicy_MONITOR {
		http.Error(w, "Web protection configuration is unavailable", http.StatusServiceUnavailable)
		return
	}
	select {
	case e.slots <- struct{}{}:
	default:
		http.Error(w, "Web protection is busy; please retry", http.StatusServiceUnavailable)
		return
	}
	release := sync.OnceFunc(func() { <-e.slots })
	defer release()
	requestID := middleware.GetReqID(r.Context())
	prefix, interrupted, matched, err := e.inspect(w, r, mode, disabledIDs, requestID)
	if err != nil {
		e.logger.Warn("WAF inspection failed",
			zap.String("hostname", hostname), zap.String("request_id", requestID),
			zap.String("rules_version", e.version), zap.String("mode", mode.String()))
		http.Error(w, "Unable to inspect request", http.StatusBadRequest)
		return
	}
	if len(matched) > 0 {
		ids := make([]int, 0, len(matched))
		for _, rule := range matched {
			ids = append(ids, rule.ID)
		}
		e.logger.Info("WAF rule matches",
			zap.String("hostname", hostname), zap.String("request_id", requestID),
			zap.String("rules_version", e.version), zap.String("mode", mode.String()),
			zap.Ints("rule_ids", ids), zap.Bool("blocked", interrupted != nil))
	}
	if interrupted != nil {
		status := interrupted.Status
		if status < 400 || status > 599 {
			status = http.StatusForbidden
		}
		writeDenial(w, r, status, denial{Error: "request_blocked", RequestID: requestID, Version: e.version, Rules: matched})
		return
	}
	if len(prefix) > 0 {
		body := &replayBody{prefix: bytes.NewReader(prefix), source: r.Body, release: release}
		r.Body = body
		defer body.Close()
	} else {
		release()
	}
	next.ServeHTTP(w, r)
}

func (e *Engine) inspect(w http.ResponseWriter, r *http.Request, mode protocol.WAFPolicy_Mode, disabled []int, requestID string) ([]byte, *types.Interruption, []RuleInfo, error) {
	waf, err := e.variant(mode, disabled)
	if err != nil {
		return nil, nil, nil, err
	}
	tx := waf.NewTransactionWithID(requestID)
	defer func() {
		tx.ProcessLogging()
		_ = tx.Close()
	}()
	client, clientPort := splitAddress(r.RemoteAddr)
	server, serverPort := "", 0
	if addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		server, serverPort = splitAddress(addr.String())
	}
	tx.ProcessConnection(client, clientPort, server, serverPort)
	tx.ProcessURI(r.URL.RequestURI(), r.Method, r.Proto)
	tx.SetServerName(r.Host)
	tx.AddRequestHeader("Host", r.Host)
	for key, values := range r.Header {
		if strings.EqualFold(key, "Host") {
			continue
		}
		for _, value := range values {
			tx.AddRequestHeader(key, value)
		}
	}
	if r.ContentLength > 0 && r.Header.Get("Content-Length") == "" {
		tx.AddRequestHeader("Content-Length", strconv.FormatInt(r.ContentLength, 10))
	}
	for _, encoding := range r.TransferEncoding {
		tx.AddRequestHeader("Transfer-Encoding", encoding)
	}
	if interruption := tx.ProcessRequestHeaders(); interruption != nil {
		return nil, interruption, e.matches(tx), nil
	}
	var prefix []byte
	if r.Body != nil && r.Body != http.NoBody && inspectBody(r.Header.Get("Content-Type")) {
		controller := http.NewResponseController(w)
		if err := controller.SetReadDeadline(time.Now().Add(bodyReadTimeout)); err == nil {
			defer controller.SetReadDeadline(time.Time{})
		}
		interruption, _, err := tx.ReadRequestBodyFrom(r.Body)
		if err != nil || interruption != nil {
			return nil, interruption, e.matches(tx), err
		}
		reader, err := tx.RequestBodyReader()
		if err != nil {
			return nil, nil, nil, err
		}
		prefix, err = io.ReadAll(reader)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	interruption, err := tx.ProcessRequestBody()
	return prefix, interruption, e.matches(tx), err
}

func splitAddress(address string) (string, int) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return address, 0
	}
	n, _ := strconv.Atoi(port)
	return host, n
}

func inspectBody(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	if strings.HasPrefix(mediaType, "application/grpc") || strings.HasPrefix(mediaType, "application/connect+") || mediaType == "application/stream+json" {
		return false
	}
	return mediaType == "application/x-www-form-urlencoded" || mediaType == "multipart/form-data" ||
		mediaType == "application/json" || mediaType == "application/xml" || mediaType == "application/soap+xml" ||
		mediaType == "text/xml" || (strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"))
}

func (e *Engine) matches(tx types.Transaction) []RuleInfo {
	var matches []RuleInfo
	for _, match := range tx.MatchedRules() {
		if rule, ok := e.catalog[match.Rule().ID()]; ok {
			matches = append(matches, rule)
		}
	}
	slices.SortFunc(matches, func(a, b RuleInfo) int { return a.ID - b.ID })
	matches = slices.CompactFunc(matches, func(a, b RuleInfo) bool { return a.ID == b.ID })
	if len(matches) == 0 && tx.Interruption() != nil {
		matches = append(matches, RuleInfo{ID: tx.Interruption().RuleID, Category: "Request format"})
	}
	return matches
}

// replayBody owns only the copied prefix and the original request body. Coraza's
// transaction is already closed. Admission stays charged until that prefix is
// consumed or the body closes, including when the upstream never reads it.
type replayBody struct {
	mu      sync.Mutex
	prefix  *bytes.Reader
	source  io.ReadCloser
	release func()
	closed  bool
	close   sync.Once
	err     error
}

func (b *replayBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return 0, io.ErrClosedPipe
	}
	if b.prefix != nil {
		n, _ := b.prefix.Read(p)
		if b.prefix.Len() == 0 {
			b.prefix = nil
			b.release()
		}
		b.mu.Unlock()
		if n > 0 {
			return n, nil
		}
	} else {
		b.mu.Unlock()
	}
	return b.source.Read(p)
}

func (b *replayBody) Close() error {
	b.close.Do(func() {
		b.mu.Lock()
		b.closed = true
		b.prefix = nil
		b.release()
		b.mu.Unlock()
		b.err = b.source.Close()
	})
	return b.err
}

type denial struct {
	Error      string     `json:"error"`
	RequestID  string     `json:"request_id"`
	Version    string     `json:"rules_version"`
	Rules      []RuleInfo `json:"rules"`
	CanExclude bool       `json:"-"`
	Nonce      string     `json:"-"`
}

//go:embed blocked.html
var denialHTML string

var denialPage = template.Must(template.New("waf-denial").Parse(denialHTML))

func writeDenial(w http.ResponseWriter, r *http.Request, status int, response denial) {
	response.CanExclude = slices.ContainsFunc(response.Rules, func(rule RuleInfo) bool { return rule.Excludable })
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		response.Nonce = rand.Text()
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'nonce-"+response.Nonce+"'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_ = denialPage.Execute(w, response)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
