package client

import (
	"testing"
	"time"
)

const (
	testHostname = "abcd"
	testApex     = "specter.dev"
)

func TestMain(m *testing.M) {
	checkInterval = time.Millisecond * 200
	rttInterval = time.Millisecond * 250
	certCheckInterval = time.Millisecond * 100 // Shortened for testing background renewal
	m.Run()
}
