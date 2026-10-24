package client

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/waf"

	"github.com/twitchtv/twirp"
)

const maxHostWAFRequestBytes = 64 << 10

func (c *Client) hostWAFCall(ctx context.Context, write bool, call func(context.Context) (*protocol.HostWAFResponse, error)) (*protocol.HostWAFResponse, error) {
	nodes := c.getConnectedNodes()
	if len(nodes) == 0 {
		return nil, twirp.Unavailable.Error("no connected servers are available")
	}
	var last error
	for _, node := range nodes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		callCtx, cancel := context.WithTimeout(rpc.WithNode(ctx, node), rpcTimeout)
		response, err := call(callCtx)
		cancel()
		if err == nil {
			policy, err := waf.ValidatePolicy(response.GetPolicy())
			if err != nil {
				return nil, twirp.Unavailable.Errorf("server returned an unsupported WAF policy: %v", err)
			}
			response.Policy = policy
			return response, nil
		}
		var rpcError twirp.Error
		if errors.As(err, &rpcError) && (rpcError.Code() == twirp.BadRoute || rpcError.Code() == twirp.Unimplemented) {
			continue
		}
		// Reads can use another connected server. An uncertain save keeps its
		// original version guard and is left for the owner to refresh or retry.
		if write || definite(err) {
			return nil, err
		}
		last = err
	}
	if last != nil {
		return nil, last
	}
	return nil, twirp.Unimplemented.Error("connected servers do not support web protection")
}

func (c *Client) GetHostWAF(ctx context.Context, hostname string) (*protocol.HostWAFResponse, error) {
	if hostname == "" {
		return nil, twirp.InvalidArgument.Error("hostname is required")
	}
	return c.hostWAFCall(ctx, false, func(ctx context.Context) (*protocol.HostWAFResponse, error) {
		return c.tunnelClient.GetHostWAF(ctx, &protocol.GetHostWAFRequest{Hostname: hostname})
	})
}

func (c *Client) SetHostWAF(ctx context.Context, hostname, expectedVersion string, policy *protocol.WAFPolicy) (*protocol.HostWAFResponse, error) {
	if hostname == "" {
		return nil, twirp.InvalidArgument.Error("hostname is required")
	}
	policy, err := waf.ValidatePolicy(policy)
	if err != nil {
		return nil, twirp.InvalidArgument.Error(err.Error())
	}
	request := &protocol.SetHostWAFRequest{
		Hostname: hostname, ExpectedVersion: expectedVersion,
		Policy: policy,
	}
	return c.hostWAFCall(ctx, true, func(ctx context.Context) (*protocol.HostWAFResponse, error) {
		return c.tunnelClient.SetHostWAF(ctx, request)
	})
}

func (c *Client) getHostWAFHandler(w http.ResponseWriter, r *http.Request) {
	response, err := c.GetHostWAF(r.Context(), r.URL.Query().Get("hostname"))
	if err != nil {
		writeHostWAFError(w, err)
		return
	}
	writeHostWAFResult(w, response)
}

func (c *Client) setHostWAFHandler(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeHostWAFError(w, twirp.InvalidArgument.Error("send WAF settings as application/json"))
		return
	}
	var body protocol.SetHostWAFRequest
	if err := waf.DecodeJSON(r.Body, &body); err != nil || body.Policy == nil {
		writeHostWAFError(w, twirp.InvalidArgument.Error("invalid WAF settings request"))
		return
	}
	response, err := c.SetHostWAF(r.Context(), r.URL.Query().Get("hostname"), body.ExpectedVersion, body.Policy)
	if err != nil {
		writeHostWAFError(w, err)
		return
	}
	writeHostWAFResult(w, response)
}

func writeHostWAFResult(w http.ResponseWriter, response *protocol.HostWAFResponse) {
	data, err := waf.MarshalJSON(response)
	if err != nil {
		writeHostWAFError(w, twirp.InternalErrorWith(err))
		return
	}
	writeJSONResult(w, http.StatusOK, json.RawMessage(data))
}

func writeHostWAFError(w http.ResponseWriter, err error) {
	status, code := http.StatusServiceUnavailable, twirp.Unavailable
	var rpcError twirp.Error
	if errors.As(err, &rpcError) {
		code = rpcError.Code()
		switch code {
		case twirp.FailedPrecondition:
			status = http.StatusConflict
		case twirp.BadRoute:
			status = http.StatusNotImplemented
		case twirp.InvalidArgument, twirp.Malformed, twirp.PermissionDenied, twirp.Unauthenticated,
			twirp.NotFound, twirp.AlreadyExists, twirp.Unimplemented, twirp.ResourceExhausted:
			status = twirp.ServerHTTPStatusFromErrorCode(code)
		}
	}
	writeJSONResult(w, status, struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}{string(code), err.Error()})
}
