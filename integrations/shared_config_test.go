package integrations

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"go.miragespace.co/specter/acme"
	dnscmd "go.miragespace.co/specter/cmd/dns"
	servercmd "go.miragespace.co/specter/cmd/server"
	"go.miragespace.co/specter/config"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

// A child process lets the fixture crash a physical server and restart it
// without depending on graceful Chord departure.
func TestIntegrationSharedConfigProcess(t *testing.T) {
	if os.Getenv("SPECTER_SHARED_CONFIG_CHILD") != "1" {
		t.Skip("subprocess helper")
	}
	separator := slices.Index(os.Args, "--")
	require.GreaterOrEqual(t, separator, 0)
	args := os.Args[separator+1:]
	require.NotEmpty(t, args)
	var command *cli.Command
	switch args[0] {
	case "server":
		command = servercmd.Generate()
	case "dns":
		command = dnscmd.Generate()
	default:
		t.Fatalf("unknown integration command %q", args[0])
	}
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	config.DisableStacktrace = true
	logger, err := config.Build()
	require.NoError(t, err)
	app, _ := compileApp(command)
	app.Metadata["apexOverride"] = serverApex
	app.Writer = io.Discard
	app.Before = func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		cmd.Root().Metadata["logger"] = logger
		return ctx, nil
	}
	require.NoError(t, app.Run(t.Context(), append([]string{"specter"}, args...)))
}

type sharedConfigProcess struct {
	command *exec.Cmd
	output  bytes.Buffer
	done    chan struct{}
	err     error
}

func startSharedConfigProcess(t *testing.T, args ...string) *sharedConfigProcess {
	t.Helper()
	p := &sharedConfigProcess{done: make(chan struct{})}
	p.command = exec.Command(os.Args[0], append([]string{"-test.run=^TestIntegrationSharedConfigProcess$", "-test.timeout=150s", "--"}, args...)...)
	p.command.Env = append(os.Environ(), "SPECTER_SHARED_CONFIG_CHILD=1", "DOH=false")
	p.command.Stdout = &p.output
	p.command.Stderr = &p.output
	require.NoError(t, p.command.Start())
	go func() {
		p.err = p.command.Wait()
		close(p.done)
	}()
	t.Cleanup(func() {
		p.stop(t)
		if t.Failed() {
			t.Logf("%s: %s", p.command.String(), p.output.String())
		}
	})
	return p
}

func (p *sharedConfigProcess) running(t *testing.T) {
	t.Helper()
	select {
	case <-p.done:
		t.Fatalf("integration process exited: %v\n%s", p.err, p.output.String())
	default:
	}
}

func (p *sharedConfigProcess) stop(t *testing.T) {
	t.Helper()
	select {
	case <-p.done:
		return
	default:
	}
	err := p.command.Process.Kill()
	if !errors.Is(err, os.ErrProcessDone) {
		require.NoError(t, err)
	}
	select {
	case <-p.done:
	case <-time.After(10 * time.Second):
		t.Fatal("integration process did not stop")
	}
}

func sharedConfigListenAddress(t *testing.T) string {
	t.Helper()
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer tcp.Close()
	address := tcp.Addr().String()
	udp, err := net.ListenPacket("udp", address)
	require.NoError(t, err)
	require.NoError(t, udp.Close())
	return address
}

type sharedConfigNode struct {
	dataDir string
	address string
	http    string
	rpc     string
	process *sharedConfigProcess
	client  *http.Client
}

func newSharedConfigNode(t *testing.T) *sharedConfigNode {
	t.Helper()
	transport := &http.Transport{TLSClientConfig: &tls.Config{ServerName: serverApex, InsecureSkipVerify: true}}
	n := &sharedConfigNode{
		dataDir: t.TempDir(),
		address: sharedConfigListenAddress(t),
		http:    sharedConfigListenAddress(t),
		rpc:     sharedConfigListenAddress(t),
		client:  &http.Client{Transport: transport, Timeout: 2 * time.Second},
	}
	t.Cleanup(transport.CloseIdleConnections)
	return n
}

func (n *sharedConfigNode) start(t *testing.T, join string) {
	t.Helper()
	_, httpPort, err := net.SplitHostPort(n.http)
	require.NoError(t, err)
	args := []string{"server", "--cert-dir", "../certs", "--data-dir", n.dataDir,
		"--listen", n.address, "--advertise", n.address, "--listen-http", httpPort,
		"--listen-rpc", "tcp://" + n.rpc, "--apex", serverApex, "--virtual", "2",
		"--kv-provider", "memory",
		"--auth_user", "shared-config", "--auth_pass", "shared-config"}
	if join != "" {
		args = append(args, "--join", join)
	}
	n.process = startSharedConfigProcess(t, args...)
}

