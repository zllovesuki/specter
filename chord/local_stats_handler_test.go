package chord

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatsHTMLTreatsStoredKeysAsText(t *testing.T) {
	as := require.New(t)
	node := NewLocalNode(devConfig(t, as))
	as.NoError(node.Create())
	defer node.Leave()
	waitRing(as, node)
	key := "<script>alert(1)</script>"
	as.NoError(node.kv.Put(t.Context(), []byte(key), []byte("value")))
	handler := ChordStatsHandler(node, []*LocalNode{node})

	htmlResponse := httptest.NewRecorder()
	handler.ServeHTTP(htmlResponse, httptest.NewRequest(http.MethodGet, "/stats.html", nil))
	as.Equal(http.StatusOK, htmlResponse.Code)
	as.Contains(htmlResponse.Body.String(), "&lt;script&gt;")
	as.NotContains(htmlResponse.Body.String(), key)
	as.Contains(htmlResponse.Body.String(), "</pre></body></html>")

	textResponse := httptest.NewRecorder()
	handler.ServeHTTP(textResponse, httptest.NewRequest(http.MethodGet, "/stats.txt", nil))
	as.Equal(http.StatusOK, textResponse.Code)
	as.Contains(textResponse.Body.String(), key)
}
