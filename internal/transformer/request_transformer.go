// internal/transformer/request_transformer.go
package transformer

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"goloop/internal/config"
	"goloop/internal/model"
	"goloop/internal/security"
	"goloop/internal/storage"
)

const (
	maxPromptLen      = 20000
	maxPromptLenEdit  = 5000
	maxImageCount     = 14
	maxImageCountEdit = 10
	maxImageBytes     = 30 * 1024 * 1024 // 30MB
	maxImageBytesEdit = 10 * 1024 * 1024 // 10MB for edit model
)

// RequestTransformer converts Google API requests to KIE.AI requests.
type RequestTransformer struct {
	store        *storage.Store
	modelMapping map[string]config.ModelDefaults
}

func NewRequestTransformer(store *storage.Store, modelMapping map[string]config.ModelDefaults) *RequestTransformer {
	return &RequestTransformer{store: store, modelMapping: modelMapping}
}

// Transform converts a Google GenerateContent request into a KIE.AI CreateTask request.
// googleModel is the model name from the URL path (e.g. "gemini-3.1-flash-image-preview").
func (t *RequestTransformer) Transform(ctx context.Context, req *model.GoogleRequest, googleModel string) (*model.KieAICreateTaskRequest, error) {
	defaults, ok := t.modelMapping[googleModel]
	if !ok {
		return nil, fmt.Errorf("transformer: unknown model %q", googleModel)
	}

	prompt, imageURLs, err := t.extractPartsContent(req)
	if err != nil {
		return nil, err
	}

	// Check if this is an edit model
	isEditModel := strings.Contains(defaults.KieAIModel, "edit")

	// Validate prompt length based on model type
	maxPrompt := maxPromptLen
	if isEditModel {
		maxPrompt = maxPromptLenEdit
	}
	if len([]rune(prompt)) > maxPrompt {
		return nil, fmt.Errorf("transformer: prompt exceeds %d characters", maxPrompt)
	}

	// Validate image count based on model type
	maxImages := maxImageCount
	if isEditModel {
		maxImages = maxImageCountEdit
		// Edit models require at least one image
		if len(imageURLs) == 0 {
			return nil, fmt.Errorf("transformer: edit model requires at least one image")
		}
	}
	if len(imageURLs) > maxImages {
		return nil, fmt.Errorf("transformer: too many images: %d > %d", len(imageURLs), maxImages)
	}

	// Build KIE.AI input based on model type
	var input model.KieAIInput
	if isEditModel {
		// Edit model: use image_urls and image_size (aspectRatio only, no resolution)
		aspectRatio := defaults.AspectRatio
		if req.GenerationConfig != nil && req.GenerationConfig.ImageConfig != nil {
			if req.GenerationConfig.ImageConfig.AspectRatio != "" {
				aspectRatio = req.GenerationConfig.ImageConfig.AspectRatio
			}
		}

		input = model.KieAIInput{
			Prompt:       prompt,
			ImageURLs:    imageURLs,
			ImageSize:    aspectRatio, // Edit model uses aspectRatio as image_size
			OutputFormat: defaults.OutputFormat,
		}

		if req.GenerationConfig != nil && req.GenerationConfig.ImageConfig != nil {
			if req.GenerationConfig.ImageConfig.OutputFormat != "" {
				input.OutputFormat = req.GenerationConfig.ImageConfig.OutputFormat
			}
		}
	} else {
		// Text-to-image model: use image_input, aspect_ratio, and resolution
		input = model.KieAIInput{
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
			if ic.OutputFormat != "" {
				input.OutputFormat = ic.OutputFormat
			}
		}
	}

	return &model.KieAICreateTaskRequest{
		Model: defaults.KieAIModel,
		Input: input,
	}, nil
}

func (t *RequestTransformer) extractPartsContent(req *model.GoogleRequest) (string, []string, error) {
	var textParts []string
	var imageURLs []string

	for _, content := range req.Contents {
		for _, part := range content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}

			if part.InlineData != nil {
				url, err := t.saveInlineData(part.InlineData)
				if err != nil {
					return "", nil, fmt.Errorf("transformer: save inline image: %w", err)
				}
				imageURLs = append(imageURLs, url)
			}

			if part.FileData != nil && part.FileData.FileURI != "" {
				uri := part.FileData.FileURI

				// SSRF 防护：验证 URL 安全性
				if err := security.ValidateImageURL(uri); err != nil {
					return "", nil, fmt.Errorf("transformer: invalid fileUri: %w", err)
				}

				imageURLs = append(imageURLs, uri)
			}
		}
	}

	return strings.Join(textParts, " "), imageURLs, nil
}

func (t *RequestTransformer) saveInlineData(data *model.InlineData) (string, error) {
	// Pre-check: base64 encodes ~4/3 of raw size. 30MB decoded → ~40.96MB base64.
	// Reject early to avoid allocating a huge buffer.
	const maxBase64Len = 40*1024*1024 + 1024 // 40MB + margin
	if len(data.Data) > maxBase64Len {
		return "", fmt.Errorf("base64 payload too large before decode (%d bytes)", len(data.Data))
	}

	raw, err := base64.StdEncoding.DecodeString(data.Data)
	if err != nil {
		// Try URL-safe base64 as fallback
		raw, err = base64.URLEncoding.DecodeString(data.Data)
		if err != nil {
			return "", fmt.Errorf("base64 decode: %w", err)
		}
	}

	// Note: Edit models have 10MB limit, text-to-image models have 30MB limit
	// We use the larger limit here and let KIE.AI validate per-model
	if len(raw) > maxImageBytes {
		return "", fmt.Errorf("image exceeds 30MB limit (%d bytes)", len(raw))
	}

	ext := mimeToExt(data.MimeType)
	return t.store.SaveBytes(raw, ext)
}

func mimeToExt(mimeType string) string {
	switch strings.ToLower(mimeType) {
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	default:
		return "png"
	}
}
