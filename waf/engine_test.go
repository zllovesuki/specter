package waf

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.miragespace.co/specter/spec/protocol"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

func testEngine(t testing.TB, limit int) *Engine {
	t.Helper()
	rules, err := bundledRules()
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(rules, bundledVersion, limit, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}

func multipartRequest(t testing.TB, size int) (string, []byte) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("description", "Holiday photograph"); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("photo", "holiday.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(file, strings.NewReader(strings.Repeat("a", size)), int64(size)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return writer.FormDataContentType(), body.Bytes()
}

func TestEngineBodies(t *testing.T) {
	multipartType, multipartBody := multipartRequest(t, 256<<10)
	largeJSON := []byte(`{"description":"` + strings.Repeat("a", 256<<10) + `"}`)
	largeForm := []byte("description=" + strings.Repeat("%61", 100<<10))
	deepJSON := []byte(strings.Repeat("[", 1025) + "0" + strings.Repeat("]", 1025) + strings.Repeat(" ", 256<<10))
	partialAttack := []byte(`{"search":"1' OR 1=1--","padding":"` + strings.Repeat("a", 256<<10) + `"}`)
	cases := []struct {
		name        string
		contentType string
		body        []byte
		status      int
	}{
		{"form", "application/x-www-form-urlencoded", []byte("name=Rachel&message=Hello+world"), http.StatusNoContent},
		{"json", "application/json", []byte(`{"name":"Rachel","message":"Hello world"}`), http.StatusNoContent},
		{"large_form", "application/x-www-form-urlencoded", largeForm, http.StatusNoContent},
		{"large_json", "application/json", largeJSON, http.StatusNoContent},
		{"multipart", multipartType, multipartBody, http.StatusNoContent},
		{"malformed_json", "application/json", []byte(`{"message":`), http.StatusBadRequest},
		{"partial_json_depth_limit", "application/json", deepJSON, http.StatusBadRequest},
		{"body_sqli", "application/x-www-form-urlencoded", []byte("search=" + url.QueryEscape("1' OR 1=1--")), http.StatusForbidden},
		{"partial_json_sqli", "application/json", partialAttack, http.StatusForbidden},
	}
	engine := testEngine(t, DefaultBodyLimit)
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "https://example.com/submit", bytes.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			forwarded := false
			engine.ServeHTTP(response, request, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				forwarded = true
				body, err := io.ReadAll(r.Body)
				if err != nil || !bytes.Equal(body, test.body) {
					t.Fatalf("replayed body differs: bytes=%d err=%v", len(body), err)
				}
				w.WriteHeader(http.StatusNoContent)
			}), "example.com", protocol.WAFPolicy_BLOCK, nil)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
			}
			if forwarded != (test.status == http.StatusNoContent) {
				t.Fatalf("forwarded=%t", forwarded)
			}
			if len(engine.slots) != 0 {
				t.Fatal("inspection admission was not released")
			}
		})
	}
}

func TestEngineRequests(t *testing.T) {
	engine := testEngine(t, DefaultBodyLimit)
	attack := url.QueryEscape("1' OR '1'='1' -- private-payload")
	ctx := context.WithValue(t.Context(), middleware.RequestIDKey, "gateway/000001")
	request := func(path string, mode protocol.WAFPolicy_Mode, ids []int) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://example.com"+path+"?search="+attack, nil)
		r.Header.Set("X-Request-ID", "visitor-request-id")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}), "test-tunnel", mode, ids)
		return w
	}
	t.Run("exceptions and monitor", func(t *testing.T) {
		blocked := request("/search", protocol.WAFPolicy_BLOCK, nil)
		if blocked.Code != http.StatusForbidden {
			t.Fatalf("SQLi status=%d body=%s", blocked.Code, blocked.Body.String())
		}
		var response denial
		if err := json.Unmarshal(blocked.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.RequestID != "gateway/000001" || response.Version != bundledVersion || len(response.Rules) == 0 {
			t.Fatalf("missing safe denial information: %+v", response)
		}
		if strings.Contains(blocked.Body.String(), "private-payload") || strings.Contains(blocked.Body.String(), "/search") {
			t.Fatal("denial leaked request data")
		}
		policy := &protocol.WAFPolicy{Mode: protocol.WAFPolicy_BLOCK}
		for _, rule := range response.Rules {
			if !rule.Excludable || rule.ID == 949110 {
				t.Fatalf("non-detection rule in SQLi denial: %+v", rule)
			}
			policy.Exclusions = append(policy.Exclusions, &protocol.WAFExclusion{RuleId: uint32(rule.ID), Path: "/search"})
		}
		if w := request("/search", protocol.WAFPolicy_BLOCK, DisabledRules(policy, "/search")); w.Code != http.StatusNoContent {
			t.Fatalf("exact-path exception failed: %d %s", w.Code, w.Body.String())
		}
		if w := request("/elsewhere", protocol.WAFPolicy_BLOCK, DisabledRules(policy, "/elsewhere")); w.Code != http.StatusForbidden {
			t.Fatal("exception escaped its exact path")
		}
		if w := request("/search", protocol.WAFPolicy_BLOCK, []int{949110, 200002, 999999}); w.Code != http.StatusForbidden {
			t.Fatal("protected or absent rule ID weakened enforcement")
		}
		if w := request("/search", protocol.WAFPolicy_MONITOR, nil); w.Code != http.StatusNoContent {
			t.Fatal("monitor blocked the request")
		}
	})
	t.Run("streaming body", func(t *testing.T) {
		stream := &readProbe{Reader: strings.NewReader("stream content")}
		r := httptest.NewRequest(http.MethodPost, "https://example.com/service", stream)
		r.Header.Set("Content-Type", "application/grpc+proto")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if stream.reads != 0 {
				t.Fatal("gRPC stream was read before forwarding")
			}
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusNoContent)
		}), "example.com", protocol.WAFPolicy_BLOCK, nil)
		if w.Code != http.StatusNoContent {
			t.Fatalf("gRPC metadata-only request blocked: %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("unread body holds admission", func(t *testing.T) {
		// An upstream that does not consume its request body must keep the copied
		// prefix charged to the admission limit, even though Coraza has closed.
		slots := engine.slots
		engine.slots = make(chan struct{}, 1)
		defer func() { engine.slots = slots }()
		func() {
			entered, resume, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			go func() {
				defer close(finished)
				r := httptest.NewRequest(http.MethodPost, "https://example.com/submit", strings.NewReader(`{"message":"Hello"}`))
				r.Header.Set("Content-Type", "application/json")
				engine.ServeHTTP(httptest.NewRecorder(), r, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					close(entered)
					<-resume
				}), "example.com", protocol.WAFPolicy_BLOCK, nil)
			}()
			defer func() { close(resume); <-finished }()
			<-entered
			if w := request("/search", protocol.WAFPolicy_BLOCK, nil); w.Code != http.StatusServiceUnavailable {
				t.Fatalf("retained prefix did not hold admission: %d", w.Code)
			}
		}()
		if len(engine.slots) != 0 {
			t.Fatal("closing an unread replay body did not release admission")
		}
	})
	t.Run("retired generation", func(t *testing.T) {
		_ = engine.Close()
		if w := request("/search", protocol.WAFPolicy_BLOCK, nil); w.Code != http.StatusForbidden {
			t.Fatal("retirement interrupted an already-selected generation")
		}
	})
}

type readProbe struct {
	io.Reader
	reads int
}

func (r *readProbe) Read(p []byte) (int, error) {
	r.reads++
	return r.Reader.Read(p)
}
