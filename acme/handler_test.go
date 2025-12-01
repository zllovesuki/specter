package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.miragespace.co/specter/kv/memory"
	"go.miragespace.co/specter/spec/chord"

	"github.com/caddyserver/certmagic"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func generateTestCert(t *testing.T, domain string) (certPEM, keyPEM []byte) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: domain,
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(24 * time.Hour),
		DNSNames:  []string{domain},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM
}

func setupTestStorage(t *testing.T) *ChordStorage {
	t.Helper()
	logger := zaptest.NewLogger(t)
	kv := memory.WithHashFn(chord.Hash)

	storage, err := NewChordStorage(logger, kv, StorageConfig{
		RetryInterval: time.Millisecond * 100,
		LeaseTTL:      time.Minute,
	})
	require.NoError(t, err)

	return storage
}

func TestHandlerListIssuers(t *testing.T) {
	as := require.New(t)
	ctx := context.Background()

	storage := setupTestStorage(t)

	// Store test certificates
	issuer1 := "acme-v02.api.letsencrypt.org-directory"
	issuer2 := "acme.zerossl.com-v2-dv90"

	certPEM, keyPEM := generateTestCert(t, "example.com")
	meta := []byte(`{"url":"https://acme.example.com/order/123"}`)

	// Store certs for issuer1
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SiteCert(issuer1, "example.com"), certPEM))
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SitePrivateKey(issuer1, "example.com"), keyPEM))
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SiteMeta(issuer1, "example.com"), meta))

	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SiteCert(issuer1, "test.com"), certPEM))
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SitePrivateKey(issuer1, "test.com"), keyPEM))

	// Store certs for issuer2
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SiteCert(issuer2, "other.com"), certPEM))
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SitePrivateKey(issuer2, "other.com"), keyPEM))

	// Create manager with minimal config (just to get the handler)
	m := &Manager{chordStorage: storage}
	handler := AcmeManagerHandler(m)

	// Test GET /certs
	req := httptest.NewRequest("GET", "/certs", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	as.Equal(http.StatusOK, w.Result().StatusCode)

	var resp issuersResponse
	as.NoError(json.NewDecoder(w.Body).Decode(&resp))

	as.Len(resp.Issuers, 2)

	// Check counts
	issuerMap := make(map[string]int)
	for _, i := range resp.Issuers {
		issuerMap[i.Issuer] = i.Count
	}
	as.Equal(2, issuerMap[issuer1])
	as.Equal(1, issuerMap[issuer2])
}

func TestHandlerListCerts(t *testing.T) {
	as := require.New(t)
	ctx := context.Background()

	storage := setupTestStorage(t)

	issuer := "acme-v02.api.letsencrypt.org-directory"
	certPEM, keyPEM := generateTestCert(t, "example.com")

	// Store regular cert
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SiteCert(issuer, "example.com"), certPEM))
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SitePrivateKey(issuer, "example.com"), keyPEM))

	// Store wildcard cert
	wildcardCert, wildcardKey := generateTestCert(t, "*.example.com")
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SiteCert(issuer, "*.example.com"), wildcardCert))
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SitePrivateKey(issuer, "*.example.com"), wildcardKey))

	m := &Manager{chordStorage: storage}
	handler := AcmeManagerHandler(m)

	// Test GET /certs/{issuer}
	req := httptest.NewRequest("GET", "/certs/"+issuer, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	as.Equal(http.StatusOK, w.Result().StatusCode)

	var resp certsListResponse
	as.NoError(json.NewDecoder(w.Body).Decode(&resp))

	as.Equal(issuer, resp.Issuer)
	as.Equal(2, resp.Count)
	as.Len(resp.Certs, 2)

	// Check that wildcard is properly displayed
	certMap := make(map[string]string)
	for _, c := range resp.Certs {
		certMap[c.Name] = c.Domain
	}

	as.Equal("example.com", certMap["example.com"])
	as.Equal("*.example.com", certMap["wildcard_.example.com"])
}

func TestHandlerInspectCert(t *testing.T) {
	as := require.New(t)
	ctx := context.Background()

	storage := setupTestStorage(t)

	issuer := "acme-v02.api.letsencrypt.org-directory"
	certPEM, keyPEM := generateTestCert(t, "example.com")
	meta := []byte(`{"url":"https://acme.example.com/order/123","domain":"example.com"}`)

	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SiteCert(issuer, "example.com"), certPEM))
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SitePrivateKey(issuer, "example.com"), keyPEM))
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SiteMeta(issuer, "example.com"), meta))

	m := &Manager{chordStorage: storage}
	handler := AcmeManagerHandler(m)

	// Test GET /certs/{issuer}/{name}
	req := httptest.NewRequest("GET", "/certs/"+issuer+"/example.com", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	as.Equal(http.StatusOK, w.Result().StatusCode)

	var resp certInspectResponse
	as.NoError(json.NewDecoder(w.Body).Decode(&resp))

	as.Equal(issuer, resp.IssuerKey)
	as.Equal("example.com", resp.Name)
	as.Equal("example.com", resp.Domain)
	as.Contains(resp.DNSNames, "example.com")
	as.False(resp.NotBefore.IsZero())
	as.False(resp.NotAfter.IsZero())
	as.NotEmpty(resp.SerialNumber)
	as.NotEmpty(resp.SignatureAlgorithm)
	as.Equal("example.com", resp.Subject.CommonName)
	as.Equal("ECDSA (P-256)", resp.PublicKey.Algorithm)
	as.Equal(256, resp.PublicKey.Size)
	as.Equal(3, resp.Version)
	as.NotNil(resp.Metadata)
}

