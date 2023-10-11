package gateway

import (
	"fmt"
	"net/http"
	"strings"

	"go.miragespace.co/specter/spec/cipher"
)

func generateAltHeaders(port int) string {
	alt := []string{}
	for _, proto := range cipher.H3Protos {
		alt = append(alt, fmt.Sprintf(`%s=":%d"; ma=86400`, proto, port))
	}
	return strings.Join(alt, ", ")
}

func (g *Gateway) appendHeaders(h3 bool) func(h http.Header) {
	return func(h http.Header) {
		h.Set("server", "specter")
		h.Set("http3", fmt.Sprintf("%v", h3))
		h.Set("alt-svc", g.altHeaders)
	}
}
