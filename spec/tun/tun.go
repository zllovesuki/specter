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
