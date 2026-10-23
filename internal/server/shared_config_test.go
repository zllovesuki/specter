package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.miragespace.co/specter/config"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
)

func TestSharedConfigLocalRPCIsReadOnlyAndPreservesOtherRoutes(t *testing.T) {
	store, err := config.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	record, err := store.Put("future/policy", 7, []byte("opaque bytes"), "")
	require.NoError(t, err)
	handler := localRPCHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/twirp/protocol.KVService/Get" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}), config.NewSyncer(store, nil, nil, nil))
	server := httptest.NewServer(handler)
	defer server.Close()
	reader := config.NewClient(server.URL, server.Client())
	got, err := reader.Get(t.Context(), record.Name)
	require.NoError(t, err)
	require.Equal(t, record, got)

	client := protocol.NewConfigServiceProtobufClient(server.URL, server.Client())
	mutation, err := config.NewRecord("forbidden", 1, 1, nil)
	require.NoError(t, err)
	snapshot, err := config.EncodeSnapshot([]config.Record{mutation})
	require.NoError(t, err)
	_, err = client.Exchange(t.Context(), &protocol.ConfigSnapshot{Data: snapshot})
	var rpcError twirp.Error
	require.ErrorAs(t, err, &rpcError)
	require.Equal(t, twirp.BadRoute, rpcError.Code(), "peer exchange must not be mounted on the local listener")
	_, found, err := store.Get(mutation.Name)
	require.NoError(t, err)
	require.False(t, found)

	_, err = client.Get(t.Context(), &protocol.ConfigGetRequest{Name: strings.Repeat("x", config.MaxGetRequestBytes)})
	require.ErrorContains(t, err, "request body")

	request := httptest.NewRequest(http.MethodPost, "/twirp/protocol.ConfigService/Get", strings.NewReader(`{"name":"future/policy"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/twirp/protocol.KVService/Get", nil))
	require.Equal(t, http.StatusAccepted, response.Code)
}
