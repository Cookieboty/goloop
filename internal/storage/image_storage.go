// internal/storage/image_storage.go
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store saves image bytes to local disk and returns the public HTTP URL.
type Store struct {
	localPath  string
	baseURL    string
	httpClient *http.Client
}

func NewStore(localPath, baseURL string) (*Store, error) {
	if err := os.MkdirAll(localPath, 0755); err != nil {
		return nil, fmt.Errorf("storage: mkdir %q: %w", localPath, err)
	}
	return &Store{
		localPath:  localPath,
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// SaveBytes saves raw image bytes to disk and returns the public URL.
func (s *Store) SaveBytes(data []byte, ext string) (string, error) {
	if len(ext) > 0 && ext[0] != '.' {
		ext = "." + ext
	}
	name := randomHex(16) + ext
	path := filepath.Join(s.localPath, name)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("storage: write file: %w", err)
	}

	return s.baseURL + "/" + name, nil
}

// DownloadToBytes fetches a URL and returns the raw bytes (max 30MB).
func (s *Store) DownloadToBytes(url string) ([]byte, error) {
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("storage: download %q: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("storage: download %q: HTTP %d", url, resp.StatusCode)
	}

	const maxSize = 30 * 1024 * 1024 // 30MB
	limited := io.LimitReader(resp.Body, maxSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("storage: read body: %w", err)
	}
	if len(data) > maxSize {
		return nil, fmt.Errorf("storage: image exceeds 30MB limit")
	}
	return data, nil
}

// LocalPath returns the filesystem directory path.
func (s *Store) LocalPath() string {
	return s.localPath
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
