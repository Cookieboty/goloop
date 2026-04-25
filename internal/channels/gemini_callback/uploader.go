package gemini_callback

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"goloop/internal/model"
)

// Uploader handles uploading base64-encoded images to KIE's temporary storage.
type Uploader struct {
	httpClient    *http.Client
	retryAttempts int
	retryDelay    time.Duration
}

// KIE.AI file upload API base URL (different from task API)
const uploadBaseURL = "https://kieai.redpandaai.co"

// NewUploader creates an uploader for KIE's file upload API.
// Note: File upload API uses a different base URL (kieai.redpandaai.co) 
// from the task API (api.kie.ai), so we ignore the baseURL parameter.
// retryAttempts is the total number of attempts (min 1); transient failures
// are retried with exponential backoff starting at retryDelay.
func NewUploader(baseURL string, timeout time.Duration, retryAttempts int) *Uploader {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	if retryAttempts <= 0 {
		retryAttempts = 1
	}
	return &Uploader{
		httpClient:    &http.Client{Timeout: timeout},
		retryAttempts: retryAttempts,
		retryDelay:    500 * time.Millisecond,
	}
}

// UploadBase64 uploads a base64-encoded image to KIE's temporary storage and
// returns the downloadUrl from KIE's response. The base64Data may be either:
//   - A pure base64 string: "iVBORw0KGgoAAAANSUhEUgAA..."
//   - A data URL: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA..."
//
// KIE's API accepts both formats. If mimeType is empty, defaults to "image/png".
// On transient failures (network errors, 5xx, 429) the call is retried with
// exponential backoff up to retryAttempts. Context cancellation aborts
// immediately without further retries.
func (u *Uploader) UploadBase64(ctx context.Context, apiKey, base64Data, mimeType string) (string, error) {
	if mimeType == "" {
		mimeType = "image/png"
	}
	if !strings.HasPrefix(base64Data, "data:") {
		base64Data = fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)
	}

	var lastErr error
	delay := u.retryDelay
	for attempt := 0; attempt < u.retryAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}

		url, err := u.uploadOnce(ctx, apiKey, base64Data)
		if err == nil {
			return url, nil
		}
		lastErr = err

		// Abort on context cancellation or non-retryable client errors.
		if ctx.Err() != nil {
			return "", err
		}
		if !isRetryable(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("uploader: exhausted %d attempts: %w", u.retryAttempts, lastErr)
}

func (u *Uploader) uploadOnce(ctx context.Context, apiKey, base64Data string) (string, error) {
	uploadReq := model.KieAIUploadRequest{
		Base64Data: base64Data,
		UploadPath: "images/goloop",
	}

	body, err := json.Marshal(uploadReq)
	if err != nil {
		return "", nonRetryable(fmt.Errorf("uploader: marshal request: %w", err))
	}

	uploadURL := uploadBaseURL + "/api/file-base64-upload"
	
	// 记录上传请求信息（隐藏敏感数据）
	slog.Info("uploader: starting file upload", 
		"url", uploadURL,
		"uploadPath", "images/goloop",
		"dataLength", len(base64Data))
	
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		uploadURL, bytes.NewReader(body))
	if err != nil {
		return "", nonRetryable(err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := u.httpClient.Do(httpReq)
	if err != nil {
		slog.Warn("file upload request failed",
			"url", uploadURL,
			"err", err)
		return "", fmt.Errorf("uploader: HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("uploader: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		slog.Warn("file upload returned non-200 status",
			"url", uploadURL,
			"status", resp.StatusCode,
			"response", string(data))
		httpErr := fmt.Errorf("uploader: HTTP %d: %s", resp.StatusCode, string(data))
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return "", nonRetryable(httpErr)
		}
		return "", httpErr
	}

	var uploadResp model.KieAIUploadResponse
	if err := json.Unmarshal(data, &uploadResp); err != nil {
		return "", nonRetryable(fmt.Errorf("uploader: unmarshal response: %w", err))
	}
	if !uploadResp.Success || uploadResp.Data.DownloadUrl == "" {
		return "", nonRetryable(fmt.Errorf("uploader: upload failed: %s", uploadResp.Msg))
	}

	slog.Info("uploader: file uploaded successfully", 
		"downloadUrl", uploadResp.Data.DownloadUrl,
		"fileName", uploadResp.Data.FileName)

	return uploadResp.Data.DownloadUrl, nil
}

// nonRetryableError wraps an error to signal the retry loop should stop.
type nonRetryableError struct{ err error }

func (e *nonRetryableError) Error() string { return e.err.Error() }
func (e *nonRetryableError) Unwrap() error { return e.err }

func nonRetryable(err error) error { return &nonRetryableError{err: err} }

func isRetryable(err error) bool {
	var nr *nonRetryableError
	return !errors.As(err, &nr)
}
