package acme

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/caddyserver/certmagic"
	"github.com/miekg/dns"
	"golang.org/x/net/idna"
)

const (
	HashcashDifficulty int           = 18
	HashcashExpires    time.Duration = time.Second * 10
)

var nonDnsRegex = regexp.MustCompile(`[^a-z0-9-.]+`)

func removeSpace(str string) string {
	var b strings.Builder
	b.Grow(len(str))
	for _, ch := range str {
		if !unicode.IsSpace(ch) {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

func Normalize(zone string) (string, error) {
	trimmed := removeSpace(zone)
	if !certmagic.SubjectQualifiesForPublicCert(trimmed) {
		return "", fmt.Errorf("acme: invalid zone for acme certificate")
	}
	if strings.Contains(trimmed, "*") {
		return "", fmt.Errorf("acme: wildcard zone is not supported")
	}
	uni, err := idna.ToASCII(trimmed)
	if err != nil {
		return "", fmt.Errorf("acme: error converting zone to ascii: %w", err)
	}
	invalid := nonDnsRegex.FindStringIndex(uni)
	if len(invalid) > 0 {
		return "", fmt.Errorf("acme: zone contains invalid dns characters")
	}
	return uni, nil
}

func EncodeZone(zone string, token []byte) string {
	hash := sha1.New()
	hash.Write([]byte(zone))
	if len(token) > 0 {
		hash.Write(token)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func GenerateRecord(zone, delegation string, token []byte) (name string, content string) {
	builder := strings.Builder{}

	builder.WriteString("_acme-challenge.")
	builder.WriteString(zone)
	if !dns.IsFqdn(zone) {
		builder.WriteString(".")
	}
	name = builder.String()

	builder.Reset()
	builder.WriteString(EncodeZone(zone, token))
	builder.WriteString(".")
	builder.WriteString(delegation)
	if !dns.IsFqdn(delegation) {
		builder.WriteString(".")
	}
	content = builder.String()

	return
}

func ParseAcmeURI(uri string) (email, zone string, err error) {
	parse, err := url.Parse(uri)
	if err != nil {
		err = fmt.Errorf("error parsing acme uri: %w", err)
		return
	}
	email = parse.User.Username()
	if email == "" {
		err = fmt.Errorf("missing email address")
		return
	}
	zone = parse.Hostname()
	if zone == "" {
		err = fmt.Errorf("missing hosted zone")
		return
	}

	return
}
