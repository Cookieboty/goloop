package kieai

import (
	"context"
	"testing"

	"goloop/internal/model"
)

func TestRequestTransformer_Transform(t *testing.T) {
	mapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	rt := NewRequestTransformer(mapping)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{{Text: "draw a cat"}}},
		},
	}

	kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview")
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if kieReq.Model != "nano-banana-2" {
		t.Errorf("model: got %q", kieReq.Model)
	}
	if kieReq.Input.Prompt != "draw a cat" {
		t.Errorf("prompt: got %q", kieReq.Input.Prompt)
	}
	if kieReq.Input.AspectRatio != "1:1" {
		t.Errorf("aspect_ratio mismatch")
	}
}