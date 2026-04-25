package openai_original

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"goloop/internal/core"
)

func TestGenerateOpenAIRaw_Success(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
			t.Errorf("expected Bearer auth, got %q", got)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Ratelimit-Remaining-Requests", "99")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-123"}`))
	}))
	defer upstream.Close()

	ch := newTestChannel(t, upstream.URL)

	resp, err := ch.GenerateOpenAIRaw(context.Background(), "", []byte(`{"model":"gpt-4"}`), "/v1/chat/completions")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.Status)
	}
	if resp.Headers.Get("X-Ratelimit-Remaining-Requests") != "99" {
		t.Errorf("rate-limit header not propagated")
	}
	if !strings.Contains(string(resp.Body), "chatcmpl-123") {
		t.Errorf("unexpected body: %s", resp.Body)
	}
}

// Non-2xx upstream responses must now surface as OpenAIRawResponse (not error),
// so the handler can decide whether to fall back or propagate to the client.
func TestGenerateOpenAIRaw_Non2xxReturnedAsResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad prompt"}}`))
	}))
	defer upstream.Close()

	ch := newTestChannel(t, upstream.URL)

	resp, err := ch.GenerateOpenAIRaw(context.Background(), "", []byte(`{}`), "/v1/images/generations")
	if err != nil {
		t.Fatalf("non-2xx should not return error: %v", err)
	}
	if resp.Status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.Status)
	}
	if !strings.Contains(string(resp.Body), "bad prompt") {
		t.Errorf("body not propagated: %s", resp.Body)
	}
}

// Multipart/form-data requests must forward the client's Content-Type (which
// includes the boundary) verbatim — this is the /v1/images/edits path.
func TestGenerateOpenAIRaw_MultipartContentTypePassthrough(t *testing.T) {
	var gotCT string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	ch := newTestChannel(t, upstream.URL)

	ct := "multipart/form-data; boundary=----abc123"
	_, err := ch.GenerateOpenAIRaw(context.Background(), ct, []byte("payload"), "/v1/images/edits")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotCT != ct {
		t.Errorf("content-type not forwarded: got %q, want %q", gotCT, ct)
	}
}

func TestStreamOpenAIRaw_SuccessStreamsBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"delta\":\"hi\"}\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer upstream.Close()

	ch := newTestChannel(t, upstream.URL)
	recorder := httptest.NewRecorder()
	rw := &mockResponseWriter{ResponseWriter: recorder}

	err := ch.StreamOpenAIRaw(context.Background(), "", []byte(`{"stream":true}`), "/v1/chat/completions", rw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "[DONE]") {
		t.Errorf("stream body missing [DONE]: %s", recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type not mirrored: %q", recorder.Header().Get("Content-Type"))
	}
}

// Pre-commit upstream 4xx must return *UpstreamStatusError so the handler can
// propagate the status/body to the client without falling back.
func TestStreamOpenAIRaw_Non2xxReturnsStatusError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
	}))
	defer upstream.Close()

	ch := newTestChannel(t, upstream.URL)
	recorder := httptest.NewRecorder()
	rw := &mockResponseWriter{ResponseWriter: recorder}

	err := ch.StreamOpenAIRaw(context.Background(), "", []byte(`{"stream":true}`), "/v1/chat/completions", rw)
	if err == nil {
		t.Fatal("expected error on pre-commit non-2xx")
	}
	var se *UpstreamStatusError
	if !errors.As(err, &se) {
		t.Fatalf("expected *UpstreamStatusError, got %T: %v", err, err)
	}
	if se.Status != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", se.Status)
	}
	if !strings.Contains(string(se.Body), "slow down") {
		t.Errorf("body not captured: %s", se.Body)
	}
	// Handler must NOT have been given a committed response.
	if recorder.Code != http.StatusOK { // httptest default until WriteHeader is called
		t.Errorf("headers should not be committed before error; got %d", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("no bytes should have been written, got %q", recorder.Body.String())
	}
}

func TestProbe(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer upstream.Close()

	ch := newTestChannel(t, upstream.URL)
	acc, _ := ch.Pool.Select()
	if !ch.Probe(acc) {
		t.Error("probe should succeed on 200 /v1/models")
	}
}

func TestChannelType(t *testing.T) {
	ch := newTestChannel(t, "https://api.example.com")
	if ch.Type() != "gpt-image" {
		t.Errorf("type = %q, want gpt-image", ch.Type())
	}
}

func newTestChannel(t *testing.T, baseURL string) *Channel {
	t.Helper()
	pool := core.NewDefaultAccountPool()
	pool.AddAccount("test-api-key", 1)
	return NewChannel("test", baseURL, 10, pool, 5*time.Second)
}

type mockResponseWriter struct {
	http.ResponseWriter
}

func (m *mockResponseWriter) Flush() {
	if f, ok := m.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
