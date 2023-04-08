package migrator

import (
	"bytes"
	"crypto/tls"
	_ "embed"
	"net/http"

	"kon.nect.sh/specter/spec/pki"
	"kon.nect.sh/specter/tun/client"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type v1ClientConfig struct {
	Apex     string          `yaml:"apex" json:"apex"`
	Token    string          `yaml:"token,omitempty"`
	ClientID uint64          `yaml:"clientId,omitempty"`
	Tunnels  []client.Tunnel `yaml:"tunnels,omitempty"`
}

//go:embed helper.html
var helper []byte

func ConfigMigrator(logger *zap.Logger, ca tls.Certificate) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("content-type", "text/html; charset=utf-8")
			w.Write(helper)
		case http.MethodPost:
			migrateConfig(logger, ca, w, r)
		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
	}
}

func migrateConfig(logger *zap.Logger, ca tls.Certificate, w http.ResponseWriter, r *http.Request) {
	v1Cfg := v1ClientConfig{}

	err := yaml.NewDecoder(r.Body).Decode(&v1Cfg)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	if v1Cfg.ClientID == 0 || len(v1Cfg.Token) != 44 {
		http.Error(w, "provided yaml is not a valid v1 config", 400)
		return
	}

	certPubKey, keyPem := pki.GeneratePrivKey()

	certBytes, err := pki.GenerateCertificate(logger, ca, pki.IdentityRequest{
		PublicKey: certPubKey,
		Subject:   pki.MakeSubjectV1(v1Cfg.ClientID, v1Cfg.Token),
	})

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	certPem := pki.MarshalCertificate(certBytes)

	newCfg := client.Config{
		Version:     2,
		Apex:        v1Cfg.Apex,
		Certificate: string(certPem),
		PrivKey:     keyPem,
		Tunnels:     v1Cfg.Tunnels,
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	err = encoder.Encode(&newCfg)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("content-type", "application/yaml; charset=utf-8")
	w.WriteHeader(200)
	w.Write(out.Bytes())
}
