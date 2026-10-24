package waf

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"go.miragespace.co/specter/config"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/ProtonMail/go-crypto/openpgp/packet"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func writeFixtureRules(t *testing.T, w *zip.Writer) {
	t.Helper()
	for _, file := range []struct{ name, contents string }{
		{"crs-setup.conf.example", "SecDefaultAction \"phase:1,pass,log\"\nSecDefaultAction \"phase:2,pass,log\"\n"},
		{"rules/REQUEST-901-INITIALIZATION.conf", `SecRule ARGS "@contains <script>" "id:941100,phase:1,t:none,deny,status:403,log,msg:'Test detection',tag:'OWASP_CRS',tag:'attack-xss'"`},
		{"rules/REQUEST-949-BLOCKING-EVALUATION.conf", "# The updater fixture uses a directly disruptive detection rule.\n"},
	} {
		output, err := w.Create("coreruleset-4.30.0/" + file.name)
		require.NoError(t, err)
		_, err = output.Write([]byte(file.contents))
		require.NoError(t, err)
	}
}

func signedFixture(t *testing.T) ([]byte, []byte, openpgp.EntityList) {
	t.Helper()
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	writeFixtureRules(t, w)
	require.NoError(t, w.Close())
	signer, err := openpgp.NewEntity("WAF tests", "", "waf@example.test", &packet.Config{Algorithm: packet.PubKeyAlgoEdDSA})
	require.NoError(t, err)
	var signature bytes.Buffer
	require.NoError(t, openpgp.ArmoredDetachSign(&signature, []*openpgp.Entity{signer}, bytes.NewReader(archive.Bytes()), nil))
	return archive.Bytes(), signature.Bytes(), openpgp.EntityList{signer}
}

func TestSignedReleaseVerification(t *testing.T) {
	archive, signature, keys := signedFixture(t)
	release, err := verifyRelease("4.30.0", archive, signature, keys)
	require.NoError(t, err)
	_, err = fs.ReadFile(release.rules, "rules/REQUEST-901-INITIALIZATION.conf")
	require.NoError(t, err)

	tampered := bytes.Clone(archive)
	tampered[len(tampered)/2] ^= 1
	_, err = verifyRelease("4.30.0", tampered, signature, keys)
	require.Error(t, err, "a signed release must not accept changed bytes")
	_, err = verifyRelease("4.30.1", archive, signature, keys)
	require.Error(t, err, "a signature must not permit relabeling an older archive")
}

func TestRejectedReleaseIsNotRecompiled(t *testing.T) {
	manager := &Manager{logger: zap.NewNop()}
	engine, firstErr := manager.compileRelease(&ruleRelease{version: "4.30.0", rules: fstest.MapFS{}})
	require.Error(t, firstErr)
	require.Nil(t, engine)

	rules, err := bundledRules()
	require.NoError(t, err)
	engine, err = manager.compileRelease(&ruleRelease{version: "4.30.0", rules: rules})
	if engine != nil {
		_ = engine.Close()
	}
	require.EqualError(t, err, firstErr.Error())
	require.Nil(t, engine)
	engine, err = manager.compileRelease(&ruleRelease{version: bundledVersion, rules: rules})
	require.NoError(t, err)
	require.NoError(t, engine.Close())
}

func TestRuleArchiveRejectsUnsafeResources(t *testing.T) {
	for _, test := range []struct {
		name string
		mode fs.FileMode
		size int
	}{
		{name: "../outside"},
		{name: "/outside"},
		{name: "coreruleset-4.30.0/../outside"},
		{name: "coreruleset-4.30.0/rules\\outside"},
		{name: "coreruleset-4.30.0/rules/link", mode: fs.ModeSymlink},
		{name: "coreruleset-4.30.0/rules/oversized.conf", size: maxRuleFileBytes + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var data bytes.Buffer
			w := zip.NewWriter(&data)
			writeFixtureRules(t, w)
			header := &zip.FileHeader{Name: test.name, Method: zip.Deflate}
			header.SetMode(test.mode | 0600)
			f, err := w.CreateHeader(header)
			require.NoError(t, err)
			_, err = f.Write(make([]byte, test.size))
			require.NoError(t, err)
			require.NoError(t, w.Close())
			_, err = openRulesArchive("4.30.0", data.Bytes())
			require.Error(t, err)
		})
	}
}

