package kieai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"goloop/internal/model"
)

// newMockUploaderServer returns an httptest.Server that replies to
// /api/file-base64-upload with a success payload. The returned counter is
// incremented on every request. The urlFn is given the 1-based call number
// and returns the downloadUrl to respond with; if it returns "", a 503 is
// returned instead (useful for simulating transient failures).
func newMockUploaderServer(t *testing.T, urlFn func(n int32) string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/file-base64-upload" {
			http.NotFound(w, r)
			return
		}
		n := atomic.AddInt32(&hits, 1)
		url := urlFn(n)
		if url == "" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"code":    200,
			"msg":     "ok",
			"data":    map[string]any{"downloadUrl": url},
		})
	}))
	return srv, &hits
}

func TestRequestTransformer_Transform(t *testing.T) {
	mapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	rt := NewRequestTransformer(mapping, nil)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{{Text: "draw a cat"}}},
		},
	}

	kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "test-key")
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
	rt := NewRequestTransformer(mapping, nil)

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

	kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "test-key")
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
	rt := NewRequestTransformer(mapping, nil)

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

	kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "test-key")
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
	rt := NewRequestTransformer(mapping, nil)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{{Text: "draw"}}},
		},
		SafetySettings: []model.SafetySetting{
			{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "OFF"},
		},
	}

	kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "test-key")
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

func TestRequestTransformer_WithFileData(t *testing.T) {
	// Test FileData (external URL) handling without requiring uploader.
	mapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	rt := NewRequestTransformer(mapping, nil)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{
				Parts: []model.Part{
					{Text: "describe this image"},
					{FileData: &model.FileData{MimeType: "image/jpeg", FileURI: "https://example.com/test.jpg"}},
				},
			},
		},
	}

	kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "test-key")
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}

	if kieReq.Input.Prompt != "describe this image" {
		t.Errorf("prompt mismatch: got %q", kieReq.Input.Prompt)
	}

	if len(kieReq.Input.ImageInput) != 1 {
		t.Fatalf("expected 1 image, got %d", len(kieReq.Input.ImageInput))
	}

	if kieReq.Input.ImageInput[0] != "https://example.com/test.jpg" {
		t.Errorf("FileURI mismatch: got %q", kieReq.Input.ImageInput[0])
	}
}

func TestRequestTransformer_InvalidImageURL(t *testing.T) {
	mapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	rt := NewRequestTransformer(mapping, nil)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{
				Parts: []model.Part{
					{Text: "test"},
					{FileData: &model.FileData{MimeType: "image/jpeg", FileURI: "http://localhost/test.jpg"}},
				},
			},
		},
	}

	_, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "test-key")
	if err == nil {
		t.Fatal("expected error for localhost URL (SSRF protection), got nil")
	}
	if !contains(err.Error(), "invalid fileUri") {
		t.Errorf("expected SSRF error, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestRequestTransformer_InlineDataUpload(t *testing.T) {
	srv, hits := newMockUploaderServer(t, func(n int32) string {
		return fmt.Sprintf("https://cdn.kie.ai/u/img-%d.png", n)
	})
	defer srv.Close()

	uploader := NewUploader(srv.URL, 5*time.Second, 3)
	mapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	rt := NewRequestTransformer(mapping, uploader)

	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{
				{Text: "describe this"},
				{InlineData: &model.InlineData{MimeType: "image/png", Data: "AAAA"}},
			}},
		},
	}

	kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "api-key")
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if len(kieReq.Input.ImageInput) != 1 {
		t.Fatalf("expected 1 image, got %d: %v", len(kieReq.Input.ImageInput), kieReq.Input.ImageInput)
	}
	if kieReq.Input.ImageInput[0] != "https://cdn.kie.ai/u/img-1.png" {
		t.Errorf("expected upload URL, got %q", kieReq.Input.ImageInput[0])
	}
	if atomic.LoadInt32(hits) != 1 {
		t.Errorf("expected 1 upload call, got %d", atomic.LoadInt32(hits))
	}
}

