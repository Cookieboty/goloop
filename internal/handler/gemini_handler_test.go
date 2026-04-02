// internal/handler/gemini_handler_test.go
package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractAPIKey_XGoogHeader(t *testing.T) {
	r, _ := http.NewRequest("POST", "/", nil)
	r.Header.Set("x-goog-api-key", "my-secret-key")
	if got := extractAPIKey(r); got != "my-secret-key" {
		t.Errorf("got %q", got)
	}
}

func TestExtractAPIKey_BearerToken(t *testing.T) {
	r, _ := http.NewRequest("POST", "/", nil)
	r.Header.Set("Authorization", "Bearer bearer-token-123")
	if got := extractAPIKey(r); got != "bearer-token-123" {
		t.Errorf("got %q", got)
	}
}

func TestExtractAPIKey_Missing(t *testing.T) {
	r, _ := http.NewRequest("POST", "/", nil)
	if got := extractAPIKey(r); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestHandleHealth(t *testing.T) {
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/health", nil)
	h := &GeminiHandler{}
	h.handleHealth(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("code: got %d", w.Code)
	}
	if w.Body.String() != `{"status":"ok"}` {
		t.Errorf("body: got %q", w.Body.String())
	}
}

func TestMissingAPIKey_Returns401(t *testing.T) {
	mux := http.NewServeMux()
	h := &GeminiHandler{}
	mux.HandleFunc("POST /v1beta/models/", h.handleGenerateContent)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1beta/models/gemini-3.1-flash-image-preview:generateContent",
		strings.NewReader(`{"contents":[]}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
