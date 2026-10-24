package config

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/transport"

	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
)

type syncTestTransport struct {
	transport.Transport
	identity  *protocol.Node
	incoming  chan *transport.StreamDelegate
	connected []transport.ConnectedPeer
	dial      func(context.Context, *protocol.Node, protocol.Stream_Type) (net.Conn, error)
}

func newSyncTestTransport(address string) *syncTestTransport {
	return &syncTestTransport{
		identity: &protocol.Node{Address: address},
		incoming: make(chan *transport.StreamDelegate, 16),
	}
}

func (t *syncTestTransport) Identity() *protocol.Node { return t.identity }

func (t *syncTestTransport) AcceptStream() <-chan *transport.StreamDelegate { return t.incoming }

func (t *syncTestTransport) ListConnected() []transport.ConnectedPeer { return t.connected }

func (t *syncTestTransport) DialStream(ctx context.Context, peer *protocol.Node, kind protocol.Stream_Type) (net.Conn, error) {
	return t.dial(ctx, peer, kind)
}

type syncTestPhysicalConn struct {
	transport.PhysicalConn
	done chan struct{}
}

func (c *syncTestPhysicalConn) Done() <-chan struct{} { return c.done }

func syncTestDial(ctx context.Context, target *syncTestTransport, kind protocol.Stream_Type) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case target.incoming <- &transport.StreamDelegate{Conn: server, Kind: kind, Identity: target.identity}:
		return client, nil
	case <-ctx.Done():
		client.Close()
		server.Close()
		return nil, ctx.Err()
	}
}

func bindSyncTest(t *testing.T, syncer *Syncer, chordTransport *syncTestTransport) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	router := transport.NewStreamRouter(zap.NewNop(), chordTransport, nil)
	syncer.Bind(ctx, router)
	router.Accept(ctx)
	t.Cleanup(cancel)
	return cancel
}

func runSyncTest(t *testing.T, syncer *Syncer) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		syncer.Run(ctx)
	}()
	stop := func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("shared configuration sync did not stop after cancellation")
		}
	}
	t.Cleanup(stop)
	return stop
}

func TestSyncerRetriesFailedExchangeWithoutAnotherWrite(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	tp := newSyncTestTransport("local:443")
	var attempts atomic.Int32
	tp.dial = func(context.Context, *protocol.Node, protocol.Stream_Type) (net.Conn, error) {
		attempts.Add(1)
		return nil, errors.New("temporarily unavailable")
	}
	syncer := NewSyncer(store, tp, func() []*protocol.Node {
		return []*protocol.Node{{Address: "peer:443"}}
	}, nil)
	syncer.interval = 10 * time.Millisecond
	runSyncTest(t, syncer)
	require.Eventually(t, func() bool { return attempts.Load() >= 2 }, time.Second, time.Millisecond)
}

func TestSyncerPreservesPhysicalTargetsAndReceipts(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	tp := newSyncTestTransport("local:443")
	acknowledged := &protocol.Node{Address: "acknowledged:443"}
	peers := []*protocol.Node{
		nil, {}, {Address: "local:443"},
		{Id: 1, Address: "a:443"}, {Id: 2, Address: "b:443"}, {Id: 3, Address: "a:443"},
		acknowledged, {Address: "backoff:443"},
	}
	original := append([]*protocol.Node(nil), peers...)
	physical := &syncTestPhysicalConn{done: make(chan struct{})}
	tp.connected = []transport.ConnectedPeer{{Identity: acknowledged, Physical: physical}}
	var attemptsMu sync.Mutex
	attempts := make(map[string]int)
	tp.dial = func(_ context.Context, peer *protocol.Node, _ protocol.Stream_Type) (net.Conn, error) {
		attemptsMu.Lock()
		attempts[peer.GetAddress()]++
		attemptsMu.Unlock()
		return nil, errors.New("temporarily unavailable")
	}
	syncer := NewSyncer(store, tp, func() []*protocol.Node { return peers }, nil)
	request, err := syncer.snapshot()
	require.NoError(t, err)
	digest := sha256.Sum256(request.Data)
	retryAt := time.Now().Add(time.Hour)
	syncer.receipts["acknowledged:443"] = &syncReceipt{digest: digest, physical: physical}
	syncer.receipts["backoff:443"] = &syncReceipt{digest: digest, retryAt: retryAt}
	syncer.receipts["departed:443"] = &syncReceipt{}

	syncer.sync(t.Context())

	require.Equal(t, map[string]int{"a:443": 1, "b:443": 1}, attempts)
	require.Equal(t, original, peers, "peer selection must not reorder caller storage")
	require.Equal(t, &syncReceipt{digest: digest, physical: physical}, syncer.receipts["acknowledged:443"])
	require.Equal(t, &syncReceipt{digest: digest, retryAt: retryAt}, syncer.receipts["backoff:443"])
	require.NotContains(t, syncer.receipts, "departed:443")
}

