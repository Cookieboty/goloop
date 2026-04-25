// internal/transformer/response_transformer.go
package transformer

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
	"goloop/internal/model"
	"goloop/internal/storage"
)

// ResponseTransformer converts KIE.AI results to Google API responses.
type ResponseTransformer struct {
	store           *storage.Store
	downloadSem     *semaphore.Weighted
	maxConcDownload int64
}

func NewResponseTransformer(store *storage.Store) *ResponseTransformer {
	const maxConcurrent = 8 // Limit concurrent downloads to 8
	return &ResponseTransformer{
		store:           store,
		downloadSem:     semaphore.NewWeighted(maxConcurrent),
		maxConcDownload: maxConcurrent,
	}
}

// ToGoogleResponse converts successful KIE.AI result URLs to a Google API response.
// All images are downloaded concurrently and embedded as inlineData.
// imageOnly should be true when the request's responseModalities contains only
// "image" (no "text"), in which case the descriptive text part is omitted.
func (t *ResponseTransformer) ToGoogleResponse(ctx context.Context, resultURLs []string, imageOnly bool) (*model.GoogleResponse, error) {
	if len(resultURLs) == 0 {
		return nil, fmt.Errorf("response_transformer: no result URLs")
	}

	type result struct {
		idx  int
		data []byte
		err  error
	}

	results := make([]result, len(resultURLs))
	var wg sync.WaitGroup
	ch := make(chan result, len(resultURLs))

	for i, url := range resultURLs {
		wg.Add(1)
		go func(idx int, u string) {
			defer wg.Done()
			slog.Debug("response_transformer: downloading image", "idx", idx, "url", u)
			start := time.Now()
			data, err := t.store.DownloadToBytes(ctx, u)
			elapsed := time.Since(start)
			if err != nil {
				slog.Error("response_transformer: download failed", "idx", idx, "url", u, "elapsed", elapsed, "err", err)
			} else {
				slog.Debug("response_transformer: download succeeded", "idx", idx, "size", len(data), "elapsed", elapsed)
			}
			ch <- result{idx: idx, data: data, err: err}
		}(i, url)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for r := range ch {
		results[r.idx] = r
	}

	var parts []model.Part
	if !imageOnly {
		parts = append(parts, model.Part{
			Text: fmt.Sprintf("Generated %d image(s) successfully.", len(resultURLs)),
		})
	}

	for _, r := range results {
		if r.err != nil {
			return nil, fmt.Errorf("response_transformer: download image %d: %w", r.idx, r.err)
		}
		encoded := base64.StdEncoding.EncodeToString(r.data)
		parts = append(parts, model.Part{
			InlineData: &model.InlineData{
				MimeType: "image/png",
				Data:     encoded,
			},
		})
	}

	return &model.GoogleResponse{
		Candidates: []model.Candidate{
			{
				Content:      model.Content{Parts: parts},
				FinishReason: "STOP",
			},
		},
	}, nil
}

// ToGoogleStreamingResponse 转换为 SSE streaming 响应格式
// imageOnly 含义同 ToGoogleResponse。
func (t *ResponseTransformer) ToGoogleStreamingResponse(ctx context.Context, resultURLs []string, responseID string, imageOnly bool) (*model.StreamingResponse, error) {
	if len(resultURLs) == 0 {
		return nil, fmt.Errorf("response_transformer: no result URLs")
	}

	type result struct {
		idx  int
		data []byte
		err  error
	}

	results := make([]result, len(resultURLs))
	var wg sync.WaitGroup
	ch := make(chan result, len(resultURLs))

	for i, url := range resultURLs {
		wg.Add(1)
		go func(idx int, u string) {
			defer wg.Done()
			// Acquire semaphore to limit concurrent downloads
			if err := t.downloadSem.Acquire(ctx, 1); err != nil {
				ch <- result{idx: idx, err: err}
				return
			}
			defer t.downloadSem.Release(1)
			
			slog.Debug("response_transformer: downloading image (streaming)", "idx", idx, "url", u)
			start := time.Now()
			data, err := t.store.DownloadToBytes(ctx, u)
			elapsed := time.Since(start)
			if err != nil {
				slog.Error("response_transformer: download failed (streaming)", "idx", idx, "url", u, "elapsed", elapsed, "err", err)
			} else {
				slog.Debug("response_transformer: download succeeded (streaming)", "idx", idx, "size", len(data), "elapsed", elapsed)
			}
			ch <- result{idx: idx, data: data, err: err}
		}(i, url)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for r := range ch {
		results[r.idx] = r
	}

	parts := []model.Part{}
	for _, r := range results {
		if r.err != nil {
			return nil, fmt.Errorf("response_transformer: download image %d: %w", r.idx, r.err)
		}
		encoded := base64.StdEncoding.EncodeToString(r.data)
		parts = append(parts, model.Part{
			InlineData: &model.InlineData{
				MimeType: "image/png",
				Data:     encoded,
			},
		})
	}

	return &model.StreamingResponse{
		Candidates: []model.Candidate{
			{
				Content:      model.Content{Parts: parts},
				FinishReason: "STOP",
			},
		},
		ModelVersion: "gemini-2.5-flash",
		ResponseId:   responseID,
		UsageMetadata: &model.UsageMetadata{
			TotalTokenCount: len(parts),
		},
	}, nil
}

// ToGoogleError converts a KIE.AI error code to a Google-format error response.
func ToGoogleError(kieaiCode int, message string) (model.GoogleError, int) {
	var status string
	var httpCode int

	switch kieaiCode {
	case 401:
		status = "UNAUTHENTICATED"
		httpCode = 401
	case 402, 429:
		status = "RESOURCE_EXHAUSTED"
		httpCode = 429
	case 422:
		status = "INVALID_ARGUMENT"
		httpCode = 400
	default:
		status = "INTERNAL"
		httpCode = 500
	}

	return model.GoogleError{
		Error: model.GoogleErrorDetail{
			Code:    httpCode,
			Message: message,
			Status:  status,
		},
	}, httpCode
}
