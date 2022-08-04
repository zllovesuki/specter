package gateway

import (
	"fmt"
	"net/http"
)

func (g *Gateway) appendHeaders(h3 bool) func(h http.Header) {
	return func(h http.Header) {
		h.Set("server", "specter")
		h.Set("http3", fmt.Sprintf("%v", h3))
		h.Set("alt-svc", fmt.Sprintf(`%s=":%d"; ma=2592000`, "h3", g.GatewayPort))
	}
}
