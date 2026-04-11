package kieai

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"

	"goloop/internal/model"
	"goloop/internal/storage"
)

type ResponseTransformer struct {
	store *storage.Store
}

func NewResponseTransformer() *ResponseTransformer {
	return &ResponseTransformer{}
}

func (t *ResponseTransformer) ToGoogleResponse(ctx context.Context, resultURLs []string) (*model.GoogleResponse, error) {
	if len(resultURLs) == 0 {
		return nil, fmt.Errorf("no result URLs")
	}

	type imgResult struct {
		idx int
		data []byte
		err  error
	}
	results := make([]imgResult, len(resultURLs))
	var wg sync.WaitGroup
	ch := make(chan imgResult, len(resultURLs))

	for i, url := range resultURLs {
		wg.Add(1)
		go func(idx int, u string) {
			defer wg.Done()
			if t.store != nil {
				data, err := t.store.DownloadToBytes(ctx, u)
				ch <- imgResult{idx: idx, data: data, err: err}
			} else {
				ch <- imgResult{idx: idx}
			}
		}(i, url)
	}

	go func() { wg.Wait(); close(ch) }()
	for r := range ch {
		results[r.idx] = r
	}

	parts := []model.Part{{Text: fmt.Sprintf("Generated %d image(s) successfully.", len(resultURLs))}}
	for _, r := range results {
		if r.err != nil {
			return nil, fmt.Errorf("download image %d: %w", r.idx, r.err)
		}
		parts = append(parts, model.Part{
			InlineData: &model.InlineData{MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(r.data)},
		})
	}

	return &model.GoogleResponse{
		Candidates: []model.Candidate{
			{Content: model.Content{Parts: parts}, FinishReason: "STOP"},
		},
	}, nil
}