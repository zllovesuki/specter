package rpc

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util/ratecounter"

	pool "github.com/libp2p/go-buffer-pool"
	"github.com/twitchtv/twirp"
)

const (
	// uint32
	LengthSize = 4
)

type ChordClient interface {
	protocol.VNodeService
	protocol.KVService
	RatePer(interval time.Duration) float64
}

func getDynamicDialer(baseCtx context.Context, transport transport.Transport) func(reqCtx context.Context, _, _ string) (net.Conn, error) {
	return func(reqCtx context.Context, _, _ string) (net.Conn, error) {
		peer, ok := reqCtx.Value(contextNodeKey).(*protocol.Node)
		if !ok {
			return nil, fmt.Errorf("keys.ContextNodeKey not found in context")
		}
		return transport.DialStream(baseCtx, peer, protocol.Stream_RPC)
	}
}

// DynamicChordClient returns a rpc client suitable for both VNodeService and KVService, with the destination
// set per call dynamically according to the destination in context. Use WithNode(ctx, node) at call site to dynamically dispatch.
func DynamicChordClient(baseContext context.Context, chordTransport transport.Transport) ChordClient {
	outboundRate := ratecounter.New(time.Second, time.Second*5)

	injector := &twirp.ClientHooks{
		RequestPrepared: func(ctx context.Context, r *http.Request) (context.Context, error) {
			peer, ok := ctx.Value(contextNodeKey).(*protocol.Node)
			if !ok {
				return nil, fmt.Errorf("keys.ContextNodeKey not found in context")
			}
			SerializeContextHeader(ctx, r.Header)
			r.URL.Host = peer.GetAddress() // needed to override dialer instead of using http://chord as key
			outboundRate.Increment()
			return ctx, nil
		},
	}

	// default to http client pooling
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConnsPerHost = 10
	t.IdleConnTimeout = time.Minute
	t.DialContext = getDynamicDialer(baseContext, chordTransport)
	c := &http.Client{
		Transport: t,
	}
	// disable in testing
	if disable, ok := baseContext.Value(contextDisablePoolKey).(bool); ok && disable {
		t.DisableKeepAlives = true
		t.MaxConnsPerHost = -1
	}

	return &struct {
		protocol.VNodeService
		protocol.KVService
		*ratecounter.Rate
	}{
		VNodeService: protocol.NewVNodeServiceProtobufClient("http://chord", c, twirp.WithClientHooks(injector)),
		KVService:    protocol.NewKVServiceProtobufClient("http://chord", c, twirp.WithClientHooks(injector)),
		Rate:         outboundRate,
	}
}

// DynamicTunnelClient returns a rpc client suitable for TunnelService,  with the destination set per call dynamically
// according to the destination in context. Use WithNode(ctx, node) at call site to dynamically dispatch. Optionally,
// use WithClientToken(ctx, token) to include client token.
func DynamicTunnelClient(baseContext context.Context, tunnelTransport transport.Transport) protocol.TunnelService {
	injector := &twirp.ClientHooks{
		RequestPrepared: func(ctx context.Context, r *http.Request) (context.Context, error) {
			peer, ok := ctx.Value(contextNodeKey).(*protocol.Node)
			if !ok {
				return nil, fmt.Errorf("keys.ContextNodeKey not found in context")
			}
			token, ok := ctx.Value(contextClientTokenKey).(*protocol.ClientToken)
			if ok {
				r.Header.Set("authorization", string(token.GetToken()))
			}
			r.URL.Host = peer.GetAddress() // needed to override dialer instead of using http://tunnel as key
			return ctx, nil
		},
	}

	// default to http client pooling
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConnsPerHost = 2
	t.IdleConnTimeout = time.Minute
	t.DialContext = getDynamicDialer(baseContext, tunnelTransport)
	c := &http.Client{
		Transport: t,
	}
	// disable in testing
	if disable, ok := baseContext.Value(contextDisablePoolKey).(bool); ok && disable {
		t.DisableKeepAlives = true
		t.MaxConnsPerHost = -1
	}

	return protocol.NewTunnelServiceProtobufClient("http://tunnel", c, twirp.WithClientHooks(injector))
}

// Middleware to attach the authorization header to the current request. The value can be retrieve with GetAuthorization()
func ExtractAuthorizationHeader(base http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("authorization")
		if token != "" {
			ctx := WithAuthorization(r.Context(), token)
			r = r.WithContext(ctx)
		}
		base.ServeHTTP(w, r)
	})
}

func receive(stream io.Reader, rr VTMarshaler, checker func(size uint32) bool) error {
	var sb [LengthSize]byte

	n, err := io.ReadFull(stream, sb[:])
	if err != nil {
		return fmt.Errorf("reading RPC message buffer size: %w", err)
	}
	if n != LengthSize {
		return fmt.Errorf("expected %d bytes to be read but %d bytes was read", LengthSize, n)
	}

	ms := binary.BigEndian.Uint32(sb[:])
	if !checker(ms) {
		return fmt.Errorf("RPC message is too large")
	}

	mb := pool.Get(int(ms))
	defer pool.Put(mb)

	n, err = io.ReadFull(stream, mb)
	if err != nil {
		return fmt.Errorf("reading RPC message: %w", err)
	}
	if ms != uint32(n) {
		return fmt.Errorf("expected %d bytes to be read but %d bytes was read", ms, n)
	}

	return rr.UnmarshalVT(mb)
}

func BoundedReceive(stream io.Reader, rr VTMarshaler, max uint32) error {
	return receive(stream, rr, func(size uint32) bool {
		return size <= max
	})
}

func Receive(stream io.Reader, rr VTMarshaler) error {
	return receive(stream, rr, func(size uint32) bool {
		return true
	})
}

func Send(stream io.Writer, rr VTMarshaler) error {
	l := rr.SizeVT()
	mb := pool.Get(LengthSize + l)
	defer pool.Put(mb)

	binary.BigEndian.PutUint32(mb[0:LengthSize], uint32(l))

	_, err := rr.MarshalToSizedBufferVT(mb[LengthSize:])
	if err != nil {
		return fmt.Errorf("encoding outbound RPC message: %w", err)
	}

	n, err := stream.Write(mb)
	if err != nil {
		return fmt.Errorf("sending RPC message: %w", err)
	}
	if n != LengthSize+l {
		return fmt.Errorf("expected %d bytes sent but %d bytes was sent", l, n)
	}

	return nil
}
