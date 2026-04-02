// internal/transformer/response_transformer_test.go
package transformer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"goloop/internal/storage"
)

func TestToGoogleResponse_MultipleImages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("fake-png"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	store, _ := storage.NewStore(dir, srv.URL)
	rt := NewResponseTransformer(store)

	urls := []string{srv.URL + "/img1.png", srv.URL + "/img2.png"}
	resp, err := rt.ToGoogleResponse(context.Background(), urls)
	if err != nil {
		t.Fatalf("ToGoogleResponse error: %v", err)
	}

	if len(resp.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(resp.Candidates))
	}
	parts := resp.Candidates[0].Content.Parts
	// text part + 2 image parts
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts (1 text + 2 images), got %d", len(parts))
	}
	if parts[0].Text == "" {
		t.Error("first part should be text")
	}
	for i := 1; i <= 2; i++ {
		if parts[i].InlineData == nil {
			t.Errorf("part %d should be inlineData", i)
		}
		if parts[i].InlineData.MimeType != "image/png" {
			t.Errorf("mimeType: got %q", parts[i].InlineData.MimeType)
		}
	}
	if resp.Candidates[0].FinishReason != "STOP" {
		t.Errorf("finishReason: got %q", resp.Candidates[0].FinishReason)
	}
}

func TestToGoogleError_Mapping(t *testing.T) {
	cases := []struct {
		code       int
		wantHTTP   int
		wantStatus string
	}{
		{401, 401, "UNAUTHENTICATED"},
		{402, 429, "RESOURCE_EXHAUSTED"},
		{429, 429, "RESOURCE_EXHAUSTED"},
		{422, 400, "INVALID_ARGUMENT"},
		{500, 500, "INTERNAL"},
		{501, 500, "INTERNAL"},
	}

	for _, tc := range cases {
		gErr, httpCode := ToGoogleError(tc.code, "test error")
		if httpCode != tc.wantHTTP {
			t.Errorf("code %d: HTTP %d, want %d", tc.code, httpCode, tc.wantHTTP)
		}
		if gErr.Error.Status != tc.wantStatus {
			t.Errorf("code %d: status %q, want %q", tc.code, gErr.Error.Status, tc.wantStatus)
		}
	}
}
