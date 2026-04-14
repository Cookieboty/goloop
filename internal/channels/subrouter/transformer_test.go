package subrouter

import (
	"testing"

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
