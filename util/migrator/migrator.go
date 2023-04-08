package migrator

import (
	"bytes"
	"crypto/tls"
	"net/http"

	"kon.nect.sh/specter/spec/pki"
	"kon.nect.sh/specter/tun/client"

	"gopkg.in/yaml.v3"
)

type v1ClientConfig struct {
	Apex     string          `yaml:"apex" json:"apex"`
	Token    string          `yaml:"token,omitempty"`
	ClientID uint64          `yaml:"clientId,omitempty"`
	Tunnels  []client.Tunnel `yaml:"tunnels,omitempty"`
}

func ConfigMigrator(ca tls.Certificate) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v1Cfg := v1ClientConfig{}

		err := yaml.NewDecoder(r.Body).Decode(&v1Cfg)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		certPubKey, keyPem := pki.GeneratePrivKey()

		certBytes, err := pki.GenerateCertificate(ca, pki.IdentityRequest{
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
		err = yaml.NewEncoder(&out).Encode(&newCfg)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		w.Header().Set("content-type", "application/yaml; charset=utf-8")
		w.WriteHeader(200)
		w.Write(out.Bytes())
	}
}
