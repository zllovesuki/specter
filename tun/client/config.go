package client

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"go.miragespace.co/specter/spec/pki"

	"go.yaml.in/yaml/v3"
)

type Tunnel struct {
	parsed             *url.URL
	Target             string        `yaml:"target" json:"target"`
	Hostname           string        `yaml:"hostname,omitempty" json:"hostname,omitempty"`
	Insecure           bool          `yaml:"insecure,omitempty" json:"insecure"`
	ProxyHeaderTimeout time.Duration `yaml:"headerTimeout,omitempty" json:"headerTimeout,omitempty"`
	ProxyHeaderHost    string        `yaml:"headerHost,omitempty" json:"headerHost,omitempty"`
	ProxyHeaderMode    string        `yaml:"headerMode,omitempty" json:"headerMode,omitempty"`
}

func (t *Tunnel) UnmarshalYAML(value *yaml.Node) error {
	type tunnelAlias Tunnel
	var aux struct {
		tunnelAlias `yaml:",inline"`
		// legacy names for backward compatibility
		OldHeaderTimeout time.Duration `yaml:"proxyHeaderTimeout,omitempty"`
		OldHeaderHost    string        `yaml:"proxyHeaderHost,omitempty"`
		OldHeaderMode    string        `yaml:"proxyHeaderMode,omitempty"`
	}
	if err := value.Decode(&aux); err != nil {
		return err
	}
	*t = Tunnel(aux.tunnelAlias)

	// If new header* fields are not set, fall back to legacy proxyHeader* values.
	// This ensures configs using header* take precedence when both are present.
	if aux.OldHeaderTimeout != 0 && t.ProxyHeaderTimeout == 0 {
		t.ProxyHeaderTimeout = aux.OldHeaderTimeout
	}
	if aux.OldHeaderHost != "" && t.ProxyHeaderHost == "" {
		t.ProxyHeaderHost = aux.OldHeaderHost
	}
	if aux.OldHeaderMode != "" && t.ProxyHeaderMode == "" {
		t.ProxyHeaderMode = aux.OldHeaderMode
	}

	return nil
}

type Config struct {
	path        string
	Version     int      `yaml:"version" json:"version"`
	Apex        string   `yaml:"apex" json:"apex"`
	Certificate string   `yaml:"certificate,omitempty" json:"certificate,omitempty"`
	PrivKey     string   `yaml:"privKey,omitempty" json:"privKey,omitempty"`
	Tunnels     []Tunnel `yaml:"tunnels,omitempty" json:"tunnels,omitempty"`
}

func NewConfig(path string) (*Config, error) {
	cfg := &Config{
		path: path,
	}
	if err := cfg.readFile(); err != nil {
		return nil, err
	}
	if err := cfg.checkVersion(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) clone() *Config {
	cfg := *c
	cfg.Tunnels = make([]Tunnel, len(c.Tunnels))
	for i := range c.Tunnels {
		cfg.Tunnels[i] = c.Tunnels[i]
		cfg.Tunnels[i].parsed = nil
	}
	cfg.validate()
	return &cfg
}

func (c *Config) checkVersion() error {
	if c.Version != 2 {
		return fmt.Errorf("expecting config version 2, got %v; migration is needed", c.Version)
	}
	return nil
}

func (c *Config) validate() error {
	for i, tunnel := range c.Tunnels {
		u, err := parseTarget(tunnel.Target)
		if err != nil {
			return err
		}

		switch tunnel.ProxyHeaderMode {
		case "", "target", "hostname", "custom":
		default:
			return fmt.Errorf("unsupported proxyHeaderMode %q", tunnel.ProxyHeaderMode)
		}

		if tunnel.ProxyHeaderMode == "custom" && tunnel.ProxyHeaderHost == "" {
			return fmt.Errorf("proxyHeaderMode 'custom' requires proxyHeaderHost to be set")
		}

		if (u.Scheme == "unix" || u.Scheme == "winio") && tunnel.ProxyHeaderMode == "target" {
			return fmt.Errorf("proxyHeaderMode 'target' is not valid for pipe targets; use 'hostname' or 'custom'")
		}
		c.Tunnels[i].parsed = u
	}
	if c.PrivKey == "" {
		_, c.PrivKey = pki.GeneratePrivKey()
	}
	return nil
}

func (c *Config) reloadFile() error {
	f, err := os.Open(c.path)
	if err != nil {
		return fmt.Errorf("error opening config file for reading: %w", err)
	}
	defer f.Close()

	next := &Config{}
	if err := yaml.NewDecoder(f).Decode(next); err != nil {
		return fmt.Errorf("error decoding config file: %w", err)
	}

	if err := next.validate(); err != nil {
		return fmt.Errorf("error validating config file: %w", err)
	}

	c.Tunnels = next.Tunnels
	return nil
}

func (c *Config) readFile() error {
	f, err := os.Open(c.path)
	if err != nil {
		return fmt.Errorf("error opening config file for reading: %w", err)
	}
	defer f.Close()
	return yaml.NewDecoder(f).Decode(c)
}

func (c *Config) writeFile() (err error) {
	f, err := os.OpenFile(c.path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("error opening config file for writing: %w", err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()

	encoder := yaml.NewEncoder(f)
	encoder.SetIndent(2)
	if err := encoder.Encode(c); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	return f.Sync()
}

func parseTarget(target string) (*url.URL, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("error parsing target %s: %w", target, err)
	}
	switch u.Scheme {
	case "http", "https", "tcp", "unix":
	default:
		if strings.HasPrefix(u.Path, "\\\\.\\pipe") {
			u.Scheme = "winio"
			break
		}
		return nil, fmt.Errorf("unsupported scheme. valid schemes: http, https, tcp, or unix; got %s", u.Scheme)
	}
	switch u.Scheme {
	case "winio":
		if runtime.GOOS != "windows" {
			return nil, errors.New("named pipe is not supported on non-Windows platform")
		}
	case "unix":
		if runtime.GOOS == "windows" {
			return nil, errors.New("unix socket is not supported on non-Unix platform")
		}
	}

	return u, nil
}
