package gateway

import "net/http"

type proxyRoundTripper struct {
	h1 http.RoundTripper
	h2 http.RoundTripper
}

var _ http.RoundTripper = (*proxyRoundTripper)(nil)

func (r *proxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.ProtoAtLeast(2, 0) {
		return r.h2.RoundTrip(req)
	} else {
		return r.h1.RoundTrip(req)
	}
}
