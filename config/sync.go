package config

import (
	"context"
	"crypto/sha256"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/util"
	"go.miragespace.co/specter/util/acceptor"

	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
)

const (
	// One protobuf bytes field adds a tag and at most four length bytes.
	maxSyncBytes     = MaxSnapshotBytes + 5
	maxParallelSyncs = 4
)

// Syncer exchanges complete configuration snapshots along the existing Chord
// peer graph. It neither discovers fleet membership nor assigns key ownership.
// Each physical server has one Syncer, regardless of its virtual node count.
type Syncer struct {
	store     *Store
	transport transport.Transport
	peers     func() []*protocol.Node
	logger    *zap.Logger
	incoming  chan struct{}
	handling  chan struct{}
	wake      chan struct{}
	receipts  map[string]*syncReceipt
	interval  time.Duration
	timeout   time.Duration
}

// A physical connection acknowledges only the snapshot sent on it. A failed
// attempt retains its digest and retry time so unrelated wakeups cannot spin.
// The serial synchronization rounds own these receipts; none are persisted.
type syncReceipt struct {
	digest   [sha256.Size]byte
	physical transport.PhysicalConn
	retryAt  time.Time
}

// NewSyncer takes the authenticated Chord transport and a source of peer graph
// edges. Include the successors of every local virtual node as well as current
// connections: a connection cache alone is not a membership view. Addresses are
// deduplicated before dialing so multiple virtual nodes share one exchange.
func NewSyncer(store *Store, chordTransport transport.Transport, peers func() []*protocol.Node, logger *zap.Logger) *Syncer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Syncer{
		store:     store,
		transport: chordTransport,
		peers:     peers,
		logger:    logger,
		incoming:  make(chan struct{}, maxParallelSyncs),
		handling:  make(chan struct{}, maxParallelSyncs),
		wake:      make(chan struct{}, 1),
		receipts:  make(map[string]*syncReceipt),
		interval:  15 * time.Second,
		timeout:   transport.ConnectTimeout,
	}
}

// Bind registers only on the mutually authenticated Chord side of the router.
// It must be called before the router starts accepting streams. No handler is
// exposed to the tunnel transport or the unauthenticated local RPC listener.
func (s *Syncer) Bind(ctx context.Context, router *transport.StreamRouter) {
	listener := acceptor.NewH2Acceptor(nil)
	srv := &http.Server{
		BaseContext:       func(net.Listener) context.Context { return ctx },
		Handler:           s.rpcHandler(),
		MaxHeaderBytes:    1 << 10,
		ReadHeaderTimeout: s.timeout,
		ReadTimeout:       s.timeout,
		WriteTimeout:      s.timeout,
		ErrorLog:          util.GetStdLogger(s.logger, "config_rpc"),
	}
	srv.SetKeepAlivesEnabled(false)
	go srv.Serve(listener)
	context.AfterFunc(ctx, func() { srv.Close() })

	router.HandleChord(protocol.Stream_CONFIG, nil, func(delegate *transport.StreamDelegate) {
		if ctx.Err() != nil {
			delegate.Close()
			return
		}
		select {
		case s.incoming <- struct{}{}:
		default:
			delegate.Close()
			return
		}
		streamCtx, cancel := context.WithTimeout(ctx, s.timeout)
		conn := &syncConn{StreamDelegate: delegate, closed: func() {
			cancel()
			<-s.incoming
		}}
		// HTTP may reset I/O deadlines. This timer bounds the entire stream,
		// including time queued for acceptance, request decoding, and replies.
		context.AfterFunc(streamCtx, func() { conn.Close() })
		listener.Handle(conn)
	})
}

// syncConn keeps the admission slot until HTTP has finished using the stream.
type syncConn struct {
	*transport.StreamDelegate
	once   sync.Once
	closed func()
}

func (c *syncConn) Close() (err error) {
	c.once.Do(func() {
		err = c.StreamDelegate.Close()
		c.closed()
	})
	return err
}

func (s *Syncer) rpcHandler() http.Handler {
	handler := protocol.NewConfigServiceServer(s, twirp.WithServerHooks(&twirp.ServerHooks{
		Error: func(ctx context.Context, err twirp.Error) context.Context {
			if ctx.Err() == nil {
				s.logger.Debug("Shared configuration exchange failed", zap.Error(err))
			}
			return ctx
		},
	}))
	bounded := util.LimitBody(maxSyncBytes)(handler)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Peers use the protobuf client. Excluding the optional JSON encoding
		// keeps base64 expansion from increasing the response's wire bound.
		if r.Header.Get("Content-Type") != "application/protobuf" {
			twirp.WriteError(w, twirp.Malformed.Error("configuration exchange requires application/protobuf"))
			return
		}
		// Closing a timed-out stream cannot interrupt SQLite or a store mutex
		// wait. Keep decoded request work bounded until its handler returns.
		select {
		case s.handling <- struct{}{}:
			defer func() { <-s.handling }()
		default:
			twirp.WriteError(w, twirp.ResourceExhausted.Error("configuration exchanges are busy"))
			return
		}
		bounded.ServeHTTP(w, r)
	})
}

