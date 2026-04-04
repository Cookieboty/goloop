// internal/storage/image_storage.go
package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"goloop/internal/security"
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
	name, err := randomHex(16)
	if err != nil {
		return "", err
	}
	name += ext
	path := filepath.Join(s.localPath, name)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("storage: write file: %w", err)
	}

	return s.baseURL + "/" + name, nil
}

// DownloadToBytes fetches a URL and returns the raw bytes (max 30MB).
// The request is cancelled when ctx is done.
func (s *Store) DownloadToBytes(ctx context.Context, url string) ([]byte, error) {
	// SSRF 防护：验证 URL 安全性
	if err := security.ValidateImageURL(url); err != nil {
		return nil, fmt.Errorf("storage: blocked URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("storage: build request %q: %w", url, err)
	}
	resp, err := s.httpClient.Do(req)
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

// SetHTTPClient allows setting a custom HTTP client (for testing)
func (s *Store) SetHTTPClient(client *http.Client) {
	s.httpClient = client
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("storage: rand.Read: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// CleanupOldImages 删除超过 maxAge 的图片
func (s *Store) CleanupOldImages(maxAge time.Duration) error {
	now := time.Now()
	var deleted, failed int

	entries, err := os.ReadDir(s.localPath)
	if err != nil {
		return fmt.Errorf("storage: read dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// 检查文件年龄
		if now.Sub(info.ModTime()) > maxAge {
			path := filepath.Join(s.localPath, entry.Name())
			if err := os.Remove(path); err != nil {
				slog.Warn("storage: failed to delete old image", "file", entry.Name(), "err", err)
				failed++
			} else {
				deleted++
			}
		}
	}

	if deleted > 0 || failed > 0 {
		slog.Info("storage: cleanup completed", "deleted", deleted, "failed", failed)
	}
	return nil
}

// CheckDiskSpace 检查磁盘空间（返回可用字节数）
func (s *Store) CheckDiskSpace() (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.localPath, &stat); err != nil {
		return 0, fmt.Errorf("storage: statfs: %w", err)
	}

	avail := stat.Bavail * uint64(stat.Bsize)
	return avail, nil
}

// StartCleanupWorker 启动清理 goroutine
func (s *Store) StartCleanupWorker(ctx context.Context, interval, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("storage: cleanup worker started", "interval", interval, "maxAge", maxAge)

	for {
		select {
		case <-ctx.Done():
			slog.Info("storage: cleanup worker stopped")
			return
		case <-ticker.C:
			// 检查磁盘空间
			avail, err := s.CheckDiskSpace()
			if err != nil {
				slog.Error("storage: disk space check failed", "err", err)
				continue
			}

			const minSpace = 1 * 1024 * 1024 * 1024 // 1GB
			if avail < minSpace {
				slog.Warn("storage: low disk space", "available_gb", avail/(1024*1024*1024))
			}

			// 清理旧文件
			if err := s.CleanupOldImages(maxAge); err != nil {
				slog.Error("storage: cleanup failed", "err", err)
			}
		}
	}
}