func TestSyncerReceiptRequiresCurrentStateAndConnection(t *testing.T) {
	for _, change := range []string{
		"unchanged", "merged document", "uncertain commit",
		"missing connection", "ended connection", "replacement connection",
	} {
		t.Run(change, func(t *testing.T) {
			store := openTestStore(t, t.TempDir())
			record, err := store.Put("dns", 1, []byte("before"), "")
			require.NoError(t, err)
			peer := &protocol.Node{Address: "peer:443"}
			physical := &syncTestPhysicalConn{done: make(chan struct{})}
			tp := newSyncTestTransport("local:443")
			tp.connected = []transport.ConnectedPeer{{Identity: peer, Physical: physical}}
			var attempts atomic.Int32
			tp.dial = func(context.Context, *protocol.Node, protocol.Stream_Type) (net.Conn, error) {
				attempts.Add(1)
				return nil, errors.New("temporarily unavailable")
			}
			// A configured join peer remains eligible even after its cached
			// connection disappears; the address alone cannot be a receipt.
			syncer := NewSyncer(store, tp, func() []*protocol.Node { return []*protocol.Node{peer} }, nil)
			request, err := syncer.snapshot()
			require.NoError(t, err)
			syncer.receipts[peer.Address] = &syncReceipt{
				digest: sha256.Sum256(request.Data), physical: physical,
			}
			switch change {
			case "merged document":
				incoming, err := NewRecord("future.feature", 19, 1, []byte{0xff, 0})
				require.NoError(t, err)
				require.NoError(t, store.Merge([]Record{incoming}))
			case "uncertain commit":
				store.commit = func(tx *sql.Tx) error {
					if err := tx.Commit(); err != nil {
						return err
					}
					return errors.New("commit acknowledgement lost")
				}
				_, err := store.Put("dns", 1, []byte("after"), record.Token())
				require.ErrorIs(t, err, ErrDurabilityUncertain)
			case "missing connection":
				tp.connected = nil
			case "ended connection":
				close(physical.done)
			case "replacement connection":
				tp.connected[0].Physical = &syncTestPhysicalConn{done: make(chan struct{})}
			}
			syncer.sync(t.Context())
			if change == "unchanged" {
				require.Zero(t, attempts.Load(), "an acknowledged live connection needs no repeated snapshot")
			} else {
				require.EqualValues(t, 1, attempts.Load(), "stale receipts must leave work pending")
			}
			syncer.sync(t.Context())
			if change == "unchanged" {
				require.Zero(t, attempts.Load())
			} else {
				require.EqualValues(t, 1, attempts.Load(), "another observation must respect the failed attempt's retry delay")
			}
		})
	}
}