// Wake requests a local state check without waiting for the periodic check.
// Repeated requests coalesce, and unchanged, acknowledged state stays quiet.
func (s *Syncer) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run checks local state and peer connections at startup and periodically. The
// periodic read also observes writes by other processes and uncertain commits.
// Only pending snapshots are exchanged. Cancellation waits for outgoing work.
func (s *Syncer) Run(ctx context.Context) {
	s.receipts = make(map[string]*syncReceipt)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		s.sync(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
	}
}

func (s *Syncer) sync(ctx context.Context) {
	request, err := s.snapshot()
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Debug("Could not read shared configuration for synchronization", zap.Error(err))
		}
		return
	}
	digest := sha256.Sum256(request.GetData())
	connected := make(map[string]transport.PhysicalConn)
	for _, peer := range s.transport.ListConnected() {
		connected[peer.Identity.GetAddress()] = peer.Physical
	}
	seen := map[string]bool{s.transport.Identity().GetAddress(): true, "": true}
	var addresses []string
	for _, peer := range s.peers() {
		address := peer.GetAddress()
		if seen[address] {
			continue
		}
		seen[address] = true
		addresses = append(addresses, address)
	}
	// Avoid consistently filling the outgoing slots with the same address prefix.
	rand.Shuffle(len(addresses), func(i, j int) {
		addresses[i], addresses[j] = addresses[j], addresses[i]
	})
	active := make(chan struct{}, maxParallelSyncs)
	var wg sync.WaitGroup
	defer wg.Wait()
	for _, address := range addresses {
		receipt := s.receipts[address]
		if receipt == nil {
			receipt = &syncReceipt{}
			s.receipts[address] = receipt
		}
		if receipt.physical != nil {
			if receipt.physical != connected[address] {
				receipt.physical = nil
			} else {
				select {
				case <-receipt.physical.Done():
					receipt.physical = nil
				default:
				}
			}
		}
		if receipt.digest == digest && (receipt.physical != nil || time.Now().Before(receipt.retryAt)) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case active <- struct{}{}:
		}
		wg.Go(func() {
			defer func() { <-active }()
			// CONFIG is physical: a virtual node ID is neither required nor
			// allowed to create an independent configuration replica.
			peer := &protocol.Node{Address: address}
			physical, err := s.exchange(ctx, peer, request)
			// A response may have advanced SQLite, or a writer may have saved
			// while this request was in flight. Neither acknowledges that newer
			// state: record only the immutable snapshot sent by this round.
			receipt.digest = digest
			receipt.physical = physical
			receipt.retryAt = time.Now().Add(s.interval)
			if err != nil && ctx.Err() == nil {
				s.logger.Debug("Could not synchronize shared configuration", zap.Object("peer", peer), zap.Error(err))
			}
		})
	}
	for address := range s.receipts {
		if !seen[address] {
			delete(s.receipts, address)
		}
	}
}

func (s *Syncer) exchange(ctx context.Context, peer *protocol.Node, request *protocol.ConfigSnapshot) (transport.PhysicalConn, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	var physical transport.PhysicalConn
	var physicalMu sync.Mutex
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			var used transport.PhysicalConn
			if provider, ok := info.Conn.(transport.PhysicalConnProvider); ok {
				used = provider.PhysicalConn()
			}
			physicalMu.Lock()
			physical = used
			physicalMu.Unlock()
		},
	})
	// Each exchange owns one HTTP connection and its dial context. QUIC still
	// reuses the physical peer connection underneath the individual stream.
	httpTransport := &http.Transport{
		DialTLSContext:         rpc.StreamDialer(ctx, s.transport, protocol.Stream_CONFIG),
		DisableKeepAlives:      true,
		DisableCompression:     true,
		MaxResponseHeaderBytes: 1 << 10,
	}
	defer httpTransport.CloseIdleConnections()
	client := protocol.NewConfigServiceProtobufClient("https://"+peer.GetAddress(), boundedClient(&http.Client{
		Transport: httpTransport,
	}, maxSyncBytes))
	response, err := client.Exchange(rpc.WithNode(ctx, peer), request)
	if err != nil {
		return nil, err
	}
	records, err := DecodeSnapshot(response.GetData())
	if err != nil {
		return nil, err
	}
	if err := s.store.Merge(records); err != nil {
		return nil, err
	}
	s.Wake()
	physicalMu.Lock()
	defer physicalMu.Unlock()
	return physical, nil
}

func (s *Syncer) snapshot() (*protocol.ConfigSnapshot, error) {
	records, err := s.store.Snapshot()
	if err != nil {
		return nil, err
	}
	data, err := EncodeSnapshot(records)
	if err != nil {
		return nil, err
	}
	return &protocol.ConfigSnapshot{Data: data}, nil
}

// Exchange merges a peer's complete snapshot before returning the observed local
// snapshot. Namespace-specific payloads stay opaque to both sides of the RPC.
func (s *Syncer) Exchange(_ context.Context, request *protocol.ConfigSnapshot) (*protocol.ConfigSnapshot, error) {
	records, err := DecodeSnapshot(request.GetData())
	if err != nil {
		return nil, twirp.InvalidArgument.Error(err.Error())
	}
	if err := s.store.Merge(records); err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	s.Wake()
	response, err := s.snapshot()
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	return response, nil
}
