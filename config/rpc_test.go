package config

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
)

func TestConfigRPCGet(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	// Exercise the complete wire bound with opaque bytes and a future schema.
	record := testRecord(t, strings.Repeat("x", MaxNameBytes), math.MaxUint64, math.MaxUint64, bytes.Repeat([]byte{0, 0xff}, MaxValueBytes/2))
	require.NoError(t, store.Merge([]Record{record, testRecord(t, "dns", 1, 1, []byte("another document"))}))
	server := httptest.NewServer(protocol.NewConfigServiceServer(NewSyncer(store, nil, nil, nil)))
	defer server.Close()
	client := NewClient(server.URL, server.Client())

	got, err := client.Get(t.Context(), record.Name)
	require.NoError(t, err)
	require.Equal(t, record, got)

	for _, tc := range []struct {
		name string
		code twirp.ErrorCode
	}{
		{name: "absent", code: twirp.NotFound},
		{name: "invalid name", code: twirp.InvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.Get(t.Context(), tc.name)
			var rpcError twirp.Error
			require.ErrorAs(t, err, &rpcError)
			require.Equal(t, tc.code, rpcError.Code())
		})
	}

	require.NoError(t, store.Close())
	_, err = client.Get(t.Context(), record.Name)
	var rpcError twirp.Error
	require.ErrorAs(t, err, &rpcError)
	require.Equal(t, twirp.Unavailable, rpcError.Code())
}

func TestConfigClientRejectsInvalidRecords(t *testing.T) {
	record := testRecord(t, "future.feature", 19, 1, []byte{0, 0xff, 0xfe})
	for _, tc := range []struct {
		name     string
		response *protocol.ConfigRecord
		want     error
	}{
		{
			name:     "invalid version",
			response: &protocol.ConfigRecord{Name: record.Name, Version: record.Version.bytes()[:versionBytes-1], Value: record.Value},
			want:     ErrInvalidVersion,
		},
		{
			name:     "changed value",
			response: &protocol.ConfigRecord{Name: record.Name, Version: record.Version.bytes(), Value: []byte("changed")},
			want:     ErrInvalidRecord,
		},
		{
			name:     "different document",
			response: &protocol.ConfigRecord{Name: "elsewhere", Version: record.Version.bytes(), Value: record.Value},
			want:     ErrInvalidRecord,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := tc.response.MarshalVT()
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/protobuf")
				w.Write(data)
			}))
			defer server.Close()
			_, err = NewClient(server.URL, server.Client()).Get(t.Context(), record.Name)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestConfigClientBoundsSuccessfulAndErrorResponses(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				w.WriteHeader(status)
				w.Write(bytes.Repeat([]byte("x"), maxGetResponseBytes+1))
			}))
			defer server.Close()
			for name, httpClient := range map[string]*http.Client{"custom transport": server.Client(), "default transport": {}} {
				t.Run(name, func(t *testing.T) {
					_, err := NewClient(server.URL, httpClient).Get(t.Context(), "dns")
					var oversized *http.MaxBytesError
					require.ErrorAs(t, err, &oversized, "response must be bounded before generated success/error decoding")
					require.EqualValues(t, maxGetResponseBytes, oversized.Limit)
				})
			}
		})
	}
}

func TestConfigClientDoesNotFollowRedirects(t *testing.T) {
	var redirected, checked atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirected" {
			redirected.Store(true)
			return
		}
		http.Redirect(w, r, "/redirected", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	httpClient := server.Client()
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		checked.Store(true)
		return nil
	}
	_, err := NewClient(server.URL, httpClient).Get(t.Context(), "dns")
	require.Error(t, err)
	require.True(t, checked.Load(), "preserve the caller's redirect callback")
	require.False(t, redirected.Load(), "preserve the generated client's no-redirect behavior")

	response, err := httpClient.Get(server.URL)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.True(t, redirected.Load(), "the caller's shared HTTP client must remain unchanged")
}