// TestRequestTransformer_OrderPreserved exercises the critical fix: when
// FileData and InlineData parts are interleaved, the resulting image_input
// array must preserve their original order.
func TestRequestTransformer_OrderPreserved(t *testing.T) {
	// Use per-request artificial delay so uploads finish out-of-order with
	// high probability, proving that ordering is not accidental.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Extract a marker from the base64 payload to identify which input
		// this request is (payloads are "DATA1", "DATA2", etc.).
		var marker string
		for _, m := range []string{"DATA1", "DATA2", "DATA3"} {
			if strings.Contains(string(body), m) {
				marker = m
				break
			}
		}
		// Later markers resolve faster so responses arrive reversed.
		switch marker {
		case "DATA1":
			time.Sleep(60 * time.Millisecond)
		case "DATA2":
			time.Sleep(30 * time.Millisecond)
		case "DATA3":
			time.Sleep(5 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"code":    200,
			"data":    map[string]any{"downloadUrl": "https://cdn.kie.ai/u/" + marker + ".png"},
		})
	}))
	defer srv.Close()

	uploader := NewUploader(srv.URL, 5*time.Second, 1)
	mapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	rt := NewRequestTransformer(mapping, uploader)

	// Interleaved: [inline DATA1, file B, inline DATA2, file D, inline DATA3]
	req := &model.GoogleRequest{
		Contents: []model.Content{
			{Parts: []model.Part{
				{InlineData: &model.InlineData{MimeType: "image/png", Data: "DATA1"}},
				{FileData: &model.FileData{MimeType: "image/png", FileURI: "https://example.com/B.png"}},
				{InlineData: &model.InlineData{MimeType: "image/png", Data: "DATA2"}},
				{FileData: &model.FileData{MimeType: "image/png", FileURI: "https://example.com/D.png"}},
				{InlineData: &model.InlineData{MimeType: "image/png", Data: "DATA3"}},
			}},
		},
	}

	kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "api-key")
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}

	want := []string{
		"https://cdn.kie.ai/u/DATA1.png",
		"https://example.com/B.png",
		"https://cdn.kie.ai/u/DATA2.png",
		"https://example.com/D.png",
		"https://cdn.kie.ai/u/DATA3.png",
	}
	if len(kieReq.Input.ImageInput) != len(want) {
		t.Fatalf("length mismatch: got %d, want %d: %v", len(kieReq.Input.ImageInput), len(want), kieReq.Input.ImageInput)
	}
	for i, w := range want {
		if kieReq.Input.ImageInput[i] != w {
			t.Errorf("slot %d: got %q, want %q", i, kieReq.Input.ImageInput[i], w)
		}
	}
}

// TestRequestTransformer_ConcurrentUpload verifies that multiple inline
// images are uploaded in parallel rather than serialised.
func TestRequestTransformer_ConcurrentUpload(t *testing.T) {
	const n = 5
	var inflight, maxInflight int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt32(&inflight, 1)
		for {
			mx := atomic.LoadInt32(&maxInflight)
			if cur <= mx || atomic.CompareAndSwapInt32(&maxInflight, mx, cur) {
				break
			}
		}
		// Hold open so a sequential loop would serialise.
		time.Sleep(40 * time.Millisecond)
		atomic.AddInt32(&inflight, -1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"code":    200,
			"data":    map[string]any{"downloadUrl": fmt.Sprintf("https://cdn.kie.ai/u/%d.png", cur)},
		})
	}))
	defer srv.Close()

	uploader := NewUploader(srv.URL, 5*time.Second, 1)
	mapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	rt := NewRequestTransformer(mapping, uploader)

	parts := make([]model.Part, n)
	for i := range parts {
		parts[i] = model.Part{InlineData: &model.InlineData{MimeType: "image/png", Data: fmt.Sprintf("D%d", i)}}
	}
	req := &model.GoogleRequest{Contents: []model.Content{{Parts: parts}}}

	start := time.Now()
	kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "k")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}
	if len(kieReq.Input.ImageInput) != n {
		t.Fatalf("expected %d images, got %d", n, len(kieReq.Input.ImageInput))
	}
	if got := atomic.LoadInt32(&maxInflight); got < 2 {
		t.Errorf("expected concurrent uploads (maxInflight >= 2), got %d", got)
	}
	// Sequential worst case would be ~n*40ms = 200ms. Parallel should stay
	// comfortably under 150ms.
	if elapsed > 150*time.Millisecond {
		t.Errorf("uploads appear serialised; elapsed=%v", elapsed)
	}
}

