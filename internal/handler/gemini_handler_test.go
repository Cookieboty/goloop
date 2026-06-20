package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestMissingAPIKey_Returns401(t *testing.T) {
	h := &GeminiHandler{}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1beta/models/gemini-3.1-flash-image-preview:generateContent",
		strings.NewReader(`{"contents":[]}`))
	req.Header.Set("Content-Type", "application/json")

	h.handleGenerateContent(w, req)

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

func TestUpstreamStatus(t *testing.T) {
	if got := upstreamStatus(&core.UpstreamStatusError{Status: 400, Body: []byte("bad request")}); got != 400 {
		t.Errorf("upstreamStatus(UpstreamStatusError 400) = %d, want 400", got)
	}
	if got := upstreamStatus(fmt.Errorf("wrapped: %w", &core.UpstreamStatusError{Status: 429})); got != 429 {
		t.Errorf("upstreamStatus(wrapped 429) = %d, want 429", got)
	}
	if got := upstreamStatus(errors.New("transport failure")); got != 0 {
		t.Errorf("upstreamStatus(plain err) = %d, want 0", got)
	}
	if got := upstreamStatus(nil); got != 0 {
		t.Errorf("upstreamStatus(nil) = %d, want 0", got)
	}
}
