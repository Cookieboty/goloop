// internal/storage/image_storage_test.go
package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveBytes(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, "http://localhost:8080/images")
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("fake png data")
	url, err := store.SaveBytes(data, "png")
	if err != nil {
		t.Fatalf("SaveBytes error: %v", err)
	}

	if !strings.HasPrefix(url, "http://localhost:8080/images/") {
		t.Errorf("unexpected URL: %q", url)
	}
	if !strings.HasSuffix(url, ".png") {
		t.Errorf("expected .png extension in URL: %q", url)
	}

	// Verify file exists on disk
	filename := filepath.Base(url)
	saved, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		t.Fatalf("file not found on disk: %v", err)
	}
	if string(saved) != string(data) {
		t.Errorf("content mismatch")
	}
}

func TestDownloadToBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("png-image-data"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	store, _ := NewStore(dir, "http://localhost:8080/images")

	data, err := store.DownloadToBytes(context.Background(), srv.URL+"/img.png")
	if err != nil {
		t.Fatalf("DownloadToBytes error: %v", err)
	}
	if string(data) != "png-image-data" {
		t.Errorf("data mismatch: %q", data)
	}
}

func TestDownloadToBytes_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	store, _ := NewStore(dir, "http://localhost:8080/images")

	_, err := store.DownloadToBytes(context.Background(), srv.URL+"/missing.png")
	if err == nil {
		t.Error("expected error for HTTP 404, got nil")
	}
}

func TestNewStore_MkdirAll(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")
	_, err := NewStore(nested, "http://localhost/images")
	if err != nil {
		t.Fatalf("NewStore should create nested dirs: %v", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("directory not created: %v", err)
	}
}
