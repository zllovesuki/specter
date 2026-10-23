package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.miragespace.co/specter/acme"
	"go.miragespace.co/specter/config"

	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
)

const testZone = "acme.example.com"

func dnsRecord(t *testing.T, schema, revision uint64, value string) config.Record {
	t.Helper()
	if value == "" {
		value = `{"zone":"acme.example.com","nameservers":[{"host":"ns.acme.example.com","addresses":["192.0.2.1"]}]}`
	}
	record, err := config.NewRecord(acme.DNSConfigName, schema, revision, []byte(value))
	require.NoError(t, err)
	return record
}

func TestWaitDNSConfigWaitsForUsableLocalDocument(t *testing.T) {
	good := dnsRecord(t, 1, 1, "")
	future := dnsRecord(t, 2, 2, "")
	wrongZone := dnsRecord(t, 1, 3, `{"zone":"other.example.com","nameservers":[{"host":"ns.other.example.com","addresses":["192.0.2.1"]}]}`)
	requests := 0
	load := func(context.Context) (config.Record, error) {
		requests++
		switch requests {
		case 1:
			return config.Record{}, twirp.NotFound.Error("missing")
		case 2:
			return config.Record{}, twirp.Unavailable.Error("unavailable")
		case 3:
			return future, nil
		case 4:
			return wrongZone, nil
		default:
			return good, nil
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	record, err := waitDNSConfig(ctx, zap.NewNop(), load, testZone, time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, good.Token(), record.Token())
	require.Equal(t, 5, requests)
}

func TestWaitDNSConfigCancellation(t *testing.T) {
	requested := make(chan struct{}, 1)
	load := func(context.Context) (config.Record, error) {
		requested <- struct{}{}
		return config.Record{}, twirp.NotFound.Error("missing")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := waitDNSConfig(ctx, zap.NewNop(), load, testZone, time.Hour)
		done <- err
	}()
	select {
	case <-requested:
	case <-time.After(time.Second):
		t.Fatal("configuration request did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("startup wait did not stop on cancellation")
	}
}

func TestRefreshDNSConfigRetainsWorkingVersionAndRecovers(t *testing.T) {
	initial := dnsRecord(t, 1, 1, "")
	handler, err := acme.NewDNS(t.Context(), zap.NewNop(), nil, "issuer@example.com", testZone, initial)
	require.NoError(t, err)
	type response struct {
		record config.Record
		err    error
	}
	var current atomic.Pointer[response]
	current.Store(&response{err: twirp.Unavailable.Error("unavailable")})
	var requests atomic.Int32
	load := func(context.Context) (config.Record, error) {
		r := current.Load()
		requests.Add(1)
		return r.record, r.err
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		refreshDNSConfig(ctx, zap.NewNop(), load, handler, time.Millisecond)
		close(done)
	}()
	require.Eventually(t, func() bool { return requests.Load() > 1 }, time.Second, time.Millisecond)
	require.Equal(t, initial.Token(), handler.ConfigVersion())
	before := requests.Load()
	current.Store(&response{record: dnsRecord(t, 2, 2, "")})
	require.Eventually(t, func() bool { return requests.Load() > before+2 }, time.Second, time.Millisecond)
	require.Equal(t, initial.Token(), handler.ConfigVersion())
	updated := dnsRecord(t, 1, 3, `{"zone":"acme.example.com","nameservers":[{"host":"ns.acme.example.com","addresses":["192.0.2.3"]}]}`)
	current.Store(&response{record: updated})
	require.Eventually(t, func() bool { return handler.ConfigVersion() == updated.Token() }, time.Second, time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("configuration refresh did not stop on cancellation")
	}
}

func TestLoadDNSConfigExplainsMissingDocumentAndUsesDeadline(t *testing.T) {
	client := config.NewClient("http://rpc", &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		deadline, ok := request.Context().Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), dnsConfigTimeout)
		response := httptest.NewRecorder()
		twirp.WriteError(response, twirp.NotFound.Error("missing"))
		return response.Result(), nil
	})})
	_, err := loadDNSConfig(t.Context(), client)
	require.ErrorContains(t, err, "specter config init")

	client = config.NewClient("http://rpc", &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	_, err = loadDNSConfig(ctx, client)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
