package kieai

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"

	"goloop/internal/model"
	"goloop/internal/security"
)

type ModelDefaults struct {
	KieAIModel   string
	AspectRatio  string
	Resolution   string
	OutputFormat string
}

type RequestTransformer struct {
	modelMapping map[string]ModelDefaults
	uploader     *Uploader
}

func NewRequestTransformer(mapping map[string]ModelDefaults, uploader *Uploader) *RequestTransformer {
	return &RequestTransformer{
		modelMapping: mapping,
		uploader:     uploader,
	}
}

func (t *RequestTransformer) Transform(ctx context.Context, req *model.GoogleRequest, googleModel, apiKey string) (*model.KieAICreateTaskRequest, error) {
	defaults, ok := t.modelMapping[googleModel]
	if !ok {
		return nil, nil // passthrough to let caller handle
	}

	prompt, imageURLs, err := t.extractPartsContent(ctx, req, apiKey)
	if err != nil {
		return nil, err
	}

	input := model.KieAIInput{
		Prompt:       prompt,
		ImageInput:   imageURLs,
		AspectRatio:  defaults.AspectRatio,
		Resolution:   defaults.Resolution,
		OutputFormat: defaults.OutputFormat,
	}

	if req.GenerationConfig != nil && req.GenerationConfig.ImageConfig != nil {
		ic := req.GenerationConfig.ImageConfig
		if ic.AspectRatio != "" {
			input.AspectRatio = ic.AspectRatio
		}
		if ic.ImageSize != "" {
			input.Resolution = ic.ImageSize
		}
		// OutputFormat is a non-standard convenience field kept for backward compatibility.
		// The Google API canonical field is imageOutputOptions.mimeType; prefer it when present.
		if ic.ImageOutputOptions != nil && ic.ImageOutputOptions.MimeType != "" {
			input.OutputFormat = ic.ImageOutputOptions.MimeType
		} else if ic.OutputFormat != "" {
			input.OutputFormat = ic.OutputFormat
		}
	}

	return &model.KieAICreateTaskRequest{
		Model: defaults.KieAIModel,
		Input: input,
	}, nil
}

// extractPartsContent extracts text and image URLs from the request parts.
// InlineData images are concurrently uploaded to KIE's temporary storage via
// the uploader; FileData URLs are SSRF-validated and passed through. Both
// types share a single imageURLs slice and use placeholder slots so that
// the original interleaved order of Parts is preserved.
func (t *RequestTransformer) extractPartsContent(ctx context.Context, req *model.GoogleRequest, apiKey string) (string, []string, error) {
	var textParts []string
	var imageURLs []string

	type uploadTask struct {
		slot       int
		inlineData *model.InlineData
	}
	var tasks []uploadTask

	for _, content := range req.Contents {
		for _, part := range content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
			if part.InlineData != nil {
				tasks = append(tasks, uploadTask{slot: len(imageURLs), inlineData: part.InlineData})
				imageURLs = append(imageURLs, "") // placeholder, filled after upload
			}
			if part.FileData != nil && part.FileData.FileURI != "" {
				uri := part.FileData.FileURI
				if err := security.ValidateImageURL(uri); err != nil {
					return "", nil, fmt.Errorf("kieai: invalid fileUri: %w", err)
				}
				imageURLs = append(imageURLs, uri)
			}
		}
	}

	if len(tasks) > 0 {
		if t.uploader == nil {
			return "", nil, fmt.Errorf("kieai: uploader not configured but inlineData present")
		}
		g, gctx := errgroup.WithContext(ctx)
		for _, task := range tasks {
			task := task
			g.Go(func() error {
				url, err := t.uploader.UploadBase64(gctx, apiKey, task.inlineData.Data, task.inlineData.MimeType)
				if err != nil {
					return fmt.Errorf("kieai: upload inline image (slot %d): %w", task.slot, err)
				}
				imageURLs[task.slot] = url
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return "", nil, err
		}
	}

	return strings.Join(textParts, " "), imageURLs, nil
}
