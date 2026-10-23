package acme

import (
	"bytes"
	"fmt"
	"net"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/mocks"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
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

	ctx := t.Context()

	h, err := NewDNS(ctx, logger, kv, testEmail, testDomain, dnsTestRecord(t, 1, dnsTestConfig()))
	as.NoError(err)

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

	ctx := t.Context()

	h, err := NewDNS(ctx, logger, kv, testEmail, testDomain, dnsTestRecord(t, 1, dnsTestConfig()))
	as.NoError(err)

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

func TestDNSConfigUpdatesRecordsAndSerialTogether(t *testing.T) {
	initial := dnsTestRecord(t, 1, dnsTestConfig())
	handler, err := NewDNS(t.Context(), zap.NewNop(), nil, testEmail, testDomain, initial)
	require.NoError(t, err)

	require.NoError(t, handler.ApplyConfig(initial))
	require.Equal(t, initial.Token(), handler.ConfigVersion())
	require.Equal(t, uint32(1), dnsQuery(handler, testDomain, dns.TypeSOA).Answer[0].(*dns.SOA).Serial)
	updatedConfig := DNSConfig{Zone: testDomain, Nameservers: []DNSNameserver{
		{Host: "ns2.edge." + testDomain, Addresses: []string{"192.0.2.2", "2001:db8::2"}},
		{Host: "ns.external.example", Addresses: []string{"192.0.2.3"}},
	}}
	updated := dnsTestRecord(t, 2, updatedConfig)
	require.NoError(t, handler.ApplyConfig(updated))
	require.Equal(t, updated.Token(), handler.ConfigVersion())

	soa := dnsQuery(handler, testDomain, dns.TypeSOA).Answer[0].(*dns.SOA)
	require.Equal(t, uint32(2), soa.Serial)
	require.Equal(t, "ns.external.example.", soa.Ns)
	ns := dnsQuery(handler, testDomain, dns.TypeNS).Answer
	require.Len(t, ns, 2)
	require.Equal(t, "ns.external.example.", ns[0].(*dns.NS).Ns)
	require.Equal(t, "ns2.edge."+testDomain+".", ns[1].(*dns.NS).Ns)
	require.Equal(t, "192.0.2.2", dnsQuery(handler, "ns2.edge."+testDomain, dns.TypeA).Answer[0].(*dns.A).A.String())
	require.Equal(t, "2001:db8::2", dnsQuery(handler, "ns2.edge."+testDomain, dns.TypeAAAA).Answer[0].(*dns.AAAA).AAAA.String())
	require.Empty(t, dnsQuery(handler, "ns.external.example", dns.TypeA).Answer, "external NS addresses are not authoritative records for this zone")
	negative := dnsQuery(handler, testNSName, dns.TypeA)
	require.Equal(t, dns.RcodeNameError, negative.Rcode)
	require.Empty(t, negative.Answer)
	require.Equal(t, uint32(2), negative.Ns[0].(*dns.SOA).Serial)

	invalid := dnsTestConfig()
	invalid.Nameservers[0].Addresses = []string{"invalid"}
	require.Error(t, handler.ApplyConfig(dnsTestRecord(t, 3, invalid)))
	require.Equal(t, updated.Token(), handler.ConfigVersion())
	require.Equal(t, uint32(2), dnsQuery(handler, testDomain, dns.TypeSOA).Answer[0].(*dns.SOA).Serial)
}

func TestDNSConfigSerialWrapAndRestart(t *testing.T) {
	before := dnsTestRecord(t, 1<<32-1, dnsTestConfig())
	handler, err := NewDNS(t.Context(), zap.NewNop(), nil, testEmail, testDomain, before)
	require.NoError(t, err)
	previous := dnsQuery(handler, testDomain, dns.TypeSOA).Answer[0].(*dns.SOA).Serial
	require.Equal(t, uint32(1<<32-1), previous)
	after := dnsTestRecord(t, 1<<32, dnsTestConfig())
	require.NoError(t, handler.ApplyConfig(after))
	serial := dnsQuery(handler, testDomain, dns.TypeSOA).Answer[0].(*dns.SOA).Serial
	require.Equal(t, uint32(0), serial)
	require.Equal(t, uint32(1), serial-previous, "RFC 1982 increment wraps at 32 bits")

	restarted, err := NewDNS(t.Context(), zap.NewNop(), nil, testEmail, testDomain, after)
	require.NoError(t, err)
	require.Equal(t, serial, dnsQuery(restarted, testDomain, dns.TypeSOA).Answer[0].(*dns.SOA).Serial)
}

func TestDNSConfigIgnoresOlderLocalSnapshots(t *testing.T) {
	first := dnsTestConfig()
	second := dnsTestConfig()
	second.Nameservers[0].Addresses = []string{"192.0.2.2"}
	lower := dnsTestRecord(t, 7, first)
	higher := dnsTestRecord(t, 7, second)
	if bytes.Compare(lower.Version.Digest[:], higher.Version.Digest[:]) > 0 {
		lower, higher = higher, lower
	}
	handler, err := NewDNS(t.Context(), zap.NewNop(), nil, testEmail, testDomain, lower)
	require.NoError(t, err)

	require.NoError(t, handler.ApplyConfig(higher))
	require.Equal(t, higher.Token(), handler.ConfigVersion(), "the higher hash wins concurrent writes at one revision")
	activeAnswer := dnsQuery(handler, testNSName, dns.TypeA).Answer[0].String()

	require.NoError(t, handler.ApplyConfig(lower))
	require.Equal(t, higher.Token(), handler.ConfigVersion(), "an older concurrent version cannot undo the winner")
	require.Equal(t, activeAnswer, dnsQuery(handler, testNSName, dns.TypeA).Answer[0].String())

	require.NoError(t, handler.ApplyConfig(dnsTestRecord(t, 6, first)))
	require.Equal(t, higher.Token(), handler.ConfigVersion(), "a local server restored from an older snapshot cannot roll back DNS")
	require.Equal(t, activeAnswer, dnsQuery(handler, testNSName, dns.TypeA).Answer[0].String())
	require.Equal(t, uint32(7), dnsQuery(handler, testDomain, dns.TypeSOA).Answer[0].(*dns.SOA).Serial)
}

func TestDNSResponseUsesOneConfigDuringUpdate(t *testing.T) {
	kv := new(mocks.VNode)
	defer kv.AssertExpectations(t)
	entered := make(chan struct{})
	resume := make(chan struct{})
	kv.On("PrefixList", mock.Anything, []byte(dnsKeyName(testTXTSubdomain))).
		Run(func(mock.Arguments) {
			close(entered)
			<-resume
		}).Return([][]byte{}, nil).Once()
	handler, err := NewDNS(t.Context(), zap.NewNop(), kv, testEmail, testDomain, dnsTestRecord(t, 1, dnsTestConfig()))
	require.NoError(t, err)
	request := new(dns.Msg)
	request.SetQuestion(testTXTSubdomain+"."+testDomain+".", dns.TypeTXT)
	writer := &dnsResponseRecorder{}
	done := make(chan struct{})
	go func() {
		handler.ServeDNS(writer, request)
		close(done)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("TXT lookup did not start")
	}
	updated := DNSConfig{Zone: testDomain, Nameservers: []DNSNameserver{{Host: "ns2." + testDomain, Addresses: []string{"192.0.2.2"}}}}
	err = handler.ApplyConfig(dnsTestRecord(t, 2, updated))
	close(resume)
	require.NoError(t, err)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("DNS response did not finish")
	}
	require.Empty(t, writer.message.Answer)
	require.Equal(t, dns.RcodeNameError, writer.message.Rcode)
	require.Equal(t, testNSName+".", writer.message.Ns[0].(*dns.SOA).Ns)
	require.Equal(t, uint32(1), writer.message.Ns[0].(*dns.SOA).Serial)
	require.Equal(t, uint32(2), dnsQuery(handler, testDomain, dns.TypeSOA).Answer[0].(*dns.SOA).Serial)
}

func dnsQuery(handler dns.Handler, name string, kind uint16) *dns.Msg {
	request := new(dns.Msg)
	request.SetQuestion(dns.Fqdn(name), kind)
	writer := &dnsResponseRecorder{}
	handler.ServeDNS(writer, request)
	return writer.message
}

type dnsResponseRecorder struct {
	message *dns.Msg
}

func (*dnsResponseRecorder) LocalAddr() net.Addr       { return &net.UDPAddr{} }
func (*dnsResponseRecorder) RemoteAddr() net.Addr      { return &net.UDPAddr{} }
func (*dnsResponseRecorder) Write([]byte) (int, error) { panic("unexpected raw DNS write") }
func (*dnsResponseRecorder) Close() error              { return nil }
func (*dnsResponseRecorder) TsigStatus() error         { return nil }
func (*dnsResponseRecorder) TsigTimersOnly(bool)       {}
func (*dnsResponseRecorder) Hijack()                   {}
func (w *dnsResponseRecorder) WriteMsg(message *dns.Msg) error {
	w.message = message
	return nil
}
