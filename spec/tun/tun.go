package tun

import (
	"errors"
	"io"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
)

const (
	NumRedundantLinks = 3
)

func SendStatusProto(dest io.Writer, err error) {
	status := &protocol.TunnelStatus{}
	if err != nil {
		if errors.Is(err, ErrTunnelClientNotConnected) ||
			errors.Is(err, transport.ErrNoDirect) {
			status.Status = protocol.TunnelStatusCode_NO_DIRECT
		} else {
			status.Status = protocol.TunnelStatusCode_UKNOWN_ERROR
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