func (n *sharedConfigNode) restart(t *testing.T, join string) {
	t.Helper()
	n.start(t, join)
	// Match service-manager recovery: a peer may retain the crashed server's
	// QUIC connection until its idle timeout, causing an early join to fail.
	// Retry exited startups, then check configuration catch-up separately.
	require.Eventually(t, func() bool {
		select {
		case <-n.process.done:
			n.start(t, join)
			return false
		default:
		}
		_, status, err := n.request(t.Context(), http.MethodGet, nil)
		return err == nil && status == http.StatusOK
	}, 45*time.Second, time.Second, "server %s did not restart", n.address)
}

type sharedDNSResponse struct {
	Version string         `json:"version"`
	Config  acme.DNSConfig `json:"config"`
	Server  struct {
		ID      string `json:"id"`
		Address string `json:"address"`
	} `json:"server"`
}

func (n *sharedConfigNode) request(ctx context.Context, method string, body []byte) (sharedDNSResponse, int, error) {
	var result sharedDNSResponse
	request, err := http.NewRequestWithContext(ctx, method, "https://"+n.address+"/_internal/api/config/dns", bytes.NewReader(body))
	if err != nil {
		return result, 0, err
	}
	request.Host = serverApex
	request.SetBasicAuth("shared-config", "shared-config")
	request.Header.Set("Content-Type", "application/json")
	response, err := n.client.Do(request)
	if err != nil {
		return result, 0, err
	}
	defer response.Body.Close()
	err = json.NewDecoder(response.Body).Decode(&result)
	return result, response.StatusCode, err
}

func (n *sharedConfigNode) waitVersion(t *testing.T, record config.Record) {
	t.Helper()
	var lastErr error
	var status int
	var result sharedDNSResponse
	require.Eventually(t, func() bool {
		n.process.running(t)
		result, status, lastErr = n.request(t.Context(), http.MethodGet, nil)
		return lastErr == nil && status == http.StatusOK && result.Version == record.Token()
	}, 30*time.Second, 100*time.Millisecond, "server %s did not reach %s", n.address, record.Token())
	dnsConfig, err := acme.DecodeDNSConfig(record, "")
	require.NoError(t, err)
	require.Equal(t, dnsConfig, result.Config)
	require.Equal(t, n.address, result.Server.Address)
	require.NotEmpty(t, result.Server.ID)
}

