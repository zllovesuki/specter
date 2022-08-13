package client

import (
	"fmt"
	"net/url"
	"os"

	"github.com/zhangyunhao116/skipmap"
	"gopkg.in/yaml.v3"
	"kon.nect.sh/specter/spec/chord"
)

type Tunnel struct {
	parsed   *url.URL
	Target   string `yaml:"target"`
	Hostname string `yaml:"hostname,omitempty"`
}

type Config struct {
	router   *skipmap.StringMap[*url.URL]
	path     string
	Apex     string   `yaml:"apex"`
	ClientID uint64   `yaml:"clientId,omitempty"`
	Token    string   `yaml:"token,omitempty"`
	Tunnels  []Tunnel `yaml:"tunnels,omitempty"`
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
		case "http", "https", "tcp":
		default:
			return fmt.Errorf("unsupported scheme. valid schemes: http, https, tcp; got %s", u.Scheme)
		}
		c.Tunnels[i].parsed = u
	}
	if c.ClientID == 0 {
		c.ClientID = chord.Random()
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
	encoder := yaml.NewEncoder(f)
	encoder.SetIndent(2)
	defer encoder.Close()
	return encoder.Encode(c)
}
