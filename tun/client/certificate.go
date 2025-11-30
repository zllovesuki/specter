package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	pkiImpl "go.miragespace.co/specter/pki"
	"go.miragespace.co/specter/spec/pki"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"

	"go.uber.org/zap"
)

func (c *Client) updateTransportCert() error {
	cert, err := tls.X509KeyPair([]byte(c.Configuration.Certificate), []byte(c.Configuration.PrivKey))
	if err != nil {
		return fmt.Errorf("error parsing certificate: %w", err)
	}
	if tp, ok := c.ServerTransport.(transport.ClientTransport); ok {
		if err := tp.WithClientCertificate(cert); err != nil {
			return err
		}
		c.Logger = c.Logger.With(zap.Uint64("id", c.ServerTransport.Identity().GetId()))
	} else {
		return fmt.Errorf("transport does not support client certificate override")
	}
	return nil
}

func (c *Client) Register(ctx context.Context) error {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	if c.Configuration.Certificate != "" {
		clientCert, err := tls.X509KeyPair([]byte(c.Configuration.Certificate), []byte(c.Configuration.PrivKey))
		if err != nil {
			return fmt.Errorf("error parsing certificate: %w", err)
		}
		cert, err := x509.ParseCertificate(clientCert.Certificate[0])
		if err != nil {
			return fmt.Errorf("error parsing certificate: %w", err)
		}

		// Check if certificate needs renewal
		if shouldRenewCertificate(cert) && c.PKIClient != nil {
			c.Logger.Info("Certificate is within renewal window, attempting renewal")
			if err := c.renewCertificate(ctx, cert); err != nil {
				c.Logger.Warn("Failed to renew certificate, continuing with existing certificate", zap.Error(err))
			} else {
				// Re-parse the updated certificate after successful renewal
				clientCert, err = tls.X509KeyPair([]byte(c.Configuration.Certificate), []byte(c.Configuration.PrivKey))
				if err != nil {
					return fmt.Errorf("error parsing renewed certificate: %w", err)
				}
				cert, err = x509.ParseCertificate(clientCert.Certificate[0])
				if err != nil {
					return fmt.Errorf("error parsing renewed certificate: %w", err)
				}
			}
		}

		resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.ClientPingResponse, error) {
			ctx = rpc.WithNode(ctx, node)
			return c.tunnelClient.Ping(ctx, &protocol.ClientPingRequest{})
		})
		if err != nil {
			return err
		}
		root := resp.GetApex()
		c.rootDomain.Store(root)

		identity, err := pki.ExtractCertificateIdentity(cert)
		if err != nil {
			return fmt.Errorf("failed to extract certificate identity: %w", err)
		}
		c.Logger.Info("Reusing existing client certificate", zap.Object("identity", identity))

		return nil
	}

	if c.PKIClient == nil {
		return errors.New("no client certificate found: please ensure your client is registered with apex first with the tunnel subcommand")
	}

	c.Logger.Info("Obtaining a new client certificate from apex")

	privKey, err := pki.UnmarshalPrivateKey([]byte(c.Configuration.PrivKey))
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	pkiReq, err := pkiImpl.CreateRequest(privKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate request: %w", err)
	}

	pkiResp, err := c.PKIClient.RequestCertificate(c.parentCtx, pkiReq)
	if err != nil {
		return fmt.Errorf("failed to obtain a client certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(pkiResp.GetCertDer())
	if err != nil {
		return fmt.Errorf("invalid certificate from PKIService: %w", err)
	}

	c.Configuration.Certificate = string(pkiResp.GetCertPem())

	identity, err := pki.ExtractCertificateIdentity(cert)
	if err != nil {
		return fmt.Errorf("failed to extract certificate identity: %w", err)
	}
	c.Logger.Info("Client certificate obtained", zap.Object("identity", identity))

	if err := c.updateTransportCert(); err != nil {
		return fmt.Errorf("failed to update transport certificate: %w", err)
	}

	if err := c.bootstrap(c.parentCtx, c.Configuration.Apex); err != nil {
		return fmt.Errorf("failed to bootstrap with certificate: %w", err)
	}

	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.RegisterIdentityResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.RegisterIdentity(ctx, &protocol.RegisterIdentityRequest{})
	})
	if err != nil {
		return err
	}

	root := resp.GetApex()
	c.rootDomain.Store(root)

	if err := c.Configuration.writeFile(); err != nil {
		c.Logger.Error("Error saving token to config file", zap.Error(err))
	}

	return nil
}

// shouldRenewCertificate checks if the certificate is within the renewal window.
func shouldRenewCertificate(cert *x509.Certificate) bool {
	return time.Until(cert.NotAfter) <= renewalWindow
}

