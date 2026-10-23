// implementation is adopted from https://github.com/joohoi/acme-dns/blob/master/dns.go
package acme

import (
	"context"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"go.miragespace.co/specter/config"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/timing"

	"github.com/miekg/dns"
	"go.uber.org/zap"
)

type DNS struct {
	parentCtx context.Context
	logger    *zap.Logger
	storage   chord.KV
	domain    string
	email     string
	updateMu  sync.Mutex
	view      atomic.Pointer[dnsView]
}

// dnsView is immutable after publication. Every question and the negative
// response SOA in one DNS response use the same view.
type dnsView struct {
	version string
	soa     *dns.SOA
	records map[string][]dns.RR
}

var _ dns.Handler = (*DNS)(nil)

func NewDNS(ctx context.Context, logger *zap.Logger, kv chord.KV, email, domain string, record config.Record) (*DNS, error) {
	server := &DNS{
		parentCtx: ctx,
		logger:    logger,
		storage:   kv,
		domain:    dns.CanonicalName(domain),
		email:     email,
	}
	if err := server.ApplyConfig(record); err != nil {
		return nil, err
	}
	return server, nil
}

// ApplyConfig publishes a complete validated view, retaining the current one
// if decoding or validation fails. Older or repeated versions are a no-op, so a
// local server recovering from stale storage cannot roll back a running DNS view.
func (d *DNS) ApplyConfig(record config.Record) error {
	d.updateMu.Lock()
	defer d.updateMu.Unlock()

	dnsConfig, err := DecodeDNSConfig(record, d.domain)
	if err != nil {
		return err
	}
	version := record.Token()
	if previous := d.view.Load(); previous != nil {
		order, err := record.CompareVersion(previous.version)
		if err != nil {
			return err
		}
		if order <= 0 {
			return nil
		}
	}
	view := &dnsView{
		version: version,
		records: make(map[string][]dns.RR, len(dnsConfig.Nameservers)+1),
	}
	for _, ns := range dnsConfig.Nameservers {

		nsRR := new(dns.NS)
		nsRR.Hdr = dns.RR_Header{Name: d.domain, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 3600}
		nsRR.Ns = ns.Host
		view.records[d.domain] = append(view.records[d.domain], nsRR)
		if !dns.IsSubDomain(d.domain, ns.Host) {
			continue
		}

		for _, ip := range ns.Addresses {
			parsed := net.ParseIP(ip)

			if strings.Contains(ip, ":") {
				aaaaRR := new(dns.AAAA)
				aaaaRR.Hdr = dns.RR_Header{Name: ns.Host, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 3600}
				aaaaRR.AAAA = parsed
				view.records[ns.Host] = append(view.records[ns.Host], aaaaRR)
			} else {
				aRR := new(dns.A)
				aRR.Hdr = dns.RR_Header{Name: ns.Host, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 3600}
				aRR.A = parsed
				view.records[ns.Host] = append(view.records[ns.Host], aRR)
			}
		}
	}

	// The replicated revision supplies an RFC 1982 serial, including uint32
	// wraparound. Concurrent versions at one revision can share a serial; this
	// server does not provide AXFR/IXFR and does not use SOA serials for merging.
	view.soa = &dns.SOA{
		Hdr:     dns.RR_Header{Name: d.domain, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 3600},
		Ns:      dnsConfig.Nameservers[0].Host,
		Mbox:    dns.CanonicalName(strings.ReplaceAll(d.email, "@", ".")),
		Serial:  uint32(record.Version.Revision),
		Refresh: 28800,
		Retry:   7200,
		Expire:  604800,
		Minttl:  86400,
	}
	view.records[d.domain] = append(view.records[d.domain], view.soa)
	d.view.Store(view)
	d.logger.Info("ACME DNS configured", zap.String("domain", d.domain), zap.String("version", version), zap.Any("ns", dnsConfig.Nameservers))
	return nil
}

func (d *DNS) ConfigVersion() string {
	return d.view.Load().version
}

func (d *DNS) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	defer w.WriteMsg(m)

	opt := r.IsEdns0()
	if opt != nil {
		m.SetEdns0(512, false)
		if opt.Version() != 0 {
			m.MsgHdr.Rcode = dns.RcodeBadVers
			return
		}
	}
	if r.Opcode == dns.OpcodeQuery {
		d.readQuery(m, d.view.Load())
	}
}

func (d *DNS) readQuery(m *dns.Msg, view *dnsView) {
	var authoritative = false

	for _, que := range m.Question {
		rr, rc, auth := d.answer(que, view)
		if auth {
			authoritative = auth
		}
		m.MsgHdr.Rcode = rc
		m.Answer = append(m.Answer, rr...)
	}

	m.MsgHdr.Authoritative = authoritative
	if authoritative && m.MsgHdr.Rcode == dns.RcodeNameError {
		m.Ns = append(m.Ns, view.soa)
	}
}

func (d *DNS) isImmediate(q dns.Question) bool {
	qname := strings.ToLower(q.Name)
	query := strings.Split(qname, ".")
	self := strings.Split(d.domain, ".")
	return dns.IsSubDomain(d.domain, qname) &&
		len(query) >= len(self) &&
		len(query)-len(self) <= 1
}

func (d *DNS) answer(q dns.Question, view *dnsView) (rr []dns.RR, rcode int, auth bool) {
	qname := strings.ToLower(q.Name)
	defined := view.records[qname]
	auth = len(defined) > 0 || d.isImmediate(q)
	if !auth {
		return nil, dns.RcodeNameError, true
	}

	if q.Qtype == dns.TypeANY {
		rcode = dns.RcodeNotImplemented
		return
	}

	for _, ri := range defined {
		if ri.Header().Rrtype == q.Qtype {
			rr = append(rr, ri)
		}
	}

	if q.Qtype == dns.TypeTXT && d.isImmediate(q) {
		txtRRs, err := d.answerTXT(q)
		if err != nil {
			rcode = dns.RcodeServerFailure
		} else {
			rr = append(rr, txtRRs...)
		}
	}

	if len(rr) == 0 && rcode != dns.RcodeServerFailure {
		rcode = dns.RcodeNameError
	}

	d.logger.Debug("Answering DNS query", zap.String("qtype", dns.TypeToString[q.Qtype]), zap.String("rcode", dns.RcodeToString[rcode]), zap.String("domain", qname))

	return rr, rcode, auth
}

func (d *DNS) answerTXT(q dns.Question) ([]dns.RR, error) {
	var ra []dns.RR

	qname := strings.ToLower(q.Name)
	idx := strings.Index(qname, d.domain)
	if idx <= 0 {
		return ra, nil
	}
	subdomain := qname[0 : idx-1]

	callCtx, cancel := context.WithTimeout(d.parentCtx, timing.DNSLookupTimeout)
	defer cancel()

	vals, err := d.storage.PrefixList(callCtx, []byte(dnsKeyName(subdomain)))
	if err != nil {
		d.logger.Error("Failed to lookup TXT", zap.String("subdomain", subdomain), zap.Error(err))
		return nil, err
	}

	for _, v := range vals {
		if len(v) > 0 {
			r := new(dns.TXT)
			r.Hdr = dns.RR_Header{Name: q.Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 1}
			r.Txt = append(r.Txt, string(v))
			ra = append(ra, r)
		}
	}

	return ra, nil
}