func (n *sharedConfigNode) put(t *testing.T, expected config.Record, dnsConfig acme.DNSConfig) config.Record {
	t.Helper()
	body, err := json.Marshal(struct {
		ExpectedVersion string         `json:"expected_version"`
		Config          acme.DNSConfig `json:"config"`
	}{expected.Token(), dnsConfig})
	require.NoError(t, err)
	result, status, err := n.request(t.Context(), http.MethodPut, body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	record, err := config.NewClient("http://"+n.rpc, n.client).Get(t.Context(), acme.DNSConfigName)
	require.NoError(t, err)
	require.Equal(t, expected.Version.Revision+1, record.Version.Revision)
	require.Equal(t, record.Token(), result.Version)
	require.Equal(t, dnsConfig, result.Config)
	return record
}

func waitSharedDNS(t *testing.T, process *sharedConfigProcess, address string, config acme.DNSConfig, revision uint64) {
	t.Helper()
	// Test both listeners and the complete active view, including IPv6 glue.
	for _, network := range []string{"udp", "tcp"} {
		client := &dns.Client{Net: network, Timeout: time.Second}
		require.Eventually(t, func() bool {
			process.running(t)
			for _, kind := range []uint16{dns.TypeNS, dns.TypeSOA, dns.TypeA, dns.TypeAAAA} {
				name := config.Zone
				if kind == dns.TypeA || kind == dns.TypeAAAA {
					name = config.Nameservers[0].Host
				}
				message := new(dns.Msg)
				message.SetQuestion(name, kind)
				response, _, err := client.ExchangeContext(t.Context(), message, address)
				if err != nil || response.Rcode != dns.RcodeSuccess || !response.Authoritative || len(response.Answer) != 1 {
					return false
				}
				switch answer := response.Answer[0].(type) {
				case *dns.NS:
					if answer.Ns != config.Nameservers[0].Host {
						return false
					}
				case *dns.SOA:
					if answer.Serial != uint32(revision) || answer.Ns != config.Nameservers[0].Host {
						return false
					}
				case *dns.A:
					if !slices.Contains(config.Nameservers[0].Addresses, answer.A.String()) {
						return false
					}
				case *dns.AAAA:
					if !slices.Contains(config.Nameservers[0].Addresses, answer.AAAA.String()) {
						return false
					}
				default:
					return false
				}
			}
			return true
		}, 15*time.Second, 100*time.Millisecond, "%s DNS did not activate revision %d", network, revision)
	}
}

func TestIntegrationSharedConfig(t *testing.T) {
	if os.Getenv("GO_INTEGRATION_TUNNEL") == "" {
		t.Skip("set GO_INTEGRATION_TUNNEL=1")
	}
	nodes := []*sharedConfigNode{newSharedConfigNode(t), newSharedConfigNode(t), newSharedConfigNode(t)}
	initial := acme.DNSConfig{
		Zone:        "shared.test.",
		Nameservers: []acme.DNSNameserver{{Host: "ns.shared.test.", Addresses: []string{"192.0.2.1", "2001:db8::1"}}},
	}
	value, err := json.Marshal(initial)
	require.NoError(t, err)
	record, err := config.NewRecord(acme.DNSConfigName, acme.DNSConfigSchema, 1, value)
	require.NoError(t, err)
	// A future consumer's binary payload must survive without any DNS decoding.
	opaque, err := config.NewRecord("future.example/opaque", 37, 1, []byte{0, 255, 128, 10, 42})
	require.NoError(t, err)
	// Start empty replicas before initialization, as during a first rollout.
	for i, node := range nodes {
		join := ""
		if i > 0 {
			join = nodes[0].address
			_, err := os.Stat(filepath.Join(node.dataDir, "config", config.FileName))
			require.ErrorIs(t, err, os.ErrNotExist, "only the first physical server is initialized")
		}
		node.start(t, join)
		require.Eventually(t, func() bool {
			node.process.running(t)
			response, status, err := node.request(t.Context(), http.MethodGet, nil)
			return err == nil && status == http.StatusOK && response.Version == ""
		}, 10*time.Second, 100*time.Millisecond, "server %s did not start with an empty replica", node.address)
	}
	// A separate process can populate the first database while its server runs.
	require.NoError(t, config.Init(filepath.Join(nodes[0].dataDir, "config"), []config.Record{record, opaque}))
	for _, node := range nodes {
		node.waitVersion(t, record)
	}

	dnsAddress := sharedConfigListenAddress(t)
	// Existing launcher flags must not override the shared DNS document.
	dnsProcess := startSharedConfigProcess(t, "dns", "--listen", dnsAddress, "--rpc", "tcp://"+nodes[2].rpc,
		"--acme", "acme://admin@example.com@shared.test", "--acme-ns", "old.shared.test/192.0.2.99")
	waitSharedDNS(t, dnsProcess, dnsAddress, initial, record.Version.Revision)

	localClient := &http.Client{Timeout: 2 * time.Second}
	localURL := "http://" + nodes[2].rpc
	localRecord, err := config.NewClient(localURL, localClient).Get(t.Context(), acme.DNSConfigName)
	require.NoError(t, err)
	require.Equal(t, record, localRecord)
	_, err = protocol.NewConfigServiceProtobufClient(localURL, localClient).Exchange(t.Context(), &protocol.ConfigSnapshot{})
	var rpcError twirp.Error
	require.ErrorAs(t, err, &rpcError)
	require.Equal(t, twirp.BadRoute, rpcError.Code(), "the local listener must not expose peer exchange")

	edited := acme.DNSConfig{
		Zone:        initial.Zone,
		Nameservers: []acme.DNSNameserver{{Host: "updated.shared.test.", Addresses: []string{"192.0.2.2", "2001:db8::2"}}},
	}
	record = nodes[1].put(t, record, edited)
	for _, node := range nodes {
		node.waitVersion(t, record)
	}
	waitSharedDNS(t, dnsProcess, dnsAddress, edited, record.Version.Revision)

	// Stop the original source abruptly, and inspect its durable full replica.
	nodes[0].process.stop(t)
	store, err := config.Open(filepath.Join(nodes[0].dataDir, "config"))
	require.NoError(t, err)
	persisted, found, err := store.Get(acme.DNSConfigName)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, record, persisted)
	require.NoError(t, store.Close())

	edited.Nameservers[0].Addresses = []string{"192.0.2.3", "2001:db8::3"}
	record = nodes[1].put(t, record, edited)
	nodes[2].waitVersion(t, record)
	waitSharedDNS(t, dnsProcess, dnsAddress, edited, record.Version.Revision)

	// A restarted server retains its old copy and repairs it from a live peer.
	nodes[0].restart(t, nodes[1].address)
	nodes[0].waitVersion(t, record)
	for _, node := range nodes {
		node.process.stop(t)
		store, err := config.Open(filepath.Join(node.dataDir, "config"))
		require.NoError(t, err)
		records, err := store.Snapshot()
		require.NoError(t, err)
		require.ElementsMatch(t, []config.Record{record, opaque}, records, "every physical node owns a complete durable copy")
		require.NoError(t, store.Close())
		for vnode := range 2 {
			_, err := os.Stat(filepath.Join(node.dataDir, fmt.Sprint(vnode), "config"))
			require.ErrorIs(t, err, os.ErrNotExist, "virtual nodes must not own shared configuration replicas")
		}
	}
}
