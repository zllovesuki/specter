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
	mRand "math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"specter/chord"
	"specter/node"
	"specter/overlay"
	"specter/spec/protocol"

	"go.uber.org/zap"
)

var (
	listen = flag.String("listen", "127.0.0.1:1234", "transport listener")
	peer   = flag.String("peer", "local", "known peer")
)

func main() {
	flag.Parse()

	identity := &protocol.Node{
		Id:      mRand.Uint64() % (1 << chord.MaxFingerEntries),
		Address: *listen,
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

	t := overlay.NewTransport(logger, serverTLS, clientTLS)

	local := node.NewLocalNode(node.NodeConfig{
		Logger:                   logger,
		Identity:                 identity,
		Transport:                t,
		StablizeInterval:         time.Second * 3,
		FixFingerInterval:        time.Second * 5,
		PredecessorCheckInterval: time.Second * 7,
	})
	defer local.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go t.Accept(ctx, identity)
	go local.HandleRPC()

	go func() {
		if *peer == "local" {
			if err := local.Create(); err != nil {
				logger.Fatal("Start LocalNode with new Chord Ring", zap.Error(err))
			}
		} else {
			p, err := node.NewUnknownRemoteNode(ctx, t, *peer)
			if err != nil {
				logger.Fatal("Creating RemoteNode", zap.Error(err))
			}
			if err := local.Join(p); err != nil {
				logger.Fatal("Start LocalNode with existing Chord Ring", zap.Error(err))
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(time.Second * 5)
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				p, _ := local.GetPredecessor()
				if p != nil {
					logger.Debug("Predecessor", zap.Uint64("Pre", p.ID()))

				}
				logger.Debug("Ring", zap.String("ring", local.RingTrace()))
				logger.Debug("Finger", zap.String("table", local.FingerTrace()))
				local.Put([]byte("key"), []byte("value"))
			}
		}
	}()

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
