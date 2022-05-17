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
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zllovesuki/specter/kv"
	"github.com/zllovesuki/specter/node"
	"github.com/zllovesuki/specter/overlay"
	"github.com/zllovesuki/specter/spec/chord"
	"github.com/zllovesuki/specter/spec/protocol"
	"github.com/zllovesuki/specter/spec/tun"
	"github.com/zllovesuki/specter/tun/gateway"
	"github.com/zllovesuki/specter/tun/server"

	"go.uber.org/zap"
)

var (
	listenGateway = flag.String("gw", "127.0.0.1:1233", "gateway listener")
	listenChord   = flag.String("chord", "127.0.0.1:1234", "chord transport listener")
	listenClient  = flag.String("client", "127.0.0.1:1235", "client transport listener")
	peer          = flag.String("peer", "local", "known chord peer")
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

	// ========== TODO: THESE TLS CONFIGS ARE FOR DEVELOPMENT ONLY ==========

	serverTLS := generateTLSConfig()
	clientTLS := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}

	gwTLSConf := generateTLSConfig()
	gwTLSConf.NextProtos = []string{
		tun.ALPN(protocol.Link_HTTP),
		tun.ALPN(protocol.Link_TCP),
		tun.ALPN(protocol.Link_UNKNOWN),
	}
	tunTLSConf := generateTLSConfig()
	tunTLSConf.NextProtos = []string{
		tun.ALPN(protocol.Link_SPECTER),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gwListener, err := tls.Listen("tcp", *listenGateway, gwTLSConf)
	if err != nil {
		logger.Fatal("gateway listener", zap.Error(err))
	}
	defer gwListener.Close()

	gwPort := gwListener.Addr().(*net.TCPAddr).Port
	rootDomain := "example.com"

	chordLogger := logger.With(zap.String("component", "chord"))
	tunLogger := logger.With(zap.String("component", "tun"))
	gwLogger := logger.With(zap.String("component", "gateway"))

	chordTransport := overlay.NewQUIC(chordLogger, chordIdentity, serverTLS, clientTLS)
	defer chordTransport.Stop()

	clientTransport := overlay.NewQUIC(tunLogger, serverIdentity, tunTLSConf, clientTLS)
	defer clientTransport.Stop()

	chordNode := node.NewLocalNode(node.NodeConfig{
		Logger:                   chordLogger,
		Identity:                 chordIdentity,
		Transport:                chordTransport,
		KVProvider:               kv.WithChordHash(),
		FixFingerInterval:        time.Second * 3,
		StablizeInterval:         time.Second * 5,
		PredecessorCheckInterval: time.Second * 7,
	})
	defer chordNode.Stop()

	tunServer := server.New(tunLogger, chordNode, clientTransport, chordTransport, rootDomain)

	gw, err := gateway.New(gateway.GatewayConfig{
		Logger:      gwLogger,
		Tun:         tunServer,
		Listener:    gwListener,
		RootDomain:  rootDomain,
		GatewayPort: gwPort,
	})
	if err != nil {
		logger.Fatal("gateway", zap.Error(err))
	}

	go chordTransport.Accept(ctx)
	go chordNode.HandleRPC()

	if *peer == "local" {
		if err := chordNode.Create(); err != nil {
			logger.Fatal("Start LocalNode with new Chord Ring", zap.Error(err))
		}
	} else {
		p, err := node.NewRemoteNode(ctx, chordTransport, chordLogger, &protocol.Node{
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

	go clientTransport.Accept(ctx)
	go tunServer.Accept(ctx)
	go gw.Start(ctx)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	<-sigs
}

func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
	}
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
