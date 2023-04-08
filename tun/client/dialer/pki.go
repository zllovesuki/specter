package dialer

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"kon.nect.sh/specter/spec/protocol"
)

func GetPKIClient(cfg *tls.Config, apex *ParsedApex) protocol.PKIService {
	t := http.DefaultTransport.(*http.Transport).Clone()
	cfg.NextProtos = nil
	t.TLSClientConfig = cfg
	t.DisableKeepAlives = true
	t.MaxConnsPerHost = -1
	return protocol.NewPKIServiceProtobufClient(fmt.Sprintf("https://%s", apex.String()), &http.Client{
		Transport: t,
	})
}
