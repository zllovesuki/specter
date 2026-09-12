package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	clientcmd "go.miragespace.co/specter/cmd/client"

	"github.com/stretchr/testify/require"
)

type compatLog struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *compatLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

func (b *compatLog) messages() []map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	var messages []map[string]any
	for _, line := range strings.Split(b.Buffer.String(), "\n") {
		var msg map[string]any
		if json.Unmarshal([]byte(line), &msg) == nil {
			messages = append(messages, msg)
		}
	}
	return messages
}

func startCompatBinary(t *testing.T, binary string, signal os.Signal, args ...string) (*compatLog, func()) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	logs := &compatLog{}
	cmd.Stdout = io.Discard
	cmd.Stderr = logs
	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		cmd.Process.Signal(signal)
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(30 * time.Second):
			cmd.Process.Kill()
			<-done
			t.Error("compat binary required forced termination")
		}
	}
	t.Cleanup(stop)
	t.Cleanup(func() {
		if t.Failed() {
			for _, msg := range logs.messages() {
				t.Log(msg)
			}
		}
	})
	return logs, stop
}

func TestIntegrationCompat(t *testing.T) {
	if os.Getenv("GO_INTEGRATION_COMPAT") != "1" {
		t.Skip("set GO_INTEGRATION_COMPAT=1 and SPECTER_COMPAT_OLD_WORKTREE")
	}
	old := os.Getenv("SPECTER_COMPAT_OLD_WORKTREE")
	require.NotEmpty(t, old)
	git := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", old}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	require.Empty(t, git("status", "--porcelain"))
	revision := git("rev-parse", "HEAD")
	t.Log("Old revision:", revision)
	require.True(t, strings.HasPrefix(revision, "5d469e8"), "expected pre-feature revision")
	t.Cleanup(func() { require.Empty(t, git("status", "--porcelain")) })
	binary := filepath.Join(t.TempDir(), "specter-old")
	build := exec.Command("go", "build", "-tags", "no_mocks", "-ldflags", "-X go.miragespace.co/specter/cmd/client.devApexOverride=dev.con.nect.sh", "-o", binary, ".")
	build.Dir = old
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))
	startLightweightServers(t, []int{21968, 21969}, []int{21868, 21869})
	certs, err := filepath.Abs("../certs")
	require.NoError(t, err)
	oldLogs, _ := startCompatBinary(t, binary, syscall.SIGTERM, "server", "--cert-dir", certs, "--data-dir", t.TempDir(), "--listen", "127.0.0.1:21973", "--listen-http", "21873", "--apex", serverApex, "--join", "127.0.0.1:21968")
	require.Eventually(t, func() bool {
		count := 0
		for _, msg := range oldLogs.messages() {
			if msg["msg"] == "specter server started" || msg["msg"] == "gateway server started" {
				count++
			}
		}
		return count == 2
	}, 30*time.Second, 50*time.Millisecond)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "compat") }))
	defer target.Close()
	path := filepath.Join(t.TempDir(), "owner.yaml")
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf("version: 2\napex: 127.0.0.1:21973\ntunnels:\n  - target: %s\n", target.URL)), 0600))
	ownerLogs, stopOwner := startCompatBinary(t, binary, syscall.SIGINT, "client", "--insecure", "tunnel", "--config", path)
	var authority string
	require.Eventually(t, func() bool {
		for _, msg := range ownerLogs.messages() {
			if text, ok := msg["msg"].(string); ok && strings.Contains(text, "published") {
				if hostname, ok := msg["hostname"].(string); ok {
					authority = hostname
					return true
				}
			}
		}
		return false
	}, 30*time.Second, 50*time.Millisecond)
	if !strings.Contains(authority, ".") {
		authority += "." + serverApex
	}
	code, body := fetchLightweight(t, authority, 21968)
	require.Equal(t, 200, code, body)
	require.Equal(t, "compat", body)
	stopOwner()
	hostname := strings.TrimSuffix(authority, "."+serverApex)
	output, err := runTokenCLI(t, "mint", "--config", path, hostname)
	require.ErrorContains(t, err, "server does not support domain tokens")
	require.Empty(t, output)
	// The baseline TunnelService lacks OpenEphemeralSession. Classify the missing
	// method as unsupported without printing an ephemeral URL.
	time.Sleep(1100 * time.Millisecond)
	app, _ := compileApp(clientcmd.Generate())
	app.Metadata["apexOverride"] = serverApex
	var sessionOutput bytes.Buffer
	app.Writer = &sessionOutput
	openCtx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	err = app.Run(openCtx, []string{
		"specter", "client", "--insecure", "expose",
		"--apex", "127.0.0.1:21973", target.URL,
	})
	cancel()
	require.ErrorContains(t, err, "server 127.0.0.1:21973 does not support lightweight tunnels")
	require.Empty(t, sessionOutput.String())
	// The same registered owner now uses an upgraded endpoint in the shared ring.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = bytes.Replace(data, []byte("127.0.0.1:21973"), []byte("127.0.0.1:21968"), 1)
	require.NoError(t, os.WriteFile(path, data, 0600))
	output, err = runTokenCLI(t, "mint", "--config", path, hostname)
	require.NoError(t, err)
	var minted struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &minted))
	require.NotEmpty(t, minted.Token)
	tokenPath := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenPath, []byte(minted.Token), 0600))
	logs, _ := startIntegrationApp(t, clientcmd.Generate(), "client", "--insecure", "serve", "--apex", "127.0.0.1:21969", "--token-file", tokenPath, target.URL)
	require.Equal(t, authority, lightweightAuthority(t, logs))
	code, body = fetchLightweight(t, authority, 21973)
	require.Equal(t, 200, code, body)
	require.Equal(t, "compat", body)
	t.Logf("Compatibility record: old=%s; old client -> new gateway=PASS; new mint -> old server=UNSUPPORTED; new session -> old server=UNSUPPORTED; new serve -> old gateway=PASS", revision)
}
