package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

var (
	caCommonName = flag.String("cn", "specter client ca", "CommonName of the client CA")
	certsDir     = flag.String("certs", "certs", "path to directory to store client-ca.crt and client-ca.key")
)

func main() {
	flag.Parse()

	caPubKey, caPrivKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}

	sn := new(big.Int).SetInt64(time.Now().UnixNano())

	cert := &x509.Certificate{
		Subject: pkix.Name{
			CommonName: *caCommonName,
		},
		SerialNumber:          sn,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, cert, caPubKey, caPrivKey)
	if err != nil {
		panic(err)
	}

	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	fmt.Printf("%+s\n", certPEM.Bytes())
	err = os.WriteFile(filepath.Join(*certsDir, "client-ca.crt"), certPEM.Bytes(), 0644)
	if err != nil {
		panic(err)
	}

	x509PrivKey, err := x509.MarshalPKCS8PrivateKey(caPrivKey)
	if err != nil {
		panic(err)
	}
	certPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509PrivKey,
	})

	fmt.Printf("%+s\n", certPrivKeyPEM.Bytes())
	err = os.WriteFile(filepath.Join(*certsDir, "client-ca.key"), certPrivKeyPEM.Bytes(), 0644)
	if err != nil {
		panic(err)
	}
}