// TestRequestTransformer_UploadRetryOnTransientFailure proves a single
// inline upload is retried on transient 5xx and eventually succeeds.
func TestRequestTransformer_UploadRetryOnTransientFailure(t *testing.T) {
	srv, hits := newMockUploaderServer(t, func(n int32) string {
		if n < 3 {
			return "" // triggers 503
		}
		return "https://cdn.kie.ai/u/ok.png"
	})
	defer srv.Close()

	uploader := NewUploader(srv.URL, 5*time.Second, 3)
	uploader.retryDelay = 1 * time.Millisecond
	mapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	rt := NewRequestTransformer(mapping, uploader)

	req := &model.GoogleRequest{Contents: []model.Content{{Parts: []model.Part{
		{InlineData: &model.InlineData{MimeType: "image/png", Data: "AAAA"}},
	}}}}

	kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "k")
	if err != nil {
		t.Fatalf("Transform error (should have retried): %v", err)
	}
	if kieReq.Input.ImageInput[0] != "https://cdn.kie.ai/u/ok.png" {
		t.Errorf("unexpected url: %q", kieReq.Input.ImageInput[0])
	}
	if got := atomic.LoadInt32(hits); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

// TestRequestTransformer_UploadPartialFailureAborts verifies that when
// multiple inline images are uploaded concurrently and one exhausts its
// retries, the whole Transform call fails and the other goroutines are
// cancelled early.
func TestRequestTransformer_UploadPartialFailureAborts(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		body, _ := io.ReadAll(r.Body)
		// "POISON" payload always fails with 503; others succeed after a delay.
		if strings.Contains(string(body), "POISON") {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Slow success path — errgroup should cancel these before they finish.
		select {
		case <-r.Context().Done():
			return
		case <-time.After(200 * time.Millisecond):
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "code": 200,
			"data": map[string]any{"downloadUrl": "https://cdn.kie.ai/u/ok.png"},
		})
	}))
	defer srv.Close()

	uploader := NewUploader(srv.URL, 5*time.Second, 2)
	uploader.retryDelay = 1 * time.Millisecond
	mapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	rt := NewRequestTransformer(mapping, uploader)

	req := &model.GoogleRequest{Contents: []model.Content{{Parts: []model.Part{
		{InlineData: &model.InlineData{MimeType: "image/png", Data: "ok1"}},
		{InlineData: &model.InlineData{MimeType: "image/png", Data: "POISON"}},
		{InlineData: &model.InlineData{MimeType: "image/png", Data: "ok2"}},
	}}}}

	start := time.Now()
	_, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "k")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error when one upload exhausts retries")
	}
	if !strings.Contains(err.Error(), "upload inline image") {
		t.Errorf("error should mention upload failure; got %v", err)
	}
	// Since one upload fails fast on 503 and slow ones should be cancelled,
	// the total time should be well under the 200ms slow path (allowing for
	// the retryDelay between attempts).
	if elapsed > 150*time.Millisecond {
		t.Errorf("errgroup did not cancel siblings promptly; elapsed=%v", elapsed)
	}
}

func TestRequestTransformer_InlineDataWithoutUploader(t *testing.T) {
	mapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	rt := NewRequestTransformer(mapping, nil)

	req := &model.GoogleRequest{Contents: []model.Content{{Parts: []model.Part{
		{InlineData: &model.InlineData{MimeType: "image/png", Data: "AAAA"}},
	}}}}

	_, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview", "k")
	if err == nil {
		t.Fatal("expected error when inlineData present but uploader nil")
	}
	if !strings.Contains(err.Error(), "uploader not configured") {
		t.Errorf("expected uploader-missing error; got %v", err)
	}
}