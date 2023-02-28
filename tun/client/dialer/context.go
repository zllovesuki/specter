package dialer

import (
	"context"
)

type dialerContextKey string

const (
	contextServerNameOverride = dialerContextKey("server-name-override")
)

func WithServerNameOverride(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, contextServerNameOverride, name)
}

func GetServerNameOverride(ctx context.Context) string {
	if name, ok := ctx.Value(contextServerNameOverride).(string); ok {
		return name
	}
	return ""
}
