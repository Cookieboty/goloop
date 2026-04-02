// internal/model/google_test.go
package model

import (
	"encoding/json"
	"testing"
)

func TestGoogleRequestUnmarshal(t *testing.T) {
	raw := `{
        "contents": [{"parts": [
            {"text": "draw a cat"},
            {"inlineData": {"mimeType": "image/png", "data": "abc123"}},
            {"fileData": {"mimeType": "image/jpeg", "fileUri": "https://example.com/img.jpg"}}
        ]}],
        "generationConfig": {
            "responseModalities": ["TEXT", "IMAGE"],
            "imageConfig": {"aspectRatio": "16:9", "resolution": "2K", "outputFormat": "png"}
        }
    }`

	var req GoogleRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(req.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(req.Contents))
	}
	parts := req.Contents[0].Parts
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}
	if parts[0].Text != "draw a cat" {
		t.Errorf("text mismatch: %q", parts[0].Text)
	}
	if parts[1].InlineData == nil || parts[1].InlineData.Data != "abc123" {
		t.Errorf("inlineData mismatch")
	}
	if parts[2].FileData == nil || parts[2].FileData.FileURI != "https://example.com/img.jpg" {
		t.Errorf("fileData mismatch")
	}
	if req.GenerationConfig == nil || req.GenerationConfig.ImageConfig == nil {
		t.Fatal("generationConfig/imageConfig is nil")
	}
	if req.GenerationConfig.ImageConfig.AspectRatio != "16:9" {
		t.Errorf("aspectRatio mismatch")
	}
}

func TestGoogleResponseMarshal(t *testing.T) {
	resp := GoogleResponse{
		Candidates: []Candidate{
			{
				Content: Content{
					Parts: []Part{
						{Text: "here is the image"},
						{InlineData: &InlineData{MimeType: "image/png", Data: "base64data"}},
					},
				},
				FinishReason: "STOP",
			},
		},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var check GoogleResponse
	if err := json.Unmarshal(b, &check); err != nil {
		t.Fatalf("round-trip unmarshal error: %v", err)
	}
	if check.Candidates[0].FinishReason != "STOP" {
		t.Errorf("finishReason mismatch")
	}
}

func TestGoogleErrorMarshal(t *testing.T) {
	e := GoogleError{
		Error: GoogleErrorDetail{Code: 401, Message: "Invalid API key", Status: "UNAUTHENTICATED"},
	}
	b, _ := json.Marshal(e)
	expected := `{"error":{"code":401,"message":"Invalid API key","status":"UNAUTHENTICATED"}}`
	if string(b) != expected {
		t.Errorf("error JSON mismatch:\ngot:  %s\nwant: %s", b, expected)
	}
}