func testRuleSource(t *testing.T, brokenSignature *atomic.Bool) *ruleSource {
	t.Helper()
	archive, signature, keys := signedFixture(t)
	source := &ruleSource{keys: keys}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			_, _ = w.Write([]byte(`[{"tag_name":"nightly"},{"tag_name":"v5.0.0"},{"tag_name":"v4.99.0","prerelease":true},{"tag_name":"v4.98.0","draft":true},{"tag_name":"v4.9.0"},{"tag_name":"v4.30.0"},{"tag_name":"v4.25.2"},{"tag_name":"v4.25.1"}]`))
		case "/download/v4.30.0/coreruleset-4.30.0-minimal.zip":
			_, _ = w.Write(archive)
		case "/download/v4.30.0/coreruleset-4.30.0-minimal.zip.asc":
			if brokenSignature != nil && brokenSignature.Load() {
				_, _ = w.Write([]byte("invalid signature"))
			} else {
				_, _ = w.Write(signature)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	source.client, source.releasesURL, source.downloadURL = server.Client(), server.URL+"/releases", server.URL+"/download"
	return source
}

func TestRuleSourceChannelSelection(t *testing.T) {
	source := testRuleSource(t, nil)
	for channel, expected := range map[protocol.WAFSettings_Channel]string{protocol.WAFSettings_LTS: "4.25.2", protocol.WAFSettings_STABLE: "4.30.0"} {
		version, err := source.latest(t.Context(), channel)
		require.NoError(t, err)
		require.Equal(t, expected, version)
	}
	_, err := source.get(t.Context(), source.releasesURL, 10)
	require.ErrorContains(t, err, "size limit")
}

func putManagerSettings(t *testing.T, store *config.Store, settings *protocol.WAFSettings) string {
	t.Helper()
	current, _, err := store.Get(ConfigName)
	require.NoError(t, err)
	data, err := MarshalJSON(settings)
	require.NoError(t, err)
	record, err := store.Put(ConfigName, ConfigSchema, data, current.Token())
	require.NoError(t, err)
	return record.Token()
}

func testManager(t *testing.T) *Manager {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "config"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	manager, err := NewManager(store, t.TempDir(), zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	return manager
}

func managerAttackStatus(manager *Manager) int {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := manager.Wrap(next, func(*http.Request) (string, error) { return "app.example", nil })
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "https://app.example/?q=%3Cscript%3Ealert(1)%3C/script%3E", nil)
	handler.ServeHTTP(w, r)
	return w.Code
}

func TestManagerUpdates(t *testing.T) {
	manager := testManager(t)
	settings := DefaultSettings()
	settings.Enabled = true
	token := putManagerSettings(t, manager.store, settings)
	manager.reloadSettings()

	var broken atomic.Bool
	manager.source = testRuleSource(t, &broken)
	settings.Channel = protocol.WAFSettings_STABLE
	token = putManagerSettings(t, manager.store, settings)
	manager.reloadSettings()
	broken.Store(true)
	release, replacement, err := manager.prepareUpdate(t.Context(), settings, bundledVersion)
	require.Error(t, err)
	manager.finishUpdate(t.Context(), token, release, replacement, err)
	require.Equal(t, bundledVersion, manager.Status().ActiveVersion)
	require.NotEmpty(t, manager.Status().LastError)
	require.Equal(t, http.StatusForbidden, managerAttackStatus(manager))

	broken.Store(false)
	release, replacement, err = manager.prepareUpdate(t.Context(), settings, bundledVersion)
	require.NoError(t, err)
	manager.finishUpdate(t.Context(), token, release, replacement, nil)
	require.Equal(t, "4.30.0", manager.Status().ActiveVersion)
	require.Equal(t, bundledVersion, manager.Status().PreviousVersion)
	require.Empty(t, manager.Status().LastError)
	require.Equal(t, http.StatusForbidden, managerAttackStatus(manager))

	settings.Channel = protocol.WAFSettings_LTS
	token = putManagerSettings(t, manager.store, settings)
	manager.reloadSettings()
	require.NotEqual(t, token, manager.Status().AppliedVersion, "cross-channel changes stay pending until their rules activate")
	release, replacement, err = manager.prepareUpdate(t.Context(), settings, "4.30.0")
	require.NoError(t, err)
	manager.finishUpdate(t.Context(), token, release, replacement, nil)
	require.Equal(t, bundledVersion, manager.Status().ActiveVersion, "a channel change can intentionally lower the version")
	require.Equal(t, "4.30.0", manager.Status().PreviousVersion)
	require.Equal(t, token, manager.Status().AppliedVersion)
}

func TestManagerDiscardsStaleUpdate(t *testing.T) {
	manager := testManager(t)
	manager.source = testRuleSource(t, nil)
	settings := DefaultSettings()
	settings.Enabled = true
	putManagerSettings(t, manager.store, settings)
	manager.reloadSettings()
	settings.PinnedVersion = "4.30.0"
	token := putManagerSettings(t, manager.store, settings)
	manager.reloadSettings()
	require.NotEqual(t, token, manager.Status().AppliedVersion)
	release, replacement, err := manager.prepareUpdate(t.Context(), settings, bundledVersion)
	require.NoError(t, err)
	settings.Enabled = false
	putManagerSettings(t, manager.store, settings)
	// Do not poll first: activation must see the saved change itself.
	manager.finishUpdate(t.Context(), token, release, replacement, nil)
	require.Equal(t, bundledVersion, manager.Status().ActiveVersion)
	manager.reloadSettings()
	require.False(t, manager.Status().Enabled)
}

func TestManagerOfflineCacheRecovery(t *testing.T) {
	store, err := config.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	dir := t.TempDir()
	archive, signature, keys := signedFixture(t)
	release, err := verifyRelease("4.30.0", archive, signature, keys)
	require.NoError(t, err)
	require.NoError(t, saveRelease(dir, release))
	require.NoError(t, saveCacheState(dir, cacheState{Active: "4.30.0", Previous: bundledVersion}))
	settings := DefaultSettings()
	settings.Enabled, settings.Channel = true, protocol.WAFSettings_STABLE
	putManagerSettings(t, store, settings)
	// Both restarts use the same trust key and no HTTP client. Only the cached
	// archive bytes change, so recovery cannot pass because of a key mismatch.
	restart := func() *Manager {
		manager := &Manager{
			store: store, dir: dir, logger: zap.NewNop(),
			source: &ruleSource{keys: keys}, wake: make(chan struct{}, 1),
		}
		manager.active.Store(&activeProtection{settings: DefaultSettings()})
		manager.reloadSettings()
		t.Cleanup(func() { require.NoError(t, manager.Close()) })
		return manager
	}
	restarted := restart()
	require.True(t, restarted.Status().Ready)
	require.Equal(t, "4.30.0", restarted.Status().ActiveVersion)
	require.Empty(t, restarted.Status().LastError)
	require.NoError(t, restarted.Close())

	require.NoError(t, os.WriteFile(filepath.Join(dir, "4.30.0.zip"), []byte("corrupt"), 0600))
	recovered := restart()
	require.True(t, recovered.Status().Ready)
	require.Equal(t, bundledVersion, recovered.Status().ActiveVersion)
	require.Contains(t, recovered.Status().LastError, "verify CRS 4.30.0 signature")
}

func TestManagerUnsupportedColdConfiguration(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "config"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	record, err := config.NewRecord(ConfigName, ConfigSchema+1, 1, []byte(`{}`))
	require.NoError(t, err)
	require.NoError(t, store.Merge([]config.Record{record}))
	manager, err := NewManager(store, filepath.Join(t.TempDir(), "waf"), zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	require.True(t, manager.Status().Configured)
	require.False(t, manager.Status().Ready)
	require.True(t, strings.Contains(manager.Status().LastError, "unsupported"))
	w := httptest.NewRecorder()
	manager.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unconfigured protection must not forward") }), nil).
		ServeHTTP(w, httptest.NewRequest(http.MethodGet, "https://app.example/", nil))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}
