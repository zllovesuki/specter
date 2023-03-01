package connector

import (
	"fmt"
	"io"
	"net"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/tun/client/dialer"

	"go.uber.org/zap"
)

func statusExchange(rw io.ReadWriter) (*protocol.TunnelStatus, error) {
	// because of quic's early connection, the client need to "poke" the gateway before
	// the gateway can actually accept a stream, despite .OpenStreamSync
	status := &protocol.TunnelStatus{}
	err := rpc.Send(rw, status)
	if err != nil {
		return nil, fmt.Errorf("error sending status check: %w", err)
	}
	status.Reset()
	err = rpc.Receive(rw, status)
	if err != nil {
		return nil, fmt.Errorf("error receiving checking status: %w", err)
	}
	return status, nil
}

func GetConnection(d dialer.TransportDialer) (net.Conn, error) {
	rw, err := d.Dial()
	if err != nil {
		return nil, err
	}
	status, err := statusExchange(rw)
	if err != nil {
		return nil, err
	}
	if status.GetStatus() != protocol.TunnelStatusCode_STATUS_OK {
		return nil, fmt.Errorf("error opening tunnel: %s", status.Error)
	}
	return rw, nil
}

func HandleConnections(logger *zap.Logger, listener net.Listener, d dialer.TransportDialer) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func(local *net.TCPConn) {
			r, err := GetConnection(d)
			if err != nil {
				logger.Error("Error forwarding local connections via specter gateway", zap.Error(err))
				local.Close()
				return
			}

			logger.Info("Forwarding incoming connection", zap.String("local", conn.RemoteAddr().String()), zap.String("via", d.Remote().String()))

			go func() {
				errChan := tun.Pipe(r, local)
				for err := range errChan {
					logger.Error("error piping to target", zap.Error(err))
				}
			}()
		}(conn.(*net.TCPConn))
	}
}
