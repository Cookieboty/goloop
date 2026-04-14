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

func TestRequestTransformer_ImageConfigOverride(t *testing.T) {
	mapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	rt := NewRequestTransformer(mapping)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{{Text: "画一只猫"}}},
		},
		GenerationConfig: &model.GenerationConfig{
			ResponseModalities: []string{"image"},
			ImageConfig: &model.ImageConfig{
				AspectRatio: "9:16",
				ImageSize:   "2K",
			},
		},
	}

	kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview")
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if kieReq.Input.AspectRatio != "9:16" {
		t.Errorf("aspectRatio: expected 9:16, got %q", kieReq.Input.AspectRatio)
	}
	if kieReq.Input.Resolution != "2K" {
		t.Errorf("resolution: expected 2K, got %q", kieReq.Input.Resolution)
	}
	// OutputFormat should retain default since not overridden
	if kieReq.Input.OutputFormat != "png" {
		t.Errorf("outputFormat: expected png, got %q", kieReq.Input.OutputFormat)
	}
}

func TestRequestTransformer_ImageOutputOptionsOverride(t *testing.T) {
	mapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	rt := NewRequestTransformer(mapping)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{{Text: "draw"}}},
		},
		GenerationConfig: &model.GenerationConfig{
			ImageConfig: &model.ImageConfig{
				ImageOutputOptions: &model.ImageOutputOptions{MimeType: "image/jpeg"},
				OutputFormat:       "png", // should be overridden by ImageOutputOptions
			},
		},
	}

	kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview")
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if kieReq.Input.OutputFormat != "image/jpeg" {
		t.Errorf("outputFormat: expected image/jpeg (from imageOutputOptions), got %q", kieReq.Input.OutputFormat)
	}
}

func TestRequestTransformer_SafetySettingsIgnored(t *testing.T) {
	// safetySettings are parsed but not forwarded to KIE (KIE handles safety internally).
	mapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	rt := NewRequestTransformer(mapping)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{{Text: "draw"}}},
		},
		SafetySettings: []model.SafetySetting{
			{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "OFF"},
		},
	}

	kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview")
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if kieReq == nil {
		t.Fatal("expected non-nil kieReq")
	}
	// KIE request should be valid; safetySettings have no KIE equivalent
	if kieReq.Input.Prompt != "draw" {
		t.Errorf("prompt mismatch: %q", kieReq.Input.Prompt)
	}
}