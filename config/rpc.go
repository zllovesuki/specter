package config

import (
	"context"
	"fmt"
	"net/http"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/util"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/twitchtv/twirp"
)

const (
	// MaxGetRequestBytes bounds keyed reads before the generated handler decodes them.
	MaxGetRequestBytes = 4 << 10
	// Three length-delimited fields add their tags and length prefixes.
	maxGetResponseBytes = MaxNameBytes + versionBytes + MaxValueBytes + 9
)

// ReadHandler exposes only keyed reads on the local RPC listener. Peer exchange
// remains available only through the authenticated configuration transport.
func ReadHandler(service protocol.ConfigService) http.Handler {
	server := protocol.NewConfigServiceServer(service)
	read := middleware.SetHeader("Cache-Control", "no-store")(util.LimitBody(MaxGetRequestBytes)(server))
	mux := http.NewServeMux()
	mux.Handle(server.PathPrefix()+"Get", read)
	return mux
}

// Get reads one document from the local durable replica without interpreting its schema.
func (s *Syncer) Get(_ context.Context, request *protocol.ConfigGetRequest) (*protocol.ConfigRecord, error) {
	name := request.GetName()
	if !validName(name) {
		return nil, twirp.InvalidArgument.Error("invalid configuration document name")
	}
	record, found, err := s.store.Get(name)
	if err != nil {
		return nil, twirp.Unavailable.Error(err.Error())
	}
	if !found {
		return nil, twirp.NotFound.Error("configuration document not found")
	}
	return &protocol.ConfigRecord{Name: record.Name, Version: record.Version.bytes(), Value: record.Value}, nil
}

// Client reads complete, validated documents through the generated protobuf RPC.
// Callers own request cancellation and deadlines.
type Client struct {
	service protocol.ConfigService
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{service: protocol.NewConfigServiceProtobufClient(baseURL, boundedClient(httpClient, maxGetResponseBytes))}
}

func (c *Client) Get(ctx context.Context, name string) (Record, error) {
	response, err := c.service.Get(ctx, &protocol.ConfigGetRequest{Name: name})
	if err != nil {
		return Record{}, err
	}
	version, err := decodeVersion(response.GetVersion())
	if err != nil {
		return Record{}, err
	}
	record := Record{Name: response.GetName(), Version: version, Value: response.GetValue()}
	if err := record.Validate(); err != nil {
		return Record{}, err
	}
	if record.Name != name {
		return Record{}, fmt.Errorf("%w: requested document %q, received %q", ErrInvalidRecord, name, record.Name)
	}
	return record, nil
}

// Bound successful and error responses before generated Twirp decoding reads
// them. The request bound is installed on the receiving HTTP handler.
type boundedRoundTripper struct {
	http.RoundTripper
	limit int64
}

func boundedClient(client *http.Client, limit int64) *http.Client {
	copy := *client
	transport := copy.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	// Keep the concrete client visible so Twirp owns its redirect policy.
	copy.Transport = boundedRoundTripper{RoundTripper: transport, limit: limit}
	return &copy
}

func (t boundedRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.RoundTripper.RoundTrip(request)
	if err == nil && response != nil && response.Body != nil {
		response.Body = http.MaxBytesReader(nil, response.Body, t.limit)
	}
	return response, err
}
