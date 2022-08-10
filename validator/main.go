package main

import (
	"crypto/tls"
	"fmt"
)

var (
	nodes = []string{
		"seed:1113",
		"srva:1113",
		"srvb:1113",
		"srvc:1113",
		"srvd:1113",
	}
)

const (
	apex = "dev.con.nect.sh"
)

func main() {
	tlsCfg := &tls.Config{
		ServerName:         apex,
		InsecureSkipVerify: true,
	}
	serialMap := map[string]int{}
	for _, node := range nodes {
		func() {
			conn, err := tls.Dial("tcp", node, tlsCfg)
			if err != nil {
				fmt.Printf("%s failed to connection: %v\n", node, err)
				return
			}
			defer conn.Close()
			cs := conn.ConnectionState()
			for _, cert := range cs.PeerCertificates {
				if cert.Subject.CommonName != apex {
					continue
				}
				serial := cert.SerialNumber.String()
				fmt.Printf("%s - %+v\n", node, serial)
				serialMap[serial]++
			}
		}()
	}
	if len(serialMap) == 1 {
		fmt.Printf("validation successful\n")
	} else {
		fmt.Printf("validation failed: nodes do not have the same certificate")
	}
}
