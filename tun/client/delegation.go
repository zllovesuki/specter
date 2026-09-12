package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"

	"github.com/twitchtv/twirp"
)

var ErrDelegationUnsupported = errors.New("server does not support domain tokens")

var ErrMintAmbiguous = errors.New("mint outcome is ambiguous; list grants before retrying")

func unsupported(err error) bool {
	var te twirp.Error
	return errors.As(err, &te) && te.Code() == twirp.BadRoute
}

func definite(err error) bool {
	var local *attachmentError
	if errors.As(err, &local) {
		return true
	}
	var te twirp.Error
	if !errors.As(err, &te) {
		return false
	}
	switch te.Code() {
	case twirp.InvalidArgument, twirp.PermissionDenied, twirp.Unauthenticated, twirp.NotFound, twirp.Malformed, twirp.Unimplemented:
		return true
	}
	return false
}

func delegationCall[V any](c *Client, ctx context.Context, retryable bool, fn func(context.Context) (V, error)) (resp V, err error) {
	nodes := c.getConnectedNodes()
	if len(nodes) == 0 {
		return resp, fmt.Errorf("no rpc candidates available")
	}
	var last error
	for _, node := range nodes {
		if ctx.Err() != nil {
			return resp, ctx.Err()
		}
		callCtx, cancel := context.WithTimeout(rpc.WithNode(ctx, node), rpcTimeout)
		resp, err = fn(callCtx)
		cancel()
		var rpcError twirp.Error
		// The owner's grant quota is a definite refusal; attachment capacity is
		// still retryable for lightweight clients on a different server.
		quota := errors.As(err, &rpcError) && rpcError.Code() == twirp.ResourceExhausted
		if err == nil || definite(err) || quota {
			return resp, err
		}
		if unsupported(err) {
			continue
		}
		if !retryable {
			return resp, fmt.Errorf("%w: %w", ErrMintAmbiguous, err)
		}
		last = err
	}
	if last != nil {
		return resp, last
	}
	return resp, ErrDelegationUnsupported
}

func (c *Client) MintDelegation(ctx context.Context, hostname string, expiresAt time.Time) (*protocol.MintDelegationResponse, error) {
	var expiry int64
	if !expiresAt.IsZero() {
		expiry = expiresAt.Unix()
	}
	return delegationCall(c, ctx, false, func(ctx context.Context) (*protocol.MintDelegationResponse, error) {
		return c.tunnelClient.MintDelegation(ctx, &protocol.MintDelegationRequest{
			Hostname:  hostname,
			ExpiresAt: expiry,
		})
	})
}

func (c *Client) ListDelegations(ctx context.Context) (*protocol.ListDelegationsResponse, error) {
	return delegationCall(c, ctx, true, func(ctx context.Context) (*protocol.ListDelegationsResponse, error) {
		return c.tunnelClient.ListDelegations(ctx, &protocol.ListDelegationsRequest{})
	})
}

func (c *Client) RevokeDelegation(ctx context.Context, id string) (*protocol.RevokeDelegationResponse, error) {
	return delegationCall(c, ctx, true, func(ctx context.Context) (*protocol.RevokeDelegationResponse, error) {
		return c.tunnelClient.RevokeDelegation(ctx, &protocol.RevokeDelegationRequest{Id: id})
	})
}
