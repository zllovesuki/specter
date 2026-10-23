package server

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"go.miragespace.co/specter/acme"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/cipher"

	"go.uber.org/zap"
)

// CertificateEnvironment contains the base64-encoded certificate values supplied by the caller.
type CertificateEnvironment struct {
	CA          string
	Node        string
	NodeKey     string
	ClientCA    string
	ClientCAKey string
}

type certBundle struct {
	ca           *x509.CertPool
	clientCa     *x509.CertPool
	clientCaCert tls.Certificate
	node         tls.Certificate
}

func certLoaderFilesystem(dir string) (*certBundle, error) {
	files := []string{"ca.crt", "client-ca.crt", "client-ca.key", "node.crt", "node.key"}
	for i, name := range files {
		files[i] = filepath.Join(dir, name)
	}
	caCert, err := os.ReadFile(files[0])
	if err != nil {
		return nil, fmt.Errorf("reading ca cert from file: %w", err)
	}
	clientCaCert, err := os.ReadFile(files[1])
	if err != nil {
		return nil, fmt.Errorf("reading client ca cert from file: %w", err)
	}
	clientCaKey, err := os.ReadFile(files[2])
	if err != nil {
		return nil, fmt.Errorf("reading client ca key from file: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("unable to use provided ca bundle")
	}
	clientCaCertPool := x509.NewCertPool()
	if ok := clientCaCertPool.AppendCertsFromPEM(clientCaCert); !ok {
		return nil, fmt.Errorf("unable to use provided client ca bundle")
	}
	node, err := tls.LoadX509KeyPair(files[3], files[4])
	if err != nil {
		return nil, fmt.Errorf("creating node cert/key from files: %w", err)
	}
	clientCa, err := tls.X509KeyPair(clientCaCert, clientCaKey)
	if err != nil {
		return nil, fmt.Errorf("creating client ca cert/key from files: %w", err)
	}
	return &certBundle{
		ca:           caCertPool,
		clientCa:     clientCaCertPool,
		clientCaCert: clientCa,
		node:         node,
	}, nil
}

func certLoaderEnv(environment *CertificateEnvironment) (*certBundle, error) {
	caCert, err := base64.StdEncoding.DecodeString(environment.CA)
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_CA: %w", err)
	}
	nodeCert, err := base64.StdEncoding.DecodeString(environment.Node)
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_NODE: %w", err)
	}
	nodeKey, err := base64.StdEncoding.DecodeString(environment.NodeKey)
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_NODE_KEY: %w", err)
	}
	clientCaCert, err := base64.StdEncoding.DecodeString(environment.ClientCA)
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_NODE: %w", err)
	}
	clientCaKey, err := base64.StdEncoding.DecodeString(environment.ClientCAKey)
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_NODE_KEY: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("unable to use provided ca bundle")
	}
	clientCaCertPool := x509.NewCertPool()
	if ok := clientCaCertPool.AppendCertsFromPEM(clientCaCert); !ok {
		return nil, fmt.Errorf("unable to use provided client ca bundle")
	}
	node, err := tls.X509KeyPair(nodeCert, nodeKey)
	if err != nil {
		return nil, fmt.Errorf("creating node cert/key from env: %w", err)
	}
	clientCa, err := tls.X509KeyPair(clientCaCert, clientCaKey)
	if err != nil {
		return nil, fmt.Errorf("creating client ca cert/key from env: %w", err)
	}
	return &certBundle{
		ca:           caCertPool,
		clientCa:     clientCaCertPool,
		clientCaCert: clientCa,
		node:         node,
	}, nil
}

func configCertProvider(options Options, logger *zap.Logger, kv chord.VNode) (cipher.CertProvider, error) {
	rootDomains := options.Apex
	managedDomains := rootDomains

	if options.ACMEEnabled {
		acmeSolver := &acme.ChordSolver{
			KV:             kv,
			ManagedDomains: managedDomains,
		}
		manager, err := acme.NewManager(acme.ManagerConfig{
			Logger:         logger,
			KV:             kv,
			DNSSolver:      acmeSolver,
			ManagedDomains: managedDomains,
			CA:             options.ACMECA,
			Email:          options.ACMEEmail,
		})
		if err != nil {
			return nil, err
		}
		logger.Info("Using acme as cert provider", zap.String("email", options.ACMEEmail), zap.String("zone", options.ACMEZone))
		return manager, nil
	} else {
		logger.Info("Using self-signed as cert provider")
		self := &SelfSignedProvider{
			RootDomain: rootDomains[0],
		}
		return self, nil
	}
}
