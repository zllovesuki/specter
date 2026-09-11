package acme

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		domain string
		valid  bool
	}{
		{
			domain: "hel lo.com",
			valid:  true,
		},
		{
			domain: "hello.com",
			valid:  true,
		},
		{
			domain: "good.hello.com",
			valid:  true,
		},
		{
			domain: "你好.com",
			valid:  true,
		},
		{
			domain: "xd.后缀",
			valid:  true,
		},
		{
			domain: "你好.后缀",
			valid:  true,
		},

		{
			domain: "*.wildcard.com",
			valid:  false,
		},
		{
			domain: "*.com",
			valid:  false,
		},
		{
			domain: "sup.*.com",
			valid:  false,
		},
		{
			domain: "localhost",
			valid:  false,
		},
		{
			domain: "machine.localhost",
			valid:  false,
		},
		{
			domain: "machine.local",
			valid:  false,
		},
		{
			domain: "hello:world.com",
			valid:  false,
		},
		{
			domain: "hello%world.com",
			valid:  false,
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s should be %v", tc.domain, tc.valid),
			func(t *testing.T) {
				as := require.New(t)
				normalized, err := Normalize(tc.domain)
				if tc.valid {
					as.NoError(err)
					as.False(strings.ContainsFunc(normalized, func(r rune) bool { return r > unicode.MaxASCII }), "normalized hostname should be ascii only")
				} else {
					as.Error(err)
				}
			})
	}
}
