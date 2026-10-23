package acme

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"go.miragespace.co/specter/config"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func configTestStore(t *testing.T) *config.Store {
	t.Helper()
	store, err := config.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	return store
}

func configTestDNS(address string) DNSConfig {
	return DNSConfig{Zone: "acme.example.com", Nameservers: []DNSNameserver{{Host: "ns.acme.example.com", Addresses: []string{address}}}}
}

func configTestPut(t *testing.T, handler http.Handler, version string, config DNSConfig) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(map[string]any{"expected_version": version, "config": config})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPut, "/dns", bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestSharedDNSOperatorEditing(t *testing.T) {
	store := configTestStore(t)
	handler := DNSConfigHandler(store, &protocol.Node{Id: 1, Address: "node.example.com:443"}, "acme.example.com", zap.NewNop(), nil)
	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/dns", nil))
	require.Equal(t, http.StatusOK, read.Code)
	var empty dnsConfigResponse
	require.NoError(t, json.Unmarshal(read.Body.Bytes(), &empty))
	require.Nil(t, empty.Config)
	require.Empty(t, empty.Version)

	save := configTestPut(t, handler, "", configTestDNS("192.0.2.1"))
	require.Equal(t, http.StatusOK, save.Code, save.Body.String())
	var first dnsConfigResponse
	require.NoError(t, json.Unmarshal(save.Body.Bytes(), &first))
	require.NotEmpty(t, first.Version)
	require.Equal(t, "acme.example.com.", first.Config.Zone)
	require.Equal(t, "node.example.com:443", first.Server.Address)
	require.False(t, first.ObservedAt.IsZero())

	save = configTestPut(t, handler, first.Version, configTestDNS("192.0.2.2"))
	require.Equal(t, http.StatusOK, save.Code, save.Body.String())
	var second dnsConfigResponse
	require.NoError(t, json.Unmarshal(save.Body.Bytes(), &second))
	require.NotEqual(t, first.Version, second.Version)
	require.Equal(t, []string{"192.0.2.2"}, second.Config.Nameservers[0].Addresses)

	stale := configTestPut(t, handler, first.Version, configTestDNS("192.0.2.3"))
	require.Equal(t, http.StatusConflict, stale.Code)
	require.Contains(t, stale.Body.String(), "version_conflict")
	record, _, err := store.Get(DNSConfigName)
	require.NoError(t, err)
	require.Equal(t, second.Version, record.Token())
}

func TestSharedDNSValidationAndUnsupportedSchema(t *testing.T) {
	store := configTestStore(t)
	handler := DNSConfigHandler(store, &protocol.Node{}, "", zap.NewNop(), nil)
	bad := configTestPut(t, handler, "", configTestDNS("not an IP"))
	require.Equal(t, http.StatusBadRequest, bad.Code)
	require.Contains(t, bad.Body.String(), "nameservers.0.addresses.0")
	_, found, err := store.Get(DNSConfigName)
	require.NoError(t, err)
	require.False(t, found)

	good := configTestPut(t, handler, "", configTestDNS("192.0.2.1"))
	require.Equal(t, http.StatusOK, good.Code)
	current, _, err := store.Get(DNSConfigName)
	require.NoError(t, err)
	otherZone := configTestDNS("192.0.2.2")
	otherZone.Zone = "other.example.com"
	rejected := configTestPut(t, handler, current.Token(), otherZone)
	require.Equal(t, http.StatusBadRequest, rejected.Code)
	require.Contains(t, rejected.Body.String(), `"zone"`)

	future, err := config.NewRecord(DNSConfigName, 2, 1, []byte(`{"future":"opaque"}`))
	require.NoError(t, err)
	require.NoError(t, store.Merge([]config.Record{future}))
	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/dns", nil))
	var response dnsConfigResponse
	require.NoError(t, json.Unmarshal(read.Body.Bytes(), &response))
	require.Equal(t, future.Token(), response.Version)
	require.Nil(t, response.Config)
	require.Contains(t, response.Error, "unsupported")
	rejected = configTestPut(t, handler, future.Token(), configTestDNS("192.0.2.3"))
	require.Equal(t, http.StatusConflict, rejected.Code)
	stored, _, err := store.Get(DNSConfigName)
	require.NoError(t, err)
	require.Equal(t, future, stored)
}

