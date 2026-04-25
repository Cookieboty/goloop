package gemini_openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"goloop/internal/core"
	"goloop/internal/model"
)

func ptr[T any](v T) *T { return &v }

func TestGoogleToOpenAI_BasicText(t *testing.T) {
	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Role: "user", Parts: []model.Part{{Text: "hello"}}},
		},
	}
	chatReq := googleToOpenAI(req, "gpt-4o")

	if chatReq.Model != "gpt-4o" {
		t.Errorf("model mismatch: %q", chatReq.Model)
	}
	if len(chatReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Role != "user" {
		t.Errorf("role mismatch: %q", chatReq.Messages[0].Role)
	}
	if chatReq.Messages[0].Content != "hello" {
		t.Errorf("content mismatch: %v", chatReq.Messages[0].Content)
	}
}

func TestGoogleToOpenAI_SystemInstruction(t *testing.T) {
	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Role: "user", Parts: []model.Part{{Text: "hi"}}},
		},
		SystemInstruction: &model.Content{
			Parts: []model.Part{{Text: "You are a helpful assistant."}},
		},
	}
	chatReq := googleToOpenAI(req, "gpt-4o")

	if len(chatReq.Messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Role != "system" {
		t.Errorf("first message should be system, got %q", chatReq.Messages[0].Role)
	}
	if chatReq.Messages[0].Content != "You are a helpful assistant." {
		t.Errorf("system content mismatch: %v", chatReq.Messages[0].Content)
	}
}

func TestGoogleToOpenAI_GenerationConfig(t *testing.T) {
	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Role: "user", Parts: []model.Part{{Text: "hello"}}},
		},
		GenerationConfig: &model.GenerationConfig{
			Temperature:      ptr(0.7),
			TopP:             ptr(0.9),
			MaxOutputTokens:  ptr(512),
			CandidateCount:   ptr(1),
			StopSequences:    []string{"END"},
			PresencePenalty:  ptr(0.1),
			FrequencyPenalty: ptr(0.2),
			Seed:             ptr(42),
		},
	}
	chatReq := googleToOpenAI(req, "gpt-4o")

	if chatReq.Temperature == nil || *chatReq.Temperature != 0.7 {
		t.Errorf("temperature mismatch: %v", chatReq.Temperature)
	}
	if chatReq.TopP == nil || *chatReq.TopP != 0.9 {
		t.Errorf("top_p mismatch: %v", chatReq.TopP)
	}
	if chatReq.MaxTokens == nil || *chatReq.MaxTokens != 512 {
		t.Errorf("max_tokens mismatch: %v", chatReq.MaxTokens)
	}
	if chatReq.N == nil || *chatReq.N != 1 {
		t.Errorf("n mismatch: %v", chatReq.N)
	}
	if len(chatReq.Stop) != 1 || chatReq.Stop[0] != "END" {
		t.Errorf("stop mismatch: %v", chatReq.Stop)
	}
	if chatReq.Seed == nil || *chatReq.Seed != 42 {
		t.Errorf("seed mismatch: %v", chatReq.Seed)
	}
}

func TestGoogleToOpenAI_ResponseMimeTypeJSON(t *testing.T) {
	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Role: "user", Parts: []model.Part{{Text: "output json"}}},
		},
		GenerationConfig: &model.GenerationConfig{
			ResponseMimeType: "application/json",
		},
	}
	chatReq := googleToOpenAI(req, "gpt-4o")

	if chatReq.ResponseFormat == nil {
		t.Fatal("response_format should be set for application/json")
	}
	if chatReq.ResponseFormat.Type != "json_object" {
		t.Errorf("response_format.type mismatch: %q", chatReq.ResponseFormat.Type)
	}
}

func TestGoogleToOpenAI_SafetySettingsNotMapped(t *testing.T) {
	// safetySettings have no OpenAI equivalent; request should still succeed
	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Role: "user", Parts: []model.Part{{Text: "hello"}}},
		},
		SafetySettings: []model.SafetySetting{
			{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "OFF"},
		},
	}
	chatReq := googleToOpenAI(req, "gpt-4o")

	if len(chatReq.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(chatReq.Messages))
	}
}

// --- Stream (StreamGenerator) tests ---

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

func (f *fakeResponseWriter) Header() http.Header         { return f.headers }
func (f *fakeResponseWriter) Write(b []byte) (int, error) { f.body = append(f.body, b...); return len(b), nil }
func (f *fakeResponseWriter) WriteHeader(code int)         { f.statusCode = code }
func (f *fakeResponseWriter) Flush()                       { f.flushCount++ }

func newTestPool(apiKey string) *core.DefaultAccountPool {
	pool := core.NewDefaultAccountPool()
	pool.AddAccount(apiKey, 100)
	return pool
}

func TestChannel_ImplementsStreamGenerator(t *testing.T) {
	ch := NewChannel("test", "https://example.com", 100, newTestPool("key"), 10*time.Second, Config{})
	if _, ok := interface{}(ch).(core.StreamGenerator); !ok {
		t.Fatal("subrouter.Channel does not implement core.StreamGenerator")
	}
}

func TestStream_ConvertOpenAISSEToGoogleSSE(t *testing.T) {
	// Build a fake OpenAI SSE response with two delta chunks and a final DONE.
	chunk1 := `{"id":"1","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`
	chunk2 := `{"id":"1","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":"stop"}]}`
	sseBody := fmt.Sprintf("data: %s\n\ndata: %s\n\ndata: [DONE]\n\n", chunk1, chunk2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the upstream request has stream:true set.
		if !strings.Contains(r.URL.Path, "/v1/chat/completions") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseBody))
	}))
	defer server.Close()

	ch := NewChannel("test", server.URL, 100, newTestPool("key"), 10*time.Second, Config{})
	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Role: "user", Parts: []model.Part{{Text: "hi"}}},
		},
	}
	fw := newFakeResponseWriter()
	err := ch.Stream(context.Background(), req, "gpt-4o", fw)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	if fw.statusCode != http.StatusOK {
		t.Errorf("statusCode: got %d, want 200", fw.statusCode)
	}
	out := string(fw.body)
	if !strings.Contains(out, "hello") {
		t.Errorf("output missing 'hello': %s", out)
	}
	if !strings.Contains(out, "world") {
		t.Errorf("output missing 'world': %s", out)
	}
	if !strings.Contains(out, "[DONE]") {
		t.Errorf("output missing [DONE] sentinel: %s", out)
	}
	// Each non-empty chunk should trigger a Flush.
	if fw.flushCount == 0 {
		t.Error("Flush was never called; streaming would stall for the client")
	}
	// Verify output is in Google SSE format (contains "candidates").
	if !strings.Contains(out, "candidates") {
		t.Errorf("output not in Google format (missing 'candidates'): %s", out)
	}
}

func TestStream_UpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid key"}}`))
	}))
	defer server.Close()

	ch := NewChannel("test", server.URL, 100, newTestPool("bad"), 10*time.Second, Config{})
	req := &model.GoogleRequest{
		Contents: []model.Content{{Role: "user", Parts: []model.Part{{Text: "hi"}}}},
	}
	fw := newFakeResponseWriter()
	err := ch.Stream(context.Background(), req, "gpt-4o", fw)
	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
}