// renewCertificate performs certificate renewal using the PKI service.
// It must be called with configMu held (or by a caller that holds configMu).
func (c *Client) renewCertificate(ctx context.Context, oldCert *x509.Certificate) error {
	privKeyPEM := c.Configuration.PrivKey

	resp, newCert, err := c.performRenewalRPC(ctx, oldCert, privKeyPEM)
	if err != nil {
		return err
	}

	return c.updateConfigurationWithCert(resp, newCert)
}

// performRenewalRPC performs the network call to renew the certificate.
// It does not hold configMu.
func (c *Client) performRenewalRPC(ctx context.Context, oldCert *x509.Certificate, privKeyPEM string) (*protocol.CertificateResponse, *x509.Certificate, error) {
	if c.PKIClient == nil {
		return nil, nil, errors.New("PKI client not available for certificate renewal")
	}

	c.Logger.Info("Renewing client certificate",
		zap.Time("notAfter", oldCert.NotAfter),
		zap.Duration("timeUntilExpiry", time.Until(oldCert.NotAfter)))

	privKey, err := pki.UnmarshalPrivateKey([]byte(privKeyPEM))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	renewReq, err := pkiImpl.CreateRenewalRequest(privKey, oldCert.Raw)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create renewal request: %w", err)
	}

	renewResp, err := c.PKIClient.RenewCertificate(ctx, renewReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to renew certificate: %w", err)
	}

	newCert, err := x509.ParseCertificate(renewResp.GetCertDer())
	if err != nil {
		return nil, nil, fmt.Errorf("invalid renewed certificate from PKIService: %w", err)
	}

	return renewResp, newCert, nil
}

// updateConfigurationWithCert updates the client configuration with the renewed certificate.
// It must be called with configMu held.
func (c *Client) updateConfigurationWithCert(renewResp *protocol.CertificateResponse, newCert *x509.Certificate) error {
	c.Configuration.Certificate = string(renewResp.GetCertPem())

	identity, err := pki.ExtractCertificateIdentity(newCert)
	if err != nil {
		return fmt.Errorf("failed to extract identity from renewed certificate: %w", err)
	}
	c.Logger.Info("Certificate renewed successfully",
		zap.Object("identity", identity),
		zap.Time("newNotAfter", newCert.NotAfter))

	if err := c.updateTransportCert(); err != nil {
		return fmt.Errorf("failed to update transport certificate: %w", err)
	}

	if err := c.Configuration.writeFile(); err != nil {
		c.Logger.Error("Error saving renewed certificate to config file", zap.Error(err))
	}

	return nil
}

// certificateMaintainer runs in the background to periodically check and renew
// the client certificate before it expires. This is important for long-running clients.
func (c *Client) certificateMaintainer(ctx context.Context) {
	defer c.closeWg.Done()

	// Don't run if PKI client is not available
	if c.PKIClient == nil {
		return
	}

	ticker := time.NewTicker(certCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.closeCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkAndRenewCertificate(ctx)
		}
	}
}

// checkAndRenewCertificate checks if the certificate needs renewal and performs it if necessary.
func (c *Client) checkAndRenewCertificate(ctx context.Context) {
	c.configMu.RLock()
	certPEM := c.Configuration.Certificate
	privKeyPEM := c.Configuration.PrivKey
	c.configMu.RUnlock()

	if certPEM == "" {
		return
	}

	// Parse the certificate to check expiry
	clientCert, err := tls.X509KeyPair([]byte(certPEM), []byte(privKeyPEM))
	if err != nil {
		c.Logger.Error("Failed to parse certificate during maintenance check", zap.Error(err))
		return
	}
	cert, err := x509.ParseCertificate(clientCert.Certificate[0])
	if err != nil {
		c.Logger.Error("Failed to parse certificate during maintenance check", zap.Error(err))
		return
	}

	if !shouldRenewCertificate(cert) {
		c.Logger.Debug("Certificate does not need renewal yet",
			zap.Time("notAfter", cert.NotAfter),
			zap.Duration("timeUntilExpiry", time.Until(cert.NotAfter)))
		return
	}

	c.Logger.Info("Background certificate maintenance: renewing certificate")

	// Perform RPC without holding the lock
	renewResp, newCert, err := c.performRenewalRPC(ctx, cert, privKeyPEM)
	if err != nil {
		c.Logger.Error("Background certificate renewal failed", zap.Error(err))
		return
	}

	// Upgrade to write lock for updating configuration
	c.configMu.Lock()
	defer c.configMu.Unlock()

	// Check if configuration has changed while we were renewing
	if c.Configuration.Certificate != certPEM {
		c.Logger.Warn("Configuration changed during certificate renewal, aborting update")
		return
	}

	if err := c.updateConfigurationWithCert(renewResp, newCert); err != nil {
		c.Logger.Error("Failed to update configuration with renewed certificate", zap.Error(err))
	}
}
