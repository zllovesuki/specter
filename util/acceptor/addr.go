package acceptor

import "net"

var emptyAddr = &nilAddr{}

type nilAddr struct {
}

var _ net.Addr = (*nilAddr)(nil)

func (nilAddr) Network() string {
	return "nil"
}

func (nilAddr) String() string {
	return "nil"
}
