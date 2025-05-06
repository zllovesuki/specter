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

	"github.com/zhangyunhao116/skipmap"
	"gopkg.in/yaml.v3"
)

type Tunnel struct {
	parsed             *url.URL
	Target             string        `yaml:"target" json:"target"`
	Hostname           string        `yaml:"hostname,omitempty" json:"hostname,omitempty"`
	Insecure           bool          `yaml:"insecure,omitempty" json:"insecure"`
	ProxyHeaderTimeout time.Duration `yaml:"proxyHeaderTimeout,omitempty" json:"proxyHeaderTimeout,omitempty"`
	ProxyHeaderHost    string        `yaml:"proxyHeaderHost,omitempty" json:"proxyHeaderHost,omitempty"`
}

type Config struct {
	router      *skipmap.StringMap[route]
	path        string
	Version     int      `yaml:"version" json:"version"`
	Apex        string   `yaml:"apex" json:"apex"`
	Certificate string   `yaml:"certificate,omitempty" json:"certificate,omitempty"`
	PrivKey     string   `yaml:"privKey,omitempty" json:"privKey,omitempty"`
	Tunnels     []Tunnel `yaml:"tunnels,omitempty" json:"tunnels,omitempty"`
}

type route struct {
	parsed                 *url.URL
	insecure               bool
	proxyHeaderReadTimeout time.Duration
	proxyHeaderHost        string
}

func NewConfig(path string) (*Config, error) {
	cfg := &Config{
		path:   path,
		router: skipmap.NewString[route](),
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
	cfg.router = skipmap.NewString[route]()
	cfg.Tunnels = make([]Tunnel, len(c.Tunnels))
	for i := range c.Tunnels {
		cfg.Tunnels[i] = c.Tunnels[i]
		cfg.Tunnels[i].parsed = nil
	}
	cfg.validate()
	return &cfg
}

func (c *Config) buildRouter(drop ...Tunnel) {
	for _, tunnel := range drop {
		c.router.Delete(tunnel.Hostname)
	}
	for _, tunnel := range c.Tunnels {
		c.router.Store(tunnel.Hostname, route{
			parsed:                 tunnel.parsed,
			insecure:               tunnel.Insecure,
			proxyHeaderReadTimeout: tunnel.ProxyHeaderTimeout,
			proxyHeaderHost:        tunnel.ProxyHeaderHost,
		})
	}
}

func (c *Config) checkVersion() error {
	if c.Version != 2 {
		return fmt.Errorf("expecting config version 2, got %v; migration is needed", c.Version)
	}
	return nil
}

func (c *Config) validate() error {
	for i, tunnel := range c.Tunnels {
		u, err := url.Parse(tunnel.Target)
		if err != nil {
			return fmt.Errorf("error parsing target %s: %w", tunnel.Target, err)
		}
		switch u.Scheme {
		case "http", "https", "tcp", "unix":
		default:
			if strings.HasPrefix(u.Path, "\\\\.\\pipe") {
				u.Scheme = "winio"
				break
			}
			return fmt.Errorf("unsupported scheme. valid schemes: http, https, tcp, or unix; got %s", u.Scheme)
		}
		switch u.Scheme {
		case "winio":
			if runtime.GOOS != "windows" {
				return errors.New("named pipe is not supported on non-Windows platform")
			}
		case "unix":
			if runtime.GOOS == "windows" {
				return errors.New("unix socket is not supported on non-Unix platform")
			}
		}
		c.Tunnels[i].parsed = u
	}
	if c.PrivKey == "" {
		_, c.PrivKey = pki.GeneratePrivKey()
	}
	return nil
}

func (c *Config) reloadFile(callbacks ...func(prev, curr []Tunnel)) error {
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

	prev := c.clone()
	c.Tunnels = next.Tunnels

	for _, cb := range callbacks {
		cb(prev.Tunnels, next.Tunnels)
	}
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

func (c *Config) writeFile() error {
	f, err := os.OpenFile(c.path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("error opening config file for writing: %w", err)
	}
	defer f.Close()
	defer f.Sync()

	encoder := yaml.NewEncoder(f)
	encoder.SetIndent(2)
	defer encoder.Close()
	return encoder.Encode(c)
}