func TestHandlerInspectWildcardCert(t *testing.T) {
	as := require.New(t)
	ctx := context.Background()

	storage := setupTestStorage(t)

	issuer := "acme-v02.api.letsencrypt.org-directory"
	certPEM, keyPEM := generateTestCert(t, "*.example.com")

	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SiteCert(issuer, "*.example.com"), certPEM))
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SitePrivateKey(issuer, "*.example.com"), keyPEM))

	m := &Manager{chordStorage: storage}
	handler := AcmeManagerHandler(m)

	// Test with safe name in URL
	req := httptest.NewRequest("GET", "/certs/"+issuer+"/wildcard_.example.com", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	as.Equal(http.StatusOK, w.Result().StatusCode)

	var resp certInspectResponse
	as.NoError(json.NewDecoder(w.Body).Decode(&resp))

	as.Equal("wildcard_.example.com", resp.Name)
	as.Equal("*.example.com", resp.Domain)
	as.Contains(resp.DNSNames, "*.example.com")
}

func TestHandlerInspectCertNotFound(t *testing.T) {
	as := require.New(t)

	storage := setupTestStorage(t)

	m := &Manager{chordStorage: storage}
	handler := AcmeManagerHandler(m)

	req := httptest.NewRequest("GET", "/certs/some-issuer/nonexistent.com", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	as.Equal(http.StatusNotFound, w.Result().StatusCode)
}

func TestHandlerDeleteCert(t *testing.T) {
	as := require.New(t)
	ctx := context.Background()

	storage := setupTestStorage(t)

	issuer := "acme-v02.api.letsencrypt.org-directory"
	certPEM, keyPEM := generateTestCert(t, "example.com")
	meta := []byte(`{"url":"https://acme.example.com/order/123"}`)

	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SiteCert(issuer, "example.com"), certPEM))
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SitePrivateKey(issuer, "example.com"), keyPEM))
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SiteMeta(issuer, "example.com"), meta))

	// Verify files exist
	as.True(storage.Exists(ctx, certmagic.StorageKeys.SiteCert(issuer, "example.com")))
	as.True(storage.Exists(ctx, certmagic.StorageKeys.SitePrivateKey(issuer, "example.com")))
	as.True(storage.Exists(ctx, certmagic.StorageKeys.SiteMeta(issuer, "example.com")))

	m := &Manager{chordStorage: storage}
	handler := AcmeManagerHandler(m)

	// Test DELETE /certs/{issuer}/{name}
	req := httptest.NewRequest("DELETE", "/certs/"+issuer+"/example.com", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	as.Equal(http.StatusNoContent, w.Result().StatusCode)

	// Verify files are deleted
	as.False(storage.Exists(ctx, certmagic.StorageKeys.SiteCert(issuer, "example.com")))
	as.False(storage.Exists(ctx, certmagic.StorageKeys.SitePrivateKey(issuer, "example.com")))
	as.False(storage.Exists(ctx, certmagic.StorageKeys.SiteMeta(issuer, "example.com")))
}

func TestHandlerDeleteWildcardCert(t *testing.T) {
	as := require.New(t)
	ctx := context.Background()

	storage := setupTestStorage(t)

	issuer := "acme-v02.api.letsencrypt.org-directory"
	certPEM, keyPEM := generateTestCert(t, "*.example.com")

	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SiteCert(issuer, "*.example.com"), certPEM))
	as.NoError(storage.Store(ctx, certmagic.StorageKeys.SitePrivateKey(issuer, "*.example.com"), keyPEM))

	m := &Manager{chordStorage: storage}
	handler := AcmeManagerHandler(m)

	// Test DELETE with safe name
	req := httptest.NewRequest("DELETE", "/certs/"+issuer+"/wildcard_.example.com", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	as.Equal(http.StatusNoContent, w.Result().StatusCode)

	// Verify files are deleted
	as.False(storage.Exists(ctx, certmagic.StorageKeys.SiteCert(issuer, "*.example.com")))
	as.False(storage.Exists(ctx, certmagic.StorageKeys.SitePrivateKey(issuer, "*.example.com")))
}

func TestHandlerDeleteNonexistent(t *testing.T) {
	as := require.New(t)

	storage := setupTestStorage(t)

	m := &Manager{chordStorage: storage}
	handler := AcmeManagerHandler(m)

	// Deleting nonexistent cert should still return 204
	req := httptest.NewRequest("DELETE", "/certs/some-issuer/nonexistent.com", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	as.Equal(http.StatusNoContent, w.Result().StatusCode)
}

func TestHandlerEmptyIssuers(t *testing.T) {
	as := require.New(t)

	storage := setupTestStorage(t)

	m := &Manager{chordStorage: storage}
	handler := AcmeManagerHandler(m)

	req := httptest.NewRequest("GET", "/certs", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	as.Equal(http.StatusOK, w.Result().StatusCode)

	var resp issuersResponse
	as.NoError(json.NewDecoder(w.Body).Decode(&resp))
	as.Empty(resp.Issuers)
}

func TestUnsafeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"wildcard_.example.com", "*.example.com"},
		{"wildcard_.test.example.com", "*.test.example.com"},
		{"sub.example.com", "sub.example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := unsafeName(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}
