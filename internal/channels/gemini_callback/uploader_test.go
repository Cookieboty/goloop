package gemini_callback

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func writeUploadSuccess(t *testing.T, w http.ResponseWriter, downloadURL string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"code":    200,
		"msg":     "ok",
		"data":    map[string]any{"downloadUrl": downloadURL},
	})
}

func TestUploader_Success(t *testing.T) {
	var gotAuth, gotContentType, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		writeUploadSuccess(t, w, "https://cdn.kie.ai/u/abc.png")
	}))
	defer srv.Close()

	u := NewUploader(srv.URL, 5*time.Second, 3)
	url, err := u.UploadBase64(context.Background(), "api-key-xyz", "iVBORw0KGgo=", "image/png")
	if err != nil {
		t.Fatalf("UploadBase64 error: %v", err)
	}
	if url != "https://cdn.kie.ai/u/abc.png" {
		t.Errorf("url mismatch: got %q", url)
	}
	if gotAuth != "Bearer api-key-xyz" {
		t.Errorf("auth header: got %q", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Errorf("content-type: got %q", gotContentType)
	}
	if !strings.Contains(gotBody, "data:image/png;base64,iVBORw0KGgo=") {
		t.Errorf("body should wrap pure base64 as data URL; got %q", gotBody)
	}
	if !strings.Contains(gotBody, `"uploadPath":"images/goloop"`) {
		t.Errorf("body missing uploadPath; got %q", gotBody)
	}
}

func TestUploader_DataURLPassthrough(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		writeUploadSuccess(t, w, "https://cdn.kie.ai/u/abc.png")
	}))
	defer srv.Close()

	u := NewUploader(srv.URL, 5*time.Second, 1)
	_, err := u.UploadBase64(context.Background(), "k", "data:image/jpeg;base64,AAAA", "image/png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should NOT be double-wrapped.
	if strings.Contains(gotBody, "data:image/png;base64,data:") {
		t.Errorf("data URL was double-wrapped; got %q", gotBody)
	}
	if !strings.Contains(gotBody, "data:image/jpeg;base64,AAAA") {
		t.Errorf("original data URL lost; got %q", gotBody)
	}
}

func TestUploader_RetryThenSuccess(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("upstream glitch"))
			return
		}
		writeUploadSuccess(t, w, "https://cdn.kie.ai/u/retry.png")
	}))
	defer srv.Close()

	u := NewUploader(srv.URL, 5*time.Second, 3)
	u.retryDelay = 1 * time.Millisecond
	url, err := u.UploadBase64(context.Background(), "k", "AAAA", "image/png")
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if url != "https://cdn.kie.ai/u/retry.png" {
		t.Errorf("wrong url: %q", url)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("expected 3 hits, got %d", got)
	}
}

func TestUploader_RetryExhausted(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	u := NewUploader(srv.URL, 5*time.Second, 3)
	u.retryDelay = 1 * time.Millisecond
	_, err := u.UploadBase64(context.Background(), "k", "AAAA", "image/png")
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if !strings.Contains(err.Error(), "exhausted 3 attempts") {
		t.Errorf("error should mention attempts exhausted; got %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("expected 3 hits, got %d", got)
	}
}

func TestUploader_NonRetryable4xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	u := NewUploader(srv.URL, 5*time.Second, 5)
	u.retryDelay = 1 * time.Millisecond
	_, err := u.UploadBase64(context.Background(), "k", "AAAA", "image/png")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("400 should not retry; got %d hits", got)
	}
}

func TestUploader_Retries429(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeUploadSuccess(t, w, "https://cdn.kie.ai/u/ok.png")
	}))
	defer srv.Close()

	u := NewUploader(srv.URL, 5*time.Second, 3)
	u.retryDelay = 1 * time.Millisecond
	_, err := u.UploadBase64(context.Background(), "k", "AAAA", "image/png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("expected 2 hits (429 retried once), got %d", got)
	}
}

func TestUploader_ContextCancel(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	u := NewUploader(srv.URL, 5*time.Second, 10)
	u.retryDelay = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after first attempt but before second retry wakes up.
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := u.UploadBase64(ctx, "k", "AAAA", "image/png")
	if err == nil {
		t.Fatal("expected error from cancellation")
	}
	// Should stop quickly, not burn through all 10 attempts.
	if got := atomic.LoadInt32(&hits); got > 3 {
		t.Errorf("expected early abort, got %d hits", got)
	}
}

func TestUploader_ApiSuccessFalse(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"code":    500,
			"msg":     "internal error from kie",
			"data":    map[string]any{"downloadUrl": ""},
		})
	}))
	defer srv.Close()

	u := NewUploader(srv.URL, 5*time.Second, 4)
	u.retryDelay = 1 * time.Millisecond
	_, err := u.UploadBase64(context.Background(), "k", "AAAA", "image/png")
	if err == nil {
		t.Fatal("expected error on success=false")
	}
	// A clear application-level failure (success=false, empty url) is not retried.
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("expected 1 hit for non-retryable failure, got %d", got)
	}
	if !strings.Contains(err.Error(), "internal error from kie") {
		t.Errorf("error should include upstream msg; got %v", err)
	}
}

func TestUploader_DefaultMimeType(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		writeUploadSuccess(t, w, "https://x/y")
	}))
	defer srv.Close()

	u := NewUploader(srv.URL, 5*time.Second, 1)
	_, err := u.UploadBase64(context.Background(), "k", "AAAA", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotBody, "data:image/png;base64,AAAA") {
		t.Errorf("empty mime type should default to image/png; got %q", gotBody)
	}
}

// sanity-check: ensure the retry math exits cleanly for retryAttempts=1.
func TestUploader_SingleAttempt(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	u := NewUploader(srv.URL, 5*time.Second, 1)
	_, err := u.UploadBase64(context.Background(), "k", "AAAA", "image/png")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("expected exactly 1 hit, got %d", got)
	}
}

