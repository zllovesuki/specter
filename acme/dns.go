// implementation is adopted from https://github.com/joohoi/acme-dns/blob/master/dns.go
package acme

import (
	"context"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/util"

	"github.com/miekg/dns"
	"go.uber.org/zap"
)

var (
	BuildTime = "now"
)

type DNS struct {
	parentCtx context.Context
	logger    *zap.Logger
	storage   chord.KV
	soa       *dns.SOA
	records   map[string][]dns.RR
	domain    string
}

var _ dns.Handler = (*DNS)(nil)

func NewDNS(ctx context.Context, logger *zap.Logger, kv chord.KV, email, domain string, ns map[string][]string) *DNS {
	if !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}
	server := &DNS{
		parentCtx: ctx,
		logger:    logger,
		storage:   kv,
		domain:    strings.ToLower(domain),
		records: map[string][]dns.RR{
			domain: make([]dns.RR, 0),
		},
	}
	server.initStatic(email, ns)

	logger.Info("ACME DNS configured", zap.String("domain", server.domain), zap.String("mbox", server.soa.Mbox), zap.Any("ns", ns))

	return server
}

func (d *DNS) initStatic(email string, ns map[string][]string) {
	for ns, ips := range ns {
		ns = dns.Fqdn(ns)

		nsRR := new(dns.NS)
		nsRR.Hdr = dns.RR_Header{Name: d.domain, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 3600}
		nsRR.Ns = ns
		d.records[d.domain] = append(d.records[d.domain], nsRR)

		if _, ok := d.records[ns]; !ok {
			d.records[ns] = make([]dns.RR, 0)
		}

		for _, ip := range ips {
			parsed := net.ParseIP(ip)
			if parsed == nil {
				continue
			}

			if strings.Contains(ip, ":") {
				aaaaRR := new(dns.AAAA)
				aaaaRR.Hdr = dns.RR_Header{Name: dns.Fqdn(ns), Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 3600}
				aaaaRR.AAAA = parsed
				d.records[ns] = append(d.records[ns], aaaaRR)
			} else {
				aRR := new(dns.A)
				aRR.Hdr = dns.RR_Header{Name: dns.Fqdn(ns), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 3600}
				aRR.A = parsed
				d.records[ns] = append(d.records[ns], aRR)
			}
		}
	}

	mbox := dns.CanonicalName(strings.ReplaceAll(email, "@", "."))
	nameservers := make([]string, 0)
	for ns := range d.records {
		nameservers = append(nameservers, ns)
	}
	sort.Strings(nameservers)

	var (
		unix   int64
		serial uint32
	)
	unix, err := strconv.ParseInt(BuildTime, 10, 64)
	if err != nil {
		unix = time.Now().Unix()
	}
	serial = uint32(util.Must(strconv.ParseUint(time.Unix(unix, 0).UTC().Format("2006010215"), 10, 32)))

	soaRR := &dns.SOA{
		Hdr:     dns.RR_Header{Name: d.domain, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 3600},
		Ns:      nameservers[0],
		Mbox:    mbox,
		Serial:  serial,
		Refresh: 28800,
		Retry:   7200,
		Expire:  604800,
		Minttl:  86400,
	}
	d.records[d.domain] = append(d.records[d.domain], soaRR)
	d.soa = soaRR
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
		d.readQuery(m)
	}
}

func (d *DNS) readQuery(m *dns.Msg) {
	var authoritative = false

	for _, que := range m.Question {
		rr, rc, auth := d.answer(que)
		if auth {
			authoritative = auth
		}
		m.MsgHdr.Rcode = rc
		m.Answer = append(m.Answer, rr...)
	}

	m.MsgHdr.Authoritative = authoritative
	if authoritative && m.MsgHdr.Rcode == dns.RcodeNameError {
		m.Ns = append(m.Ns, d.soa)
	}
}

func (d *DNS) isImmediate(q dns.Question) bool {
	qname := strings.ToLower(q.Name)
	query := strings.Split(qname, ".")
	self := strings.Split(d.domain, ".")
	return strings.HasSuffix(qname, d.domain) &&
		len(query) >= len(self) &&
		len(query)-len(self) <= 1
}

func (d *DNS) answer(q dns.Question) (rr []dns.RR, rcode int, auth bool) {
	auth = d.isImmediate(q)
	if !auth {
		return nil, dns.RcodeNameError, true
	}

	if q.Qtype == dns.TypeANY {
		rcode = dns.RcodeNotImplemented
		return
	}

	qname := strings.ToLower(q.Name)
	defined := d.records[qname]
	for _, ri := range defined {
		if ri.Header().Rrtype == q.Qtype {
			rr = append(rr, ri)
		}
	}

	if q.Qtype == dns.TypeTXT {
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

	callCtx, cancel := context.WithTimeout(d.parentCtx, time.Second)
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
