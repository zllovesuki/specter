package acme

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/tun"

	"github.com/caddyserver/certmagic"
	"github.com/mholt/acmez"
	"go.uber.org/zap"
)

var (
	ErrInvalid = fmt.Errorf("acme: invalid hostname")
)

type ManagerConfig struct {
	Logger         *zap.Logger
	KV             chord.KV
	DNSSolver      acmez.Solver
	ManagedDomains []string
	CA             string
	Email          string
}

type Manager struct {
	parentCtx     context.Context
	managedConfig *certmagic.Config
	dynamicConfig *certmagic.Config
	managed       []string
	ManagerConfig
}

func NewManager(ctx context.Context, cfg ManagerConfig) (*Manager, error) {
	kvStore, err := NewChordStorage(
		cfg.Logger.With(zap.String("component", "acme_storage")),
		cfg.KV,
		StorageConfig{
			RetryInterval: time.Second * 3,
			LeaseTTL:      time.Minute,
		})
	if err != nil {
		return nil, err
	}

	isDev := cfg.CA != certmagic.LetsEncryptProductionCA
	manager := &Manager{
		parentCtx:     ctx,
		ManagerConfig: cfg,
	}

	managedConfig := certmagic.Config{
		Storage:           kvStore,
		DefaultServerName: cfg.ManagedDomains[0],
		Logger:            cfg.Logger.With(zap.String("component", "acme_managed")),
	}
	managedIssuer := certmagic.NewACMEIssuer(&managedConfig, certmagic.ACMEIssuer{
		CA:                      cfg.CA,
		Email:                   cfg.Email,
		Agreed:                  true,
		Logger:                  cfg.Logger.With(zap.String("component", "acme_managed_issuer")),
		DNS01Solver:             cfg.DNSSolver,
		DisableHTTPChallenge:    true,
		DisableTLSALPNChallenge: true,
	})
	managedConfig.Issuers = []certmagic.Issuer{managedIssuer}

	dynamicConfig := certmagic.Config{
		Storage:           kvStore,
		DefaultServerName: cfg.ManagedDomains[0],
		Logger:            cfg.Logger.With(zap.String("component", "acme_dynamic")),
		OnDemand: &certmagic.OnDemandConfig{
			DecisionFunc: manager.check,
		},
	}
	dynamicIssuer := certmagic.NewACMEIssuer(&dynamicConfig, certmagic.ACMEIssuer{
		CA:                      cfg.CA,
		Email:                   cfg.Email,
		Agreed:                  true,
		Logger:                  cfg.Logger.With(zap.String("component", "acme_dynamic_issuer")),
		DNS01Solver:             cfg.DNSSolver,
		DisableHTTPChallenge:    true,
		DisableTLSALPNChallenge: true,
	})
	dynamicConfig.Issuers = []certmagic.Issuer{dynamicIssuer}

	if isDev {
		managedConfig.OCSP = certmagic.OCSPConfig{
			DisableStapling: true,
		}
		dynamicConfig.OCSP = certmagic.OCSPConfig{
			DisableStapling: true,
		}
	}

	cache := certmagic.NewCache(certmagic.CacheOptions{
		Logger:           cfg.Logger.With(zap.String("component", "acme_cache")),
		GetConfigForCert: manager.getConfig,
	})

	manager.managedConfig = certmagic.New(cache, managedConfig)
	manager.dynamicConfig = certmagic.New(cache, dynamicConfig)
	manager.managed = make([]string, len(cfg.ManagedDomains))
	copy(manager.managed, cfg.ManagedDomains)
	for _, d := range cfg.ManagedDomains {
		manager.managed = append(manager.managed, "*."+d)
	}

	return manager, nil
}

func (m *Manager) check(name string) error {
	m.dynamicConfig.Logger.Debug("Dynamic certificate request", zap.String("name", name))

	callCtx, cancel := context.WithTimeout(m.parentCtx, time.Second)
	defer cancel()

	_, err := tun.FindCustomHostname(callCtx, m.KV, name)

	return err
}

func (m *Manager) isManaged(subject string) bool {
	for _, d := range m.managed {
		if certmagic.MatchWildcard(subject, d) {
			return true
		}
	}
	return false
}

func (m *Manager) getConfig(c certmagic.Certificate) (*certmagic.Config, error) {
	for _, d := range c.Names {
		if m.isManaged(d) {
			return m.managedConfig, nil
		}
	}
	return m.dynamicConfig, nil
}

func (m *Manager) GetCertificate(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
	sni := chi.ServerName
	if sni == "" {
		return nil, ErrInvalid
	}

	acmeHostname.Add(sni, 1)
	if m.isManaged(sni) {
		return m.managedConfig.GetCertificate(chi)
	} else {
		return m.dynamicConfig.GetCertificate(chi)
	}
}

func (m *Manager) Initialize(ctx context.Context) error {
	return m.managedConfig.ManageAsync(ctx, m.managed)
}
