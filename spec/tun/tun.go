package tun

import (
	"context"
	"errors"
	"io"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
)

const (
	NumRedundantLinks = 3
)

func IsNoDirect(err error) bool {
	return errors.Is(err, transport.ErrNoDirect) ||
		errors.Is(err, ErrTunnelClientNotConnected)
}

func SendStatusProto(dest io.Writer, err error) {
	status := &protocol.TunnelStatus{}
	if err != nil {
		if IsNoDirect(err) {
			status.Status = protocol.TunnelStatusCode_NO_DIRECT
		} else {
			status.Status = protocol.TunnelStatusCode_UNKNOWN_ERROR
		}
		status.Error = err.Error()
	}
	rpc.Send(dest, status)
}

func DrainStatusProto(src io.Reader) (err error) {
	var b [rpc.LengthSize]byte
	_, err = io.ReadFull(src, b[:])
	return
}

func SaveCustomHostname(ctx context.Context, kv chord.KV, hostname string, bundle *protocol.CustomHostname) (err error) {
	data, err := bundle.MarshalVT()
	if err != nil {
		return nil
	}

	err = kv.Put(ctx, []byte(CustomHostnameKey(hostname)), data)
	if err != nil {
		return err
	}

	return nil
}

func FindCustomHostname(ctx context.Context, kv chord.KV, hostname string) (*protocol.CustomHostname, error) {
	val, err := kv.Get(ctx, []byte(CustomHostnameKey(hostname)))
	if err != nil {
		return nil, err
	}

	if len(val) == 0 {
		return nil, ErrHostnameNotFound
	}

	bundle := &protocol.CustomHostname{}

	if err := bundle.UnmarshalVT(val); err != nil {
		return nil, err
	}

	return bundle, nil
}

func RemoveCustomHostname(ctx context.Context, kv chord.KV, hostname string) error {
	return kv.Delete(ctx, []byte(CustomHostnameKey(hostname)))
}
