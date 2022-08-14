package tun

import (
	"io"

	"kon.nect.sh/specter/rpc"
	"kon.nect.sh/specter/spec/protocol"
)

const (
	NumRedundantLinks = 3
)

func SendStatusProto(dest io.Writer, err error) {
	status := &protocol.TunnelStatus{
		Ok: true,
	}
	if err != nil {
		status.Ok = false
		status.Error = err.Error()
	}
	rpc.Send(dest, status)
}

func DrainStatusProto(src io.Reader) (err error) {
	b := make([]byte, 8)
	_, err = io.ReadFull(src, b)
	return
}
