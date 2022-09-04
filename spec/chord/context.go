package chord

import (
	"context"

	"kon.nect.sh/specter/spec/protocol"
)

type requestCtxType string

const requestCtxKey requestCtxType = "rpcRequestCtx"

func WithRequestContext(ctx context.Context, reqCtx *protocol.Context) context.Context {
	return context.WithValue(ctx, requestCtxKey, reqCtx)
}

func GetRequestContext(ctx context.Context) *protocol.Context {
	reqCtx, ok := ctx.Value(requestCtxKey).(*protocol.Context)
	if !ok {
		return &protocol.Context{}
	}
	return reqCtx
}
