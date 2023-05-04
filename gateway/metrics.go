package gateway

import "expvar"

var (
	connectHostname = expvar.NewMap("gateway.connectHostname")
)
