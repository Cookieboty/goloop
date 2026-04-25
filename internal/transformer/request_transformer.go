// internal/transformer/request_transformer.go
package transformer

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"goloop/internal/core"
	"goloop/internal/model"
	"goloop/internal/security"
	"goloop/internal/storage"
)

const (
	maxPromptLen  = 20000
	maxImageCount = 14
)

// RequestTransformer converts Google API requests to KIE.AI requests.
type RequestTransformer struct {
	store         *storage.Store
	configMgr     *core.ConfigManager
	maxImageBytes int64
}

func NewRequestTransformer(store *storage.Store, configMgr *core.ConfigManager, maxImageBytes int64) *RequestTransformer {
	if maxImageBytes <= 0 {
		maxImageBytes = 30 * 1024 * 1024 // 30MB default
	}
	return &RequestTransformer{
		store:         store,
		configMgr:     configMgr,
		maxImageBytes: maxImageBytes,
	}
}

// Transform converts a Google GenerateContent request into a KIE.AI CreateTask request.
// googleModel is the model name from the URL path (e.g. "gemini-3.1-flash-image-preview").
// channelName is used to look up channel-specific model mappings.
func (t *RequestTransformer) Transform(ctx context.Context, req *model.GoogleRequest, googleModel, channelName string) (*model.KieAICreateTaskRequest, error) {
	// Apply channel-specific model mapping if available
	mappedModel := t.configMgr.GetModelMapping(channelName, googleModel)
	
	// Use default KIE.AI model settings (can be overridden by channel mapping)
	kieAIModel := mappedModel
	if kieAIModel == googleModel {
		// No mapping found, use default for common models
		switch googleModel {
		case "gemini-3.1-flash-image-preview":
			kieAIModel = "nano-banana-2"
		case "gemini-3-pro-image-preview":
			kieAIModel = "nano-banana-pro"
		case "gemini-2.5-flash-image":
			kieAIModel = "google/nano-banana"
		default:
			kieAIModel = googleModel // Pass through as-is
		}
	}

	prompt, imageURLs, err := t.extractPartsContent(req)
	if err != nil {
		return nil, err
	}

	if len([]rune(prompt)) > maxPromptLen {
		return nil, fmt.Errorf("transformer: prompt exceeds %d characters", maxPromptLen)
	}

	if len(imageURLs) > maxImageCount {
		return nil, fmt.Errorf("transformer: too many images: %d > %d", len(imageURLs), maxImageCount)
	}

	// Default image generation settings
	input := model.KieAIInput{
		Prompt:       prompt,
		ImageInput:   imageURLs,
		AspectRatio:  "1:1",
		Resolution:   "1K",
		OutputFormat: "png",
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

	return &model.KieAICreateTaskRequest{
		Model: kieAIModel,
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
	// Pre-check: base64 encodes ~4/3 of raw size. Reject early to avoid allocating a huge buffer.
	maxBase64Len := int(t.maxImageBytes*4/3) + 1024 // base64 overhead + margin
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

	if int64(len(raw)) > t.maxImageBytes {
		return "", fmt.Errorf("image exceeds %dMB limit (%d bytes)", t.maxImageBytes/(1024*1024), len(raw))
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
