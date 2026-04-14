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
            "imageConfig": {"aspectRatio": "16:9", "imageSize": "2K", "outputFormat": "png"}
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

func TestGoogleRequestSafetySettingsUnmarshal(t *testing.T) {
	raw := `{
        "contents": [{"parts": [{"text": "画一只猫"}]}],
        "safetySettings": [
            {"category": "HARM_CATEGORY_HARASSMENT", "threshold": "OFF"},
            {"category": "HARM_CATEGORY_HATE_SPEECH", "threshold": "OFF"},
            {"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT", "threshold": "OFF"},
            {"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "OFF"}
        ],
        "generationConfig": {
            "responseModalities": ["image"],
            "imageConfig": {"aspectRatio": "9:16", "imageSize": "2K"}
        }
    }`

	var req GoogleRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(req.SafetySettings) != 4 {
		t.Fatalf("expected 4 safetySettings, got %d", len(req.SafetySettings))
	}
	if req.SafetySettings[0].Category != "HARM_CATEGORY_HARASSMENT" {
		t.Errorf("category mismatch: %q", req.SafetySettings[0].Category)
	}
	if req.SafetySettings[0].Threshold != "OFF" {
		t.Errorf("threshold mismatch: %q", req.SafetySettings[0].Threshold)
	}
	if req.GenerationConfig == nil {
		t.Fatal("generationConfig is nil")
	}
	if len(req.GenerationConfig.ResponseModalities) != 1 || req.GenerationConfig.ResponseModalities[0] != "image" {
		t.Errorf("responseModalities mismatch: %v", req.GenerationConfig.ResponseModalities)
	}
	if req.GenerationConfig.ImageConfig == nil {
		t.Fatal("imageConfig is nil")
	}
	if req.GenerationConfig.ImageConfig.AspectRatio != "9:16" {
		t.Errorf("aspectRatio mismatch: %q", req.GenerationConfig.ImageConfig.AspectRatio)
	}
	if req.GenerationConfig.ImageConfig.ImageSize != "2K" {
		t.Errorf("imageSize mismatch: %q", req.GenerationConfig.ImageConfig.ImageSize)
	}
}

func TestGoogleRequestGenerationConfigUnmarshal(t *testing.T) {
	temp := 0.7
	maxTokens := 1024
	seed := 42
	raw := `{
        "contents": [{"parts": [{"text": "hello"}]}],
        "generationConfig": {
            "temperature": 0.7,
            "topP": 0.9,
            "maxOutputTokens": 1024,
            "seed": 42,
            "responseMimeType": "application/json",
            "stopSequences": ["END", "STOP"],
            "presencePenalty": 0.1,
            "frequencyPenalty": 0.2
        }
    }`

	var req GoogleRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	gc := req.GenerationConfig
	if gc == nil {
		t.Fatal("generationConfig is nil")
	}
	if gc.Temperature == nil || *gc.Temperature != temp {
		t.Errorf("temperature mismatch: %v", gc.Temperature)
	}
	if gc.MaxOutputTokens == nil || *gc.MaxOutputTokens != maxTokens {
		t.Errorf("maxOutputTokens mismatch: %v", gc.MaxOutputTokens)
	}
	if gc.Seed == nil || *gc.Seed != seed {
		t.Errorf("seed mismatch: %v", gc.Seed)
	}
	if gc.ResponseMimeType != "application/json" {
		t.Errorf("responseMimeType mismatch: %q", gc.ResponseMimeType)
	}
	if len(gc.StopSequences) != 2 || gc.StopSequences[0] != "END" {
		t.Errorf("stopSequences mismatch: %v", gc.StopSequences)
	}
}

func TestGoogleRequestSystemInstructionUnmarshal(t *testing.T) {
	raw := `{
        "contents": [{"parts": [{"text": "hello"}], "role": "user"}],
        "systemInstruction": {"parts": [{"text": "You are a helpful assistant."}]}
    }`

	var req GoogleRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if req.SystemInstruction == nil {
		t.Fatal("systemInstruction is nil")
	}
	if len(req.SystemInstruction.Parts) != 1 {
		t.Fatalf("expected 1 part in systemInstruction, got %d", len(req.SystemInstruction.Parts))
	}
	if req.SystemInstruction.Parts[0].Text != "You are a helpful assistant." {
		t.Errorf("systemInstruction text mismatch: %q", req.SystemInstruction.Parts[0].Text)
	}
}

func TestImageConfigImageOutputOptionsUnmarshal(t *testing.T) {
	raw := `{
        "contents": [{"parts": [{"text": "draw"}]}],
        "generationConfig": {
            "imageConfig": {
                "aspectRatio": "1:1",
                "imageSize": "1K",
                "imageOutputOptions": {"mimeType": "image/jpeg", "compressionQuality": 85},
                "personGeneration": "ALLOW_ADULT",
                "prominentPeople": "BLOCK_PROMINENT_PEOPLE"
            }
        }
    }`

	var req GoogleRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	ic := req.GenerationConfig.ImageConfig
	if ic == nil {
		t.Fatal("imageConfig is nil")
	}
	if ic.ImageOutputOptions == nil {
		t.Fatal("imageOutputOptions is nil")
	}
	if ic.ImageOutputOptions.MimeType != "image/jpeg" {
		t.Errorf("mimeType mismatch: %q", ic.ImageOutputOptions.MimeType)
	}
	if *ic.ImageOutputOptions.CompressionQuality != 85 {
		t.Errorf("compressionQuality mismatch: %v", ic.ImageOutputOptions.CompressionQuality)
	}
	if ic.PersonGeneration != "ALLOW_ADULT" {
		t.Errorf("personGeneration mismatch: %q", ic.PersonGeneration)
	}
	if ic.ProminentPeople != "BLOCK_PROMINENT_PEOPLE" {
		t.Errorf("prominentPeople mismatch: %q", ic.ProminentPeople)
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
	if check.Candidates[0].Content.Parts[1].InlineData == nil {
		t.Error("InlineData part lost in round-trip")
	}
	if check.Candidates[0].Content.Parts[1].InlineData.Data != "base64data" {
		t.Errorf("InlineData.Data mismatch: %q", check.Candidates[0].Content.Parts[1].InlineData.Data)
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
