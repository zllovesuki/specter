package acme

import "expvar"

var (
	acmeHostname = expvar.NewMap("acme.serverName")
)
