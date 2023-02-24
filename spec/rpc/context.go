package rpc

import (
	"context"
	"encoding/base64"
	"net/http"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"

	pool "github.com/libp2p/go-buffer-pool"
)

const (
	HeaderRPCContext = "x-rpc-context"
)

type rpcContextKey string

const (
	contextNodeKey           = rpcContextKey("dial-node")         // *protocol.Node to connect
	contextRPCContextKey     = rpcContextKey("rpc-context")       // *protocol.Context of the rpc request
	contextAuthorizationKey  = rpcContextKey("auth-header")       // "Authorization" header from client rpc request
	contextClientTokenKey    = rpcContextKey("client-token")      // *protocol.ClientToken from client or parsed from the header
	contextClientIdentityKey = rpcContextKey("client-identity")   // *protocol.Node of client as matched with delegation and token
	contextDelegationKey     = rpcContextKey("stream-delegation") // *transport.StreamDelegation of the rpc request
	contextDisablePoolKey    = rpcContextKey("disable-http-pool") // disable HTTP client pooling. Used in test to avoid lingering connections
)

// Disable HTTP client pool for this client
func DisablePooling(baseCtx context.Context) context.Context {
	return context.WithValue(baseCtx, contextDisablePoolKey, true)
}

// Connect to the provided node in this request
func WithNode(ctx context.Context, node *protocol.Node) context.Context {
	return context.WithValue(ctx, contextNodeKey, node)
}

// Retrieve the node of this request
func GetNode(ctx context.Context) *protocol.Node {
	if node, ok := ctx.Value(contextNodeKey).(*protocol.Node); ok {
		return node
	}
	return nil
}

// Send RPC context for this request
func WithContext(ctx context.Context, rpcCtx *protocol.Context) context.Context {
	return context.WithValue(ctx, contextRPCContextKey, rpcCtx)
}

// Retrieve the RPC context of this request
func GetContext(ctx context.Context) *protocol.Context {
	if r, ok := ctx.Value(contextRPCContextKey).(*protocol.Context); ok {
		return r
	}
	return &protocol.Context{}
}

// Make request with the provided authorization header
func WithAuthorization(ctx context.Context, authHeader string) context.Context {
	return context.WithValue(ctx, contextAuthorizationKey, authHeader)
}

// Retrieve the authorization header of this request
func GetAuthorization(ctx context.Context) string {
	if auth, ok := ctx.Value(contextAuthorizationKey).(string); ok {
		return auth
	}
	return ""
}

// Attach client token for this request
func WithClientToken(ctx context.Context, token *protocol.ClientToken) context.Context {
	return context.WithValue(ctx, contextClientTokenKey, token)
}

// Retrieve client token of this request
func GetClientToken(ctx context.Context) *protocol.ClientToken {
	if token, ok := ctx.Value(contextClientTokenKey).(*protocol.ClientToken); ok {
		return token
	}
	return nil
}

// Attach client identity for this request
func WithCientIdentity(ctx context.Context, id *protocol.Node) context.Context {
	return context.WithValue(ctx, contextClientIdentityKey, id)
}

// Retrieve client identity of this request
func GetClientIdentity(ctx context.Context) *protocol.Node {
	if id, ok := ctx.Value(contextClientIdentityKey).(*protocol.Node); ok {
		return id
	}
	return nil
}

// Attach the delegation triggering the request
func WithDelegation(ctx context.Context, delegate *transport.StreamDelegate) context.Context {
	return context.WithValue(ctx, contextDelegationKey, delegate)
}

// Retrieve the delegation of this request
func GetDelegation(ctx context.Context) *transport.StreamDelegate {
	if delegate, ok := ctx.Value(contextDelegationKey).(*transport.StreamDelegate); ok {
		return delegate
	}
	return nil
}

// Serialize RPC context as http headers
func SerializeContextHeader(ctx context.Context, r http.Header) {
	rCtx, ok := ctx.Value(contextRPCContextKey).(*protocol.Context)
	if !ok {
		return
	}

	l := rCtx.SizeVT()
	mb := pool.Get(l)
	defer pool.Put(mb)

	_, err := rCtx.MarshalToSizedBufferVT(mb)
	if err != nil {
		return
	}

	r.Set(HeaderRPCContext, base64.StdEncoding.EncodeToString(mb))
}

// Deserialize RPC context from http headers. The RPC context can be retrieved with GetContext()
func DeserializeContextHeader(ctx context.Context, r http.Header) (context.Context, bool) {
	encoded := r.Get(HeaderRPCContext)
	if len(encoded) < 1 {
		return ctx, false
	}

	mb, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ctx, false
	}

	rCtx := &protocol.Context{}
	if err := rCtx.UnmarshalVT(mb); err != nil {
		return ctx, false
	}

	return WithContext(ctx, rCtx), true
}

// Middleware to attach the deserialized RPC context to the current request
func ExtractContext(base http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, ok := DeserializeContextHeader(r.Context(), r.Header)
		if ok {
			r = r.WithContext(ctx)
		}
		base.ServeHTTP(w, r)
	})
}
