package main

import (
	"crypto/tls"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Printf("missing apex and at least 1 endpoint\n")
		os.Exit(1)
	}
	apex := os.Args[1]
	nodes := os.Args[2:]
	tlsCfg := &tls.Config{
		ServerName:         apex,
		InsecureSkipVerify: true,
	}
	serialMap := map[string]int{}
	for _, node := range nodes {
		func() {
			conn, err := tls.Dial("tcp", node, tlsCfg)
			if err != nil {
				fmt.Printf("failed to connect %s: %v\n", node, err)
				os.Exit(1)
			}
			defer conn.Close()
			serial, err := peerCertificateSerial(conn.ConnectionState(), apex)
			if err != nil {
				fmt.Printf("invalid certificate from %s: %v\n", node, err)
				os.Exit(1)
			}
			fmt.Printf("%s - %s\n", node, serial)
			serialMap[serial]++
		}()
	}
	if len(serialMap) == 1 {
		fmt.Printf("validation successful\n")
		os.Exit(0)
	} else {
		fmt.Printf("validation failed: nodes do not have the same certificate")
		os.Exit(1)
	}
}

func peerCertificateSerial(state tls.ConnectionState, hostname string) (string, error) {
	if len(state.PeerCertificates) == 0 {
		return "", fmt.Errorf("peer did not present a certificate")
	}
	leaf := state.PeerCertificates[0]
	if err := leaf.VerifyHostname(hostname); err != nil {
		return "", err
	}
	return leaf.SerialNumber.String(), nil
}
