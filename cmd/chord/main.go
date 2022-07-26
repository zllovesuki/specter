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

	"kon.nect.sh/specter/chord"
	"kon.nect.sh/specter/kv"
	"kon.nect.sh/specter/overlay"
	chordSpec "kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"go.uber.org/zap"
)

var (
	listen = flag.String("chord", "127.0.0.1:1234", "chord transport listener")
	peer   = flag.String("peer", "local", "known peer")
)

func main() {
	flag.Parse()

	identity := &protocol.Node{
		Id:      chordSpec.Random(),
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t := overlay.NewQUIC(overlay.TransportConfig{
		Logger:    logger,
		Endpoint:  identity,
		ServerTLS: serverTLS,
		ClientTLS: clientTLS,
	})
	defer t.Stop()

	local := chord.NewLocalNode(chord.NodeConfig{
		Logger:                   logger,
		Identity:                 identity,
		Transport:                t,
		KVProvider:               kv.WithChordHash(),
		FixFingerInterval:        time.Second * 3,
		StablizeInterval:         time.Second * 5,
		PredecessorCheckInterval: time.Second * 7,
	})
	defer local.Stop()

	go t.Accept(ctx)
	go local.HandleRPC(ctx)

	go func() {
		if *peer == "local" {
			if err := local.Create(); err != nil {
				logger.Fatal("Start LocalNode with new Chord Ring", zap.Error(err))
			}
		} else {
			p, err := chord.NewRemoteNode(ctx, t, logger, &protocol.Node{
				Unknown: true,
				Address: *peer,
			})
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
				var pID int64 = -1
				if p != nil {
					pID = int64(p.ID())
				}
				logger.Debug("Debug Log",
					zap.Uint64("node", local.ID()),
					zap.Int64("predecessor", pID),
					// zap.String("ring", local.RingTrace()),
					zap.String("table", local.FingerTrace()))
				// local.Put([]byte("key"), []byte("value"))
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
