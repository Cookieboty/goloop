package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"goloop/internal/core"
)

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

func TestMissingJWT_Returns401(t *testing.T) {
	mux := http.NewServeMux()
	h := &GeminiHandler{issuer: core.NewJWTIssuer("secret", time.Hour)}
	protected := core.NewJWTMiddleware(h.issuer, h.handleProtected)
	mux.Handle("POST /v1beta/models/", protected)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1beta/models/gemini-3.1-flash-image-preview:generateContent",
		strings.NewReader(`{"contents":[]}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestIsStreamingRequest(t *testing.T) {
	tests := []struct {
		accept string
		expect bool
	}{
		{"text/event-stream", true},
		{"application/json", false},
		{"", false},
		{"text/event-stream; charset=utf-8", true},
		{"multipart/x-mixed-replace", true},
	}

	for _, tt := range tests {
		r := &http.Request{Header: http.Header{"Accept": []string{tt.accept}}}
		if got := isStreamingRequest(r); got != tt.expect {
			t.Errorf("isStreamingRequest(%q) = %v, want %v", tt.accept, got, tt.expect)
		}
	}
}
