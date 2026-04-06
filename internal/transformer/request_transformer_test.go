// internal/transformer/request_transformer_test.go
package transformer

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"goloop/internal/config"
	"goloop/internal/model"
	"goloop/internal/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	dir := t.TempDir()
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(srv.Close)
	store, err := storage.NewStore(dir, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

var testModelMapping = map[string]config.ModelDefaults{
	"gemini-3.1-flash-image-preview": {
		KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png",
	},
	"gemini-3-pro-image-preview": {
		KieAIModel: "nano-banana-pro", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png",
	},
}

func TestTransform_TextOnly(t *testing.T) {
	store := newTestStore(t)
	tr := NewRequestTransformer(store, testModelMapping)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{{Text: "a beautiful sunset"}}},
		},
	}

	result, err := tr.Transform(context.Background(), req, "gemini-3.1-flash-image-preview")
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if result.Model != "nano-banana-2" {
		t.Errorf("model: got %q", result.Model)
	}
	if result.Input.Prompt != "a beautiful sunset" {
		t.Errorf("prompt: got %q", result.Input.Prompt)
	}
	if result.Input.AspectRatio != "1:1" {
		t.Errorf("aspect_ratio: got %q", result.Input.AspectRatio)
	}
}

func TestTransform_ImageConfigOverride(t *testing.T) {
	store := newTestStore(t)
	tr := NewRequestTransformer(store, testModelMapping)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{{Text: "test"}}},
		},
		GenerationConfig: &model.GenerationConfig{
			ImageConfig: &model.ImageConfig{AspectRatio: "16:9", ImageSize: "2K"},
		},
	}

	result, err := tr.Transform(context.Background(), req, "gemini-3.1-flash-image-preview")
	if err != nil {
		t.Fatal(err)
	}
	if result.Input.AspectRatio != "16:9" {
		t.Errorf("override aspect_ratio: got %q", result.Input.AspectRatio)
	}
	if result.Input.Resolution != "2K" {
		t.Errorf("override resolution: got %q", result.Input.Resolution)
	}
	// output_format not overridden, use default
	if result.Input.OutputFormat != "png" {
		t.Errorf("default output_format: got %q", result.Input.OutputFormat)
	}
}

func TestTransform_InlineData(t *testing.T) {
	store := newTestStore(t)
	tr := NewRequestTransformer(store, testModelMapping)

	imgBytes := []byte("fake-png-content")
	b64 := base64.StdEncoding.EncodeToString(imgBytes)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{
				{Text: "edit this"},
				{InlineData: &model.InlineData{MimeType: "image/png", Data: b64}},
			}},
		},
	}

	result, err := tr.Transform(context.Background(), req, "gemini-3.1-flash-image-preview")
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if len(result.Input.ImageInput) != 1 {
		t.Fatalf("expected 1 image URL, got %d", len(result.Input.ImageInput))
	}
	savedURL := result.Input.ImageInput[0]
	if savedURL == "" {
		t.Error("empty image URL returned")
	}
	_, _ = os.ReadDir(store.LocalPath())
}

func TestTransform_FileData(t *testing.T) {
	store := newTestStore(t)
	tr := NewRequestTransformer(store, testModelMapping)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{
				{FileData: &model.FileData{MimeType: "image/jpeg", FileURI: "https://example.com/cat.jpg"}},
			}},
		},
	}

	result, err := tr.Transform(context.Background(), req, "gemini-3.1-flash-image-preview")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Input.ImageInput) != 1 || result.Input.ImageInput[0] != "https://example.com/cat.jpg" {
		t.Errorf("fileData URL not preserved: %v", result.Input.ImageInput)
	}
}

func TestTransform_UnknownModel(t *testing.T) {
	store := newTestStore(t)
	tr := NewRequestTransformer(store, testModelMapping)
	_, err := tr.Transform(context.Background(), &model.GoogleRequest{}, "unknown-model")
	if err == nil {
		t.Error("expected error for unknown model")
	}
}

func TestTransform_PromptTooLong(t *testing.T) {
	store := newTestStore(t)
	tr := NewRequestTransformer(store, testModelMapping)

	longText := make([]byte, maxPromptLen+1)
	for i := range longText {
		longText[i] = 'a'
	}

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{{Text: string(longText)}}},
		},
	}
	_, err := tr.Transform(context.Background(), req, "gemini-3.1-flash-image-preview")
	if err == nil {
		t.Error("expected error for prompt too long")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
