package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"math/big"
	"os"
	"os/signal"
	"syscall"
	"time"

	"specter/kv"
	"specter/node"
	"specter/overlay"
	"specter/spec/chord"
	"specter/spec/protocol"
	"specter/tun/server"

	"go.uber.org/zap"
)

var (
	listenChord  = flag.String("chord", "127.0.0.1:1234", "chord transport listener")
	listenClient = flag.String("client", "127.0.0.1:1235", "client transport listener")
	peer         = flag.String("peer", "local", "known chord peer")
)

func main() {
	flag.Parse()

	chordIdentity := &protocol.Node{
		Id:      chord.Random(),
		Address: *listenChord,
	}
	serverIdentity := &protocol.Node{
		Id:      chord.Random(),
		Address: *listenClient,
	}

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	serverTLS := generateTLSConfig()
	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chordTransport := overlay.NewQUIC(logger, chordIdentity, serverTLS, clientTLS)
	defer chordTransport.Stop()

	clientTransport := overlay.NewQUIC(logger, serverIdentity, serverTLS, clientTLS)
	defer clientTransport.Stop()

	chordNode := node.NewLocalNode(node.NodeConfig{
		Logger:                   logger,
		Identity:                 chordIdentity,
		Transport:                chordTransport,
		KVProvider:               kv.WithChordHash(),
		FixFingerInterval:        time.Second * 3,
		StablizeInterval:         time.Second * 5,
		PredecessorCheckInterval: time.Second * 7,
	})
	defer chordNode.Stop()

	tunServer := server.New(logger, chordNode, clientTransport, chordTransport)

	go chordTransport.Accept(ctx)
	go chordNode.HandleRPC()

	if *peer == "local" {
		if err := chordNode.Create(); err != nil {
			logger.Fatal("Start LocalNode with new Chord Ring", zap.Error(err))
		}
	} else {
		p, err := node.NewRemoteNode(ctx, chordTransport, logger, &protocol.Node{
			Unknown: true,
			Address: *peer,
		})
		if err != nil {
			logger.Fatal("Creating RemoteNode", zap.Error(err))
		}
		if err := chordNode.Join(p); err != nil {
			logger.Fatal("Start LocalNode with existing Chord Ring", zap.Error(err))
		}
	}

	go tunServer.Accept(ctx)
	go clientTransport.Accept(ctx)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	<-sigs
}

func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
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
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quic-echo-example"},
	}
}
