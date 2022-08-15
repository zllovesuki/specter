package tun

import (
	"io"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
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
	var b [rpc.LengthSize]byte
	_, err = io.ReadFull(src, b[:])
	return
}
