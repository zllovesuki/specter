package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestProxyErrorHandlerEOFAndCancellation(t *testing.T) {
	g := &Gateway{GatewayConfig: GatewayConfig{Logger: zap.NewNop()}}
	for _, tc := range []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"EOF", io.EOF, http.StatusBadGateway, "An unexpected error has occurred while attempting to forward to destination."},
		{"canceled", context.Canceled, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil)
			rw := httptest.NewRecorder()
			rw.Code = 0 // Distinguish an unwritten response from an implicit 200.
			g.errorHandler(rw, req, tc.err)
			require.Equal(t, tc.status, rw.Code)
			require.Equal(t, tc.body, rw.Body.String())
		})
	}
}
