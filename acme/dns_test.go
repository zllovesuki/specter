package acme

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"kon.nect.sh/specter/spec/mocks"
)

const (
	testEmail         = "test@test.com"
	testDomain        = "acme.example.com"
	testNSName        = "ns.acme.example.com"
	testNSContentA    = "192.168.1.1"
	testNSContentAAAA = "2001:0db8:85a3:0000:0000:8a2e:0370:7334"
	testTXTSubdomain  = "subdomain"
	testTXTResponse   = "hello"
)

func getUDPListener(as *require.Assertions) (net.PacketConn, int) {
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	as.NoError(err)

	return l, l.LocalAddr().(*net.UDPAddr).Port
}

func TestStaticQuery(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	kv := new(mocks.VNode)
	defer kv.AssertExpectations(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := NewDNS(ctx, logger, kv, testEmail, testDomain, map[string][]string{
		testNSName: {testNSContentA, testNSContentAAAA},
	})

	listener, port := getUDPListener(as)
	defer listener.Close()

	mux := dns.NewServeMux()
	mux.Handle(testDomain, h)
	srv := &dns.Server{
		PacketConn: listener,
		Handler:    mux,
	}
	go srv.ActivateAndServe()

	var (
		client  = &dns.Client{}
		resp, m *dns.Msg
		err     error
	)

	// SOA
	m = new(dns.Msg)
	m.SetQuestion(dns.CanonicalName(testDomain), dns.TypeSOA)
	resp, _, err = client.Exchange(m, fmt.Sprintf("127.0.0.1:%d", port))
	as.NoError(err)

	as.Len(resp.Answer, 1)
	soa, ok := resp.Answer[0].(*dns.SOA)
	as.True(ok)
	as.Equal(dns.CanonicalName(testNSName), soa.Ns)

	// NS A record
	m = new(dns.Msg)
	m.SetQuestion(dns.CanonicalName(testNSName), dns.TypeA)
	resp, _, err = client.Exchange(m, fmt.Sprintf("127.0.0.1:%d", port))
	as.NoError(err)

	as.Len(resp.Answer, 1)
	nsA, ok := resp.Answer[0].(*dns.A)
	as.True(ok)
	as.EqualValues(net.ParseIP(testNSContentA).To4(), nsA.A)

	// NS AAAA record
	m = new(dns.Msg)
	m.SetQuestion(dns.CanonicalName(testNSName), dns.TypeAAAA)
	resp, _, err = client.Exchange(m, fmt.Sprintf("127.0.0.1:%d", port))
	as.NoError(err)

	as.Len(resp.Answer, 1)
	nsAAAA, ok := resp.Answer[0].(*dns.AAAA)
	as.True(ok)
	as.EqualValues(net.ParseIP(testNSContentAAAA), nsAAAA.AAAA)
}

func TestDynamicQuery(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	kv := new(mocks.VNode)
	defer kv.AssertExpectations(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := NewDNS(ctx, logger, kv, testEmail, testDomain, map[string][]string{
		testNSName: {testNSContentA, testNSContentAAAA},
	})

	listener, port := getUDPListener(as)
	defer listener.Close()

	mux := dns.NewServeMux()
	mux.Handle(testDomain, h)
	srv := &dns.Server{
		PacketConn: listener,
		Handler:    mux,
	}
	go srv.ActivateAndServe()

	var (
		client  = &dns.Client{}
		resp, m *dns.Msg
		err     error
	)

	kv.On("PrefixList", mock.Anything, mock.MatchedBy(func(prefix []byte) bool {
		return bytes.Equal(prefix, []byte(dnsKeyName(testTXTSubdomain)))
	})).Return([][]byte{[]byte(testTXTResponse)}, nil)

	m = new(dns.Msg)
	m.SetQuestion(dns.CanonicalName(fmt.Sprintf("%s.%s", testTXTSubdomain, testDomain)), dns.TypeTXT)
	resp, _, err = client.Exchange(m, fmt.Sprintf("127.0.0.1:%d", port))
	as.NoError(err)

	as.Len(resp.Answer, 1)
	txt, ok := resp.Answer[0].(*dns.TXT)
	as.True(ok)
	as.Len(txt.Txt, 1)
	as.Equal(testTXTResponse, txt.Txt[0])
}
