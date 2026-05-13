// internal/transformer/request_transformer_test.go
package transformer

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"goloop/internal/core"
	"goloop/internal/model"
	"goloop/internal/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	dir := t.TempDir()
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(srv.Close)
	store, err := storage.NewStore(dir, srv.URL, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func newTestConfigManager() *core.ConfigManager {
	return core.NewConfigManager(nil)
}

func TestTransform_TextOnly(t *testing.T) {
	store := newTestStore(t)
	tr := NewRequestTransformer(store, newTestConfigManager(), 0)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{{Text: "a beautiful sunset"}}},
		},
	}

	result, err := tr.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "test-channel")
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
	tr := NewRequestTransformer(store, newTestConfigManager(), 0)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{{Text: "test"}}},
		},
		GenerationConfig: &model.GenerationConfig{
			ImageConfig: &model.ImageConfig{AspectRatio: "16:9", ImageSize: "2K"},
		},
	}

	result, err := tr.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "test-channel")
	if err != nil {
		t.Fatal(err)
	}
	if result.Input.AspectRatio != "16:9" {
		t.Errorf("override aspect_ratio: got %q", result.Input.AspectRatio)
	}
	if result.Input.Resolution != "2K" {
		t.Errorf("override resolution: got %q", result.Input.Resolution)
	}
	if result.Input.OutputFormat != "png" {
		t.Errorf("default output_format: got %q", result.Input.OutputFormat)
	}
}

func TestTransform_InlineData(t *testing.T) {
	store := newTestStore(t)
	tr := NewRequestTransformer(store, newTestConfigManager(), 0)

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

	result, err := tr.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "test-channel")
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
	tr := NewRequestTransformer(store, newTestConfigManager(), 0)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{
				{FileData: &model.FileData{MimeType: "image/jpeg", FileURI: "https://example.com/cat.jpg"}},
			}},
		},
	}

	result, err := tr.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "test-channel")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Input.ImageInput) != 1 || result.Input.ImageInput[0] != "https://example.com/cat.jpg" {
		t.Errorf("fileData URL not preserved: %v", result.Input.ImageInput)
	}
}

func TestTransform_UnknownModel(t *testing.T) {
	store := newTestStore(t)
	tr := NewRequestTransformer(store, newTestConfigManager(), 0)
	result, err := tr.Transform(context.Background(), &model.GoogleRequest{}, "unknown-model", "test-channel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Model != "unknown-model" {
		t.Errorf("expected pass-through model name, got %q", result.Model)
	}
}

func TestTransform_PromptTooLong(t *testing.T) {
	store := newTestStore(t)
	tr := NewRequestTransformer(store, newTestConfigManager(), 0)

	longText := make([]byte, maxPromptLen+1)
	for i := range longText {
		longText[i] = 'a'
	}

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{{Text: string(longText)}}},
		},
	}
	_, err := tr.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "test-channel")
	if err == nil {
		t.Error("expected error for prompt too long")
	}
}
