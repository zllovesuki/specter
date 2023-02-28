package client

import (
	"fmt"
	"strconv"
	"strings"
)

type ParsedApex struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func (p *ParsedApex) String() string {
	return fmt.Sprintf("%s:%d", p.Host, p.Port)
}

func ParseApex(apex string) (*ParsedApex, error) {
	port := 443
	i := strings.Index(apex, ":")
	if i != -1 {
		nP, err := strconv.ParseInt(apex[i+1:], 0, 32)
		if err != nil {
			return nil, fmt.Errorf("error parsing port number: %w", err)
		}
		apex = apex[:i]
		port = int(nP)
	}
	return &ParsedApex{
		Host: apex,
		Port: port,
	}, nil
}
