// internal/handler/image_handler.go
package handler

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var imageFilenamePattern = regexp.MustCompile(`^[a-f0-9]{32}\.(png|jpg|jpeg|webp|gif)$`)

type ImageHandler struct {
	localPath string
}

func NewImageHandler(localPath string) *ImageHandler {
	return &ImageHandler{localPath: localPath}
}

func (h *ImageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 仅允许 GET 请求
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 提取文件名（去除路径前缀 /images/）
	filename := strings.TrimPrefix(r.URL.Path, "/images/")

	// 严格验证文件名格式（32位十六进制 + 扩展名）
	if !imageFilenamePattern.MatchString(filename) {
		slog.Warn("invalid image filename", "filename", filename, "ip", extractIPForLogging(r))
		http.NotFound(w, r)
		return
	}

	// 使用 filepath.Join 并验证最终路径
	fullPath := filepath.Join(h.localPath, filename)

	// 二次验证：确保解析后的路径在允许的目录内
	// filepath.Join 会自动清理 .. 等，但我们仍需要验证
	cleanLocal := filepath.Clean(h.localPath)
	cleanFull := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanFull, cleanLocal) {
		slog.Warn("path traversal attempt blocked", "filename", filename, "ip", extractIPForLogging(r))
		http.NotFound(w, r)
		return
	}

	// 检查文件是否存在且是普通文件
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	// 设置缓存头（图片不经常变化）
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Type", getMimeType(filename))

	http.ServeFile(w, r, fullPath)
}

func getMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}

func extractIPForLogging(r *http.Request) string {
	// 用于日志记录的 IP 提取
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}
