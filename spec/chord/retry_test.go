package chord

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type retryTestNode struct {
	VNode
	get func(context.Context, []byte) ([]byte, error)
}

func (n *retryTestNode) Get(ctx context.Context, key []byte) ([]byte, error) {
	return n.get(ctx, key)
}

func TestRetryKVPreservesAttemptsValuesAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		results []error
		wantErr error
	}{
		{name: "retry stale ownership", results: []error{ErrKVStaleOwnership, nil}},
		{name: "stop on conflict", results: []error{ErrKVSimpleConflict}, wantErr: ErrKVSimpleConflict},
		{name: "return final error", results: []error{ErrKVStaleOwnership, ErrKVPendingTransfer, ErrKVPendingTransfer}, wantErr: ErrKVPendingTransfer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			node := &retryTestNode{get: func(ctx context.Context, key []byte) ([]byte, error) {
				require.Equal(t, []byte("key"), key)
				require.NoError(t, ctx.Err())
				if calls >= len(tc.results) {
					t.Fatal("retried after the expected final attempt")
				}
				err := tc.results[calls]
				calls++
				return []byte{}, err
			}}
			value, err := WrapRetryKV(node, time.Nanosecond, 3).Get(t.Context(), []byte("key"))
			require.Equal(t, tc.wantErr, err, "errors must remain directly comparable")
			require.Equal(t, len(tc.results), calls)
			if err == nil {
				require.NotNil(t, value, "successful empty values must stay distinct from missing values")
				require.Empty(t, value)
			}
		})
	}
}

func TestRetryKVCancellationStopsBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	node := &retryTestNode{get: func(context.Context, []byte) ([]byte, error) {
		calls++
		cancel()
		return nil, ErrKVStaleOwnership
	}}
	_, err := WrapRetryKV(node, time.Hour, 3).Get(ctx, []byte("key"))
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
}