func TestSyncerRetriesUnsupportedStreamWithoutFallback(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	before, err := store.Put("dns", 1, []byte("retained"), "")
	require.NoError(t, err)
	a := newSyncTestTransport("a:443")
	old := newSyncTestTransport("old:443")
	var attempts atomic.Int32
	var fallbacks atomic.Int32
	a.dial = func(ctx context.Context, _ *protocol.Node, kind protocol.Stream_Type) (net.Conn, error) {
		if kind != protocol.Stream_CONFIG {
			fallbacks.Add(1)
			return nil, errors.New("unsafe fallback to another stream kind")
		}
		attempts.Add(1)
		return syncTestDial(ctx, old, kind)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// An older server has no CONFIG handler and closes the unknown stream.
	transport.NewStreamRouter(zap.NewNop(), old, nil).Accept(ctx)
	client := NewSyncer(store, a, func() []*protocol.Node { return []*protocol.Node{old.Identity()} }, nil)
	client.interval = 10 * time.Millisecond
	stop := runSyncTest(t, client)
	require.Eventually(t, func() bool { return attempts.Load() >= 2 }, time.Second, time.Millisecond)
	stop()
	require.Zero(t, fallbacks.Load())
	require.Equal(t, []Record{before}, storeSnapshot(t, store))
}

func TestSyncerBoundsStalledStreamsAndCancellation(t *testing.T) {
	for _, cancelEarly := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%t", cancelEarly), func(t *testing.T) {
			store := openTestStore(t, t.TempDir())
			a := newSyncTestTransport("a:443")
			stream, peer := net.Pipe()
			defer stream.Close()
			defer peer.Close()
			a.dial = func(context.Context, *protocol.Node, protocol.Stream_Type) (net.Conn, error) {
				return stream, nil
			}
			client := NewSyncer(store, a, nil, nil)
			request, err := client.snapshot()
			require.NoError(t, err)
			client.timeout = 20 * time.Millisecond
			if cancelEarly {
				client.timeout = time.Hour
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := client.exchange(ctx, &protocol.Node{Address: "stalled:443"}, request)
				done <- err
			}()
			if cancelEarly {
				// Cancel while waiting for a response. The timeout case may
				// expire before the request is written, which is also valid.
				httpRequest, err := http.ReadRequest(bufio.NewReader(peer))
				require.NoError(t, err)
				_, err = io.Copy(io.Discard, httpRequest.Body)
				require.NoError(t, err)
				require.NoError(t, httpRequest.Body.Close())
				cancel()
			}
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(time.Second):
				t.Fatal("stalled stream was not closed")
			}
		})
	}
}

func TestSyncerBoundsDialLifetime(t *testing.T) {
	for _, cancelEarly := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%t", cancelEarly), func(t *testing.T) {
			store := openTestStore(t, t.TempDir())
			tp := newSyncTestTransport("local:443")
			started := make(chan struct{})
			stopped := make(chan struct{})
			tp.dial = func(ctx context.Context, _ *protocol.Node, _ protocol.Stream_Type) (net.Conn, error) {
				defer close(stopped)
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			}
			syncer := NewSyncer(store, tp, nil, nil)
			request, err := syncer.snapshot()
			require.NoError(t, err)
			syncer.timeout = 20 * time.Millisecond
			if cancelEarly {
				syncer.timeout = time.Hour
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := syncer.exchange(ctx, &protocol.Node{Address: "peer:443"}, request)
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("exchange did not start dialing")
			}
			if cancelEarly {
				cancel()
			}
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(time.Second):
				t.Fatal("exchange did not stop")
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("HTTP detached the dial from the exchange cancellation")
			}
		})
	}
}

func TestSyncerAdmissionLastsUntilStreamCloses(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	tp := newSyncTestTransport("local:443")
	syncer := NewSyncer(store, tp, nil, nil)
	syncer.timeout = time.Hour
	cancel := bindSyncTest(t, syncer, tp)
	peers := make([]net.Conn, 0, maxParallelSyncs+1)
	for range maxParallelSyncs {
		peer, err := syncTestDial(t.Context(), tp, protocol.Stream_CONFIG)
		require.NoError(t, err)
		peers = append(peers, peer)
		t.Cleanup(func() { peer.Close() })
	}
	require.Eventually(t, func() bool { return len(syncer.incoming) == maxParallelSyncs }, time.Second, time.Millisecond)
	rejected, err := syncTestDial(t.Context(), tp, protocol.Stream_CONFIG)
	require.NoError(t, err)
	defer rejected.Close()
	if err := rejected.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		require.ErrorIs(t, err, io.ErrClosedPipe)
	}
	_, err = rejected.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF, "a fifth stream must be rejected while the first four are still idle")

	require.NoError(t, peers[0].Close())
	require.Eventually(t, func() bool { return len(syncer.incoming) == maxParallelSyncs-1 }, time.Second, time.Millisecond)
	replacement, err := syncTestDial(t.Context(), tp, protocol.Stream_CONFIG)
	require.NoError(t, err)
	defer replacement.Close()
	require.Eventually(t, func() bool { return len(syncer.incoming) == maxParallelSyncs }, time.Second, time.Millisecond)
	cancel()
	require.Eventually(t, func() bool { return len(syncer.incoming) == 0 }, time.Second, time.Millisecond)
	for _, peer := range append(peers[1:], replacement) {
		_, err := peer.Read(make([]byte, 1))
		require.ErrorIs(t, err, io.EOF, "shutdown must close accepted streams")
	}
}

