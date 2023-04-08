package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"kon.nect.sh/specter/spec/pki"
	"kon.nect.sh/specter/tun/client"

	"gopkg.in/yaml.v3"
)

type V1ClientConfig struct {
	Apex     string          `yaml:"apex" json:"apex"`
	Token    string          `yaml:"token,omitempty"`
	ClientID uint64          `yaml:"clientId,omitempty"`
	Tunnels  []client.Tunnel `yaml:"tunnels,omitempty"`
}

var (
	runOpenSSL = flag.Bool("text", false, "run OpenSSL to print the certificate information")
	configPath = flag.String("config", "specter.yaml", "path to specter.yaml before PKI")
	certsDir   = flag.String("certs", "certs", "path to directory containing client-ca.crt and client-ca.key")
)

func main() {
	flag.Parse()

	oldCfg := V1ClientConfig{}

	f, err := os.Open(*configPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	err = yaml.NewDecoder(f).Decode(&oldCfg)
	if err != nil {
		panic(err)
	}

	ca, err := tls.LoadX509KeyPair(
		filepath.Join(*certsDir, "client-ca.crt"),
		filepath.Join(*certsDir, "client-ca.key"),
	)
	if err != nil {
		panic(err)
	}

	certPubKey, keyPem := pki.GeneratePrivKey()

	certBytes, err := pki.GenerateCertificate(ca, pki.IdentityRequest{
		PublicKey: certPubKey,
		Subject:   pki.MakeSubjectV1(oldCfg.ClientID, oldCfg.Token),
	})

	if err != nil {
		panic(err)
	}

	certPem := pki.MarshalCertificate(certBytes)

	newCfg := client.Config{
		Apex:        oldCfg.Apex,
		Certificate: string(certPem),
		PrivKey:     keyPem,
		Tunnels:     oldCfg.Tunnels,
	}

	if *runOpenSSL {
		openssl := exec.Command("openssl", "x509", "-noout", "-text")
		openssl.Stdin = strings.NewReader(newCfg.Certificate)
		var out bytes.Buffer
		openssl.Stdout = &out

		err = openssl.Run()
		if err != nil {
			panic(err)
		}

		fmt.Printf("%+v\n", out.String())
	}

	err = yaml.NewEncoder(os.Stdout).Encode(&newCfg)
	if err != nil {
		panic(err)
	}
}
