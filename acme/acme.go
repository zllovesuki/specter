package acme

import "fmt"

const (
	dnsKeyPrefix = "/acme-dns/"
	kvKeyPrefix  = "/acme-storage/"
)

func dnsKeyName(key string) string {
	return fmt.Sprintf("%s%s", dnsKeyPrefix, key)
}

func kvKeyName(key string) string {
	return fmt.Sprintf("%s%s", kvKeyPrefix, key)
}
