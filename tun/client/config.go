package client

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"

	"kon.nect.sh/specter/spec/chord"

	"github.com/zhangyunhao116/skipmap"
	"gopkg.in/yaml.v3"
)

type Tunnel struct {
	parsed   *url.URL
	Target   string `yaml:"target" json:"target"`
	Hostname string `yaml:"hostname,omitempty" json:"hostname,omitempty"`
}

type Config struct {
	router   *skipmap.StringMap[*url.URL]
	path     string
	Apex     string   `yaml:"apex" json:"apex"`
	ClientID uint64   `yaml:"clientId,omitempty" json:"clientId,omitempty"`
	Token    string   `yaml:"token,omitempty" json:"token,omitempty"`
	Tunnels  []Tunnel `yaml:"tunnels,omitempty" json:"tunnels,omitempty"`
}

func NewConfig(path string) (*Config, error) {
	cfg := &Config{
		path:   path,
		router: skipmap.NewString[*url.URL](),
	}
	if err := cfg.readFile(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) buildRouter() {
	for _, tunnel := range c.Tunnels {
		c.router.Store(tunnel.Hostname, tunnel.parsed)
	}
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
			return fmt.Errorf("unsupported scheme. valid schemes: http, https, tcp; got %s", u.Scheme)
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
	if c.ClientID == 0 {
		c.ClientID = chord.Random()
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
