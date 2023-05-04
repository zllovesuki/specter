package acme

import "expvar"

var (
	tlsHostname = expvar.NewMap("tls.serverName")
)
