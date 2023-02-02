package cloudflare

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/libdns/libdns"
)

// Provider implements the libdns interfaces for Cloudflare.
// TODO: Support pagination and retries, handle rate limits.
type Provider struct {
	// API token is used for authentication. Make sure to use a
	// scoped API **token**, NOT a global API **key**. It will
	// need two permissions: Zone-Zone-Read and Zone-DNS-Edit,
	// unless you are only using `GetRecords()`, in which case
	// the second can be changed to Read.
	APIToken string `json:"api_token,omitempty"`
	// Root Zone points to the actual hosted zone for the TXT records.
	// If you are hosting secure.example.com's acme challenge, then
	// _acme-challenge.secure.example.com should be CNAME to
	// secure.example.com.{RootZone}.
	RootZone string `json:"root_zone,omitempty"`

	zoneCache *cfZone
	zoneMu    sync.Mutex
}

const (
	stripToken = "_acme-challenge."
	slen       = len(stripToken)
)

func strip(name string) string {
	if strings.HasPrefix(name, stripToken) {
		return name[slen:]
	}
	return name
}

func rewrite(name, zone string) string {
	fqdn := libdns.AbsoluteName(name, zone)
	return strip(fqdn)
}

func (p *Provider) Validate() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	_, err := p.getZoneInfo(ctx)
	return err
}

// AppendRecords adds records to the zone. It returns the records that were added.
func (p *Provider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	zoneInfo, err := p.getZoneInfo(ctx)
	if err != nil {
		return nil, err
	}

	var created []libdns.Record
	for _, rec := range records {
		rec.Name = rewrite(rec.Name, zone)
		result, err := p.createRecord(ctx, zoneInfo, rec)
		if err != nil {
			return nil, err
		}
		created = append(created, result.libdnsRecord(zone))
	}

	return created, nil
}

// DeleteRecords deletes the records from the zone. If a record does not have an ID,
// it will be looked up. It returns the records that were deleted.
func (p *Provider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	zoneInfo, err := p.getZoneInfo(ctx)
	if err != nil {
		return nil, err
	}

	var recs []libdns.Record
	for _, rec := range records {
		rec.Name = libdns.RelativeName(rec.Name, p.RootZone)
		// we create a "delete queue" for each record
		// requested for deletion; if the record ID
		// is known, that is the only one to fill the
		// queue, but if it's not known, we try to find
		// a match theoretically there could be more
		// than one
		var deleteQueue []libdns.Record

		if rec.ID == "" {
			// record ID is required; try to find it with what was provided
			exactMatches, err := p.getDNSRecords(ctx, zoneInfo, rec, true)
			if err != nil {
				return nil, err
			}
			for _, rec := range exactMatches {
				deleteQueue = append(deleteQueue, rec.libdnsRecord(zone))
			}
		} else {
			deleteQueue = []libdns.Record{rec}
		}

		for _, delRec := range deleteQueue {
			reqURL := fmt.Sprintf("%s/zones/%s/dns_records/%s", baseURL, zoneInfo.ID, delRec.ID)
			req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
			if err != nil {
				return nil, err
			}

			var result cfDNSRecord
			_, err = p.doAPIRequest(req, &result)
			if err != nil {
				return nil, err
			}

			recs = append(recs, result.libdnsRecord(zone))
		}

	}

	return recs, nil
}

// Interface guards
var (
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordDeleter  = (*Provider)(nil)
)