func TestSharedDNSReadDoesNotReturnOlderObservedVersion(t *testing.T) {
	store := configTestStore(t)
	handler := DNSConfigHandler(store, &protocol.Node{}, "", zap.NewNop(), nil)
	payload, err := json.Marshal(configTestDNS("192.0.2.1"))
	require.NoError(t, err)
	future, err := config.NewRecord(DNSConfigName, 1, 2, payload)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/dns?after="+url.QueryEscape(future.Token()), nil).WithContext(ctx)
	read := httptest.NewRecorder()
	handler.ServeHTTP(read, request)
	require.NoError(t, ctx.Err(), "a replica below the version floor must respond without waiting for synchronization")
	require.Equal(t, http.StatusServiceUnavailable, read.Code)
	require.Contains(t, read.Body.String(), "not_ready")

	read = httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/dns?after=invalid", nil))
	require.Equal(t, http.StatusBadRequest, read.Code, "a missing document must still validate the requested version")

	previous, err := config.NewRecord(DNSConfigName, 1, 1, payload)
	require.NoError(t, err)
	require.NoError(t, store.Merge([]config.Record{previous}))
	read = httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, request.URL.String(), nil))
	require.Equal(t, http.StatusServiceUnavailable, read.Code)
	require.Contains(t, read.Body.String(), "not_ready")

	require.NoError(t, store.Merge([]config.Record{future}))
	read = httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, request.URL.String(), nil))
	require.Equal(t, http.StatusOK, read.Code)
	var response dnsConfigResponse
	require.NoError(t, json.Unmarshal(read.Body.Bytes(), &response))
	require.Equal(t, future.Token(), response.Version)

	read = httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/dns?after="+url.QueryEscape(previous.Token()), nil))
	require.Equal(t, http.StatusOK, read.Code, "a replica ahead of the version floor can serve its newer document")

	read = httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/dns?after=invalid", nil))
	require.Equal(t, http.StatusBadRequest, read.Code)
}

func TestSharedDNSRejectsUnboundedOrAmbiguousInput(t *testing.T) {
	store := configTestStore(t)
	handler := DNSConfigHandler(store, &protocol.Node{}, "", zap.NewNop(), nil)
	data, err := json.Marshal(map[string]any{"config": configTestDNS("192.0.2.1")})
	require.NoError(t, err)
	valid := string(data)
	for _, body := range []string{
		valid[:len(valid)-1] + `,"unexpected":true}`,
		valid + ` {}`,
		string(bytes.Repeat([]byte(" "), config.MaxValueBytes+4096)) + valid,
	} {
		request := httptest.NewRequest(http.MethodPut, "/dns", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, http.StatusBadRequest, response.Code)
	}
	records, err := store.Snapshot()
	require.NoError(t, err)
	require.Empty(t, records)
}

func TestSharedDNSStorageFailureIsUnavailable(t *testing.T) {
	store := configTestStore(t)
	handler := DNSConfigHandler(store, &protocol.Node{}, "", zap.NewNop(), nil)
	first := configTestPut(t, handler, "", configTestDNS("192.0.2.1"))
	require.Equal(t, http.StatusOK, first.Code)
	before, _, err := store.Get(DNSConfigName)
	require.NoError(t, err)
	require.NoError(t, store.Close())

	refresh := httptest.NewRecorder()
	handler.ServeHTTP(refresh, httptest.NewRequest(http.MethodGet, "/dns", nil))
	require.Equal(t, http.StatusServiceUnavailable, refresh.Code)
	require.Contains(t, refresh.Body.String(), "not_ready")
	failed := configTestPut(t, handler, before.Token(), configTestDNS("192.0.2.2"))
	require.Equal(t, http.StatusServiceUnavailable, failed.Code)
}
