package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"

	"kon.nect.sh/specter/overlay"
	chordSpec "kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/tun/client"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Build = "head"
)

func main() {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	logger, err := config.Build()
	if err != nil {
		panic(err)
	}

	logger.Info("specter client", zap.String("build", Build))

	self := &protocol.Node{
		Id: chordSpec.Random(),
	}

	seed := &protocol.Node{
		Address: "127.0.0.1:4242",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clientTLSConf := generateTLSConfig()
	clientTLSConf.NextProtos = []string{
		tun.ALPN(protocol.Link_SPECTER_TUN),
	}
	transport := overlay.NewQUIC(overlay.TransportConfig{
		Logger:    logger,
		Endpoint:  self,
		ServerTLS: nil,
		ClientTLS: clientTLSConf,
	})
	defer transport.Stop()

	c, err := client.NewClient(ctx, logger, transport, seed)
	if err != nil {
		logger.Fatal("starting new tun client", zap.Error(err))
	}

	nodes, err := c.GetCandidates(ctx)
	if err != nil {
		logger.Fatal("starting new tun client", zap.Error(err))
	}

	for _, node := range nodes {
		fmt.Printf("%+v\n", node)
	}

	for _, node := range nodes[1:] {
		transport.DialDirect(ctx, node)
	}

	hostname, err := c.PublishTunnel(ctx, nodes)
	if err != nil {
		logger.Fatal("publishing tunnel", zap.Error(err))
	}

	logger.Debug("tunnel published", zap.String("hostname", hostname))

	c.Tunnel(ctx, hostname)
}

func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{tlsCert},
		NextProtos:         []string{"quic-echo-example"},
	}
}
