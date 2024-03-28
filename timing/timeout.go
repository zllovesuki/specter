package timing

import "time"

const (
	TLSHandshakeTimeout = time.Second * 15 // this includes on-demand certificate, if applicable
	DNSLookupTimeout    = time.Second * 3

	ChordRPCTimeout  = time.Second * 10
	ChordPingTimeout = time.Second * 3
)