func TestSyncerBoundsIncomingStreamLifetime(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	tp := newSyncTestTransport("local:443")
	syncer := NewSyncer(store, tp, nil, nil)
	syncer.timeout = 20 * time.Millisecond
	bindSyncTest(t, syncer, tp)
	peer, err := syncTestDial(t.Context(), tp, protocol.Stream_CONFIG)
	require.NoError(t, err)
	defer peer.Close()
	if err := peer.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		require.ErrorIs(t, err, io.ErrClosedPipe)
	}
	_, err = peer.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
	require.Eventually(t, func() bool { return len(syncer.incoming) == 0 }, time.Second, time.Millisecond)
}

func TestSyncerBoundsHandlerWorkAfterStreamsTimeOut(t *testing.T) {
	local := openTestStore(t, t.TempDir())
	remote := openTestStore(t, t.TempDir())
	a := newSyncTestTransport("local:443")
	b := newSyncTestTransport("remote:443")
	a.dial = func(ctx context.Context, _ *protocol.Node, kind protocol.Stream_Type) (net.Conn, error) {
		return syncTestDial(ctx, b, kind)
	}
	server := NewSyncer(remote, b, nil, nil)
	server.timeout = 100 * time.Millisecond
	bindSyncTest(t, server, b)
	client := NewSyncer(local, a, nil, nil)
	client.timeout = time.Second
	request, err := client.snapshot()
	require.NoError(t, err)

	lease, err := remote.db.Conn(t.Context())
	require.NoError(t, err)
	release := sync.OnceFunc(func() { require.NoError(t, lease.Close()) })
	t.Cleanup(release)
	done := make(chan error, maxParallelSyncs)
	for range maxParallelSyncs {
		go func() {
			_, err := client.exchange(t.Context(), b.Identity(), request)
			done <- err
		}()
	}
	require.Eventually(t, func() bool { return len(server.handling) == maxParallelSyncs }, time.Second, time.Millisecond)
	for range maxParallelSyncs {
		select {
		case err := <-done:
			require.Error(t, err, "blocked storage must outlive the stream timeout")
		case <-time.After(time.Second):
			t.Fatal("incoming stream was not closed on timeout")
		}
	}
	require.Eventually(t, func() bool { return len(server.incoming) == 0 }, time.Second, time.Millisecond)
	require.Len(t, server.handling, maxParallelSyncs, "timed-out handlers still waiting for storage must retain work admission")
	_, err = client.exchange(t.Context(), b.Identity(), request)
	var rpcError twirp.Error
	require.ErrorAs(t, err, &rpcError)
	require.Equal(t, twirp.ResourceExhausted, rpcError.Code(), "a new request must be rejected before decoding while old handlers remain blocked")

	release()
	require.Eventually(t, func() bool { return len(server.handling) == 0 }, time.Second, time.Millisecond)
	_, err = client.exchange(t.Context(), b.Identity(), request)
	require.NoError(t, err, "work admission must recover after blocked handlers return")
}

func TestSyncerBoundsSuccessfulAndErrorResponses(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			store := openTestStore(t, t.TempDir())
			before, err := store.Put("retained", 1, []byte("unchanged"), "")
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				w.WriteHeader(status)
				w.Write(bytes.Repeat([]byte("x"), maxSyncBytes+1))
			}))
			defer server.Close()
			address := strings.TrimPrefix(server.URL, "http://")
			tp := newSyncTestTransport("local:443")
			tp.dial = func(ctx context.Context, _ *protocol.Node, _ protocol.Stream_Type) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", address)
			}
			syncer := NewSyncer(store, tp, nil, nil)
			request, err := syncer.snapshot()
			require.NoError(t, err)
			_, err = syncer.exchange(t.Context(), &protocol.Node{Address: address}, request)
			var oversized *http.MaxBytesError
			require.ErrorAs(t, err, &oversized, "response must be bounded before generated success/error decoding")
			require.Equal(t, []Record{before}, storeSnapshot(t, store))
		})
	}
}

