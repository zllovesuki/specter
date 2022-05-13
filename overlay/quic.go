package overlay

import (
	"time"

	"github.com/lucas-clemente/quic-go"
)

var (
	quicConfig = &quic.Config{
		KeepAlive:            true,
		HandshakeIdleTimeout: time.Second * 3,
		MaxIdleTimeout:       time.Second * 5,
		EnableDatagrams:      true,
	}
)
