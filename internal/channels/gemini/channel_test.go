package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"goloop/internal/core"
)

// newTestPool creates a DefaultAccountPool with a single test API key.
func newTestPool(apiKey string) *core.DefaultAccountPool {
	pool := core.NewDefaultAccountPool()
	pool.AddAccount(apiKey, 100)
	return pool
}

func TestChannel_ImplementsRawBodyGenerator(t *testing.T) {
	ch := NewChannel("gemini-test", "https://example.com", 100, newTestPool("key"), 10*time.Second)
	var _ core.RawBodyGenerator = ch // compile-time assertion already in channel.go
	if _, ok := interface{}(ch).(core.RawBodyGenerator); !ok {
		t.Fatal("Channel does not implement core.RawBodyGenerator")
	}
}

func TestGenerateRaw_PassthroughBody(t *testing.T) {
	// The upstream echoes back whatever request body it receives, so we can
	// verify that the raw bytes reach the server unchanged.
	const apiKey = "test-api-key-123"

	var capturedBody []byte
	var capturedKey string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("x-goog-api-key")
		var buf [4096]byte
		n, _ := r.Body.Read(buf[:])
		capturedBody = buf[:n]

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Return a minimal valid Google response.
		json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{"content": map[string]any{"parts": []map[string]any{{"text": "ok"}}}},
			},
		})
	}))
	defer server.Close()

	ch := NewChannel("gemini-test", server.URL, 100, newTestPool(apiKey), 10*time.Second)

	reqBody := []byte(`{"contents":[{"parts":[{"text":"画一只猫"}]}],"safetySettings":[{"category":"HARM_CATEGORY_HARASSMENT","threshold":"OFF"}],"generationConfig":{"responseModalities":["image"],"imageConfig":{"aspectRatio":"9:16","imageSize":"2K"}}}`)

	resp, err := ch.GenerateRaw(context.Background(), reqBody, "gemini-3.1-flash-image-preview")
	if err != nil {
		t.Fatalf("GenerateRaw error: %v", err)
	}

	// Verify the API key header was sent correctly.
	if capturedKey != apiKey {
		t.Errorf("x-goog-api-key: got %q, want %q", capturedKey, apiKey)
	}

	// Verify the request body was forwarded verbatim.
	if string(capturedBody) != string(reqBody) {
		t.Errorf("request body mismatch:\ngot:  %s\nwant: %s", capturedBody, reqBody)
	}

	// Verify the response was returned.
	if len(resp) == 0 {
		t.Error("expected non-empty response")
	}
}

func TestGenerateRaw_URLConstruction(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path + ":" + r.URL.RawQuery
		if r.URL.Path == "" {
			capturedPath = r.RequestURI
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"candidates":[]}`))
	}))
	defer server.Close()

	ch := NewChannel("gemini-test", server.URL, 100, newTestPool("key"), 10*time.Second)
	_, err := ch.GenerateRaw(context.Background(), []byte(`{}`), "gemini-2.5-flash-image")
	if err != nil {
		t.Fatalf("GenerateRaw error: %v", err)
	}

	expected := "/v1beta/models/gemini-2.5-flash-image:generateContent"
	if capturedPath != expected+":" {
		// RawQuery is empty so path ends with ":"
		t.Errorf("URL path: got %q, want %q", capturedPath, expected)
	}
}

func TestGenerateRaw_UpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"code":401,"message":"API key not valid"}}`))
	}))
	defer server.Close()

	ch := NewChannel("gemini-test", server.URL, 100, newTestPool("bad-key"), 10*time.Second)
	_, err := ch.GenerateRaw(context.Background(), []byte(`{}`), "gemini-3.1-flash-image-preview")
	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
}

func TestGenerateRaw_NoAccount(t *testing.T) {
	emptyPool := core.NewDefaultAccountPool()
	ch := NewChannel("gemini-test", "https://example.com", 100, emptyPool, 10*time.Second)
	_, err := ch.GenerateRaw(context.Background(), []byte(`{}`), "gemini-3.1-flash-image-preview")
	if err == nil {
		t.Fatal("expected error when pool is empty, got nil")
	}
}

// --- Streaming tests ---

// fakeResponseWriter implements core.ResponseWriter for testing.
type fakeResponseWriter struct {
	headers    http.Header
	statusCode int
	body       []byte
	flushCount int
}

func newFakeResponseWriter() *fakeResponseWriter {
	return &fakeResponseWriter{headers: make(http.Header)}
}

func (f *fakeResponseWriter) Header() http.Header        { return f.headers }
func (f *fakeResponseWriter) Write(b []byte) (int, error) { f.body = append(f.body, b...); return len(b), nil }
func (f *fakeResponseWriter) WriteHeader(code int)        { f.statusCode = code }
func (f *fakeResponseWriter) Flush()                      { f.flushCount++ }

func TestChannel_ImplementsRawStreamGenerator(t *testing.T) {
	ch := NewChannel("gemini-test", "https://example.com", 100, newTestPool("key"), 10*time.Second)
	if _, ok := interface{}(ch).(core.RawStreamGenerator); !ok {
		t.Fatal("Channel does not implement core.RawStreamGenerator")
	}
}

func TestStreamRaw_PipesSSEChunks(t *testing.T) {
	// Simulate an upstream that sends two SSE chunks.
	sseData := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello\"}]}}]}\n\n" +
		"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\" world\"}]}}]}\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify URL uses streamGenerateContent endpoint.
		if !strings.Contains(r.URL.Path, ":streamGenerateContent") {
			t.Errorf("unexpected path %q, want :streamGenerateContent", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer server.Close()

	ch := NewChannel("gemini-test", server.URL, 100, newTestPool("key"), 10*time.Second)
	fw := newFakeResponseWriter()

	err := ch.StreamRaw(context.Background(), []byte(`{}`), "gemini-2.5-flash", fw)
	if err != nil {
		t.Fatalf("StreamRaw error: %v", err)
	}
	if fw.statusCode != http.StatusOK {
		t.Errorf("statusCode: got %d, want %d", fw.statusCode, http.StatusOK)
	}
	if !strings.Contains(string(fw.body), "hello") {
		t.Errorf("response body missing 'hello': %s", fw.body)
	}
	if !strings.Contains(string(fw.body), "world") {
		t.Errorf("response body missing 'world': %s", fw.body)
	}
	if fw.flushCount == 0 {
		t.Error("Flush was never called; streaming would stall for the client")
	}
}

func TestStreamRaw_UpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"code":401,"message":"API key not valid"}}`))
	}))
	defer server.Close()

	ch := NewChannel("gemini-test", server.URL, 100, newTestPool("bad-key"), 10*time.Second)
	fw := newFakeResponseWriter()
	err := ch.StreamRaw(context.Background(), []byte(`{}`), "gemini-2.5-flash", fw)
	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
}

func TestProbe_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1beta/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ch := NewChannel("gemini-test", server.URL, 100, newTestPool("key"), 10*time.Second)
	pool := ch.Pool
	accounts := pool.List()
	if len(accounts) == 0 {
		t.Fatal("pool should have one account")
	}
	if !ch.Probe(accounts[0]) {
		t.Error("Probe should return true for a healthy server")
	}
}