func TestSyncerDoesNotAcknowledgeUnavailableStorage(t *testing.T) {
	local := openTestStore(t, t.TempDir())
	remote := openTestStore(t, t.TempDir())
	a := newSyncTestTransport("local:443")
	b := newSyncTestTransport("remote:443")
	var calls atomic.Int32
	a.dial = func(ctx context.Context, _ *protocol.Node, kind protocol.Stream_Type) (net.Conn, error) {
		calls.Add(1)
		return syncTestDial(ctx, b, kind)
	}
	bindSyncTest(t, NewSyncer(remote, b, nil, nil), b)
	client := NewSyncer(local, a, func() []*protocol.Node { return []*protocol.Node{b.Identity()} }, nil)
	request, err := client.snapshot()
	require.NoError(t, err)
	require.NoError(t, remote.Close())
	_, err = client.exchange(t.Context(), b.Identity(), request)
	var rpcError twirp.Error
	require.ErrorAs(t, err, &rpcError)
	require.Equal(t, twirp.Internal, rpcError.Code())
	require.EqualValues(t, 1, calls.Load())

	require.NoError(t, local.Close())
	client.sync(t.Context())
	require.EqualValues(t, 1, calls.Load(), "a failed local snapshot must not start an exchange")
}

func TestSyncerRejectsMalformedStateBeforeMerge(t *testing.T) {
	good, err := NewRecord("new", 1, 1, []byte("valid"))
	require.NoError(t, err)
	bad := good
	bad.Name = "corrupt"
	bad.Version.Digest[0] ^= 1
	invalidRecords, err := json.Marshal(map[string]any{"format": 1, "records": []Record{good, bad}})
	require.NoError(t, err)
	for name, data := range map[string][]byte{
		"oversized":        bytes.Repeat([]byte(" "), MaxSnapshotBytes+1),
		"invalid document": invalidRecords,
	} {
		t.Run(name, func(t *testing.T) {
			store := openTestStore(t, t.TempDir())
			before, err := store.Put("retained", 1, []byte("unchanged"), "")
			require.NoError(t, err)
			syncer := NewSyncer(store, nil, nil, nil)
			server := httptest.NewServer(syncer.rpcHandler())
			defer server.Close()
			client := protocol.NewConfigServiceProtobufClient(server.URL, server.Client())
			_, err = client.Exchange(t.Context(), &protocol.ConfigSnapshot{Data: data})
			require.Error(t, err)
			if name == "oversized" {
				require.Contains(t, err.Error(), "request body", "HTTP must reject oversized bodies before protobuf decoding")
			}
			require.Equal(t, []Record{before}, storeSnapshot(t, store))
		})
	}
}

func TestSyncerDoesNotExposeConfigOnTunnelTransport(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	_, err := store.Put("private", 1, []byte("configuration"), "")
	require.NoError(t, err)
	chordTransport := newSyncTestTransport("chord:443")
	tunnelTransport := newSyncTestTransport("tunnel:443")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	router := transport.NewStreamRouter(zap.NewNop(), chordTransport, tunnelTransport)
	NewSyncer(store, chordTransport, nil, nil).Bind(ctx, router)
	conn, err := syncTestDial(ctx, tunnelTransport, protocol.Stream_CONFIG)
	require.NoError(t, err)
	defer conn.Close()
	// Set the deadline before the router can reject and close the stream.
	require.NoError(t, conn.SetDeadline(time.Now().Add(time.Second)))
	router.Accept(ctx)
	_, err = conn.Read(make([]byte, 1))
	require.Error(t, err)
	var timeout net.Error
	require.False(t, errors.As(err, &timeout) && timeout.Timeout(), "tunnel stream must be rejected immediately")
}
