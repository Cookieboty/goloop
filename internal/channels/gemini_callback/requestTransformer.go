package gemini_callback

import (
	"context"
	"fmt"
	"log/slog"
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
			// Map pixel dimensions to KIE.AI resolution options (1K, 2K, 4K)
			// If not a recognized format, keep the default from modelMapping
			if mapped := mapResolution(ic.ImageSize); mapped != "" {
				input.Resolution = mapped
			}
		}
		// OutputFormat is a non-standard convenience field kept for backward compatibility.
		// The Google API canonical field is imageOutputOptions.mimeType; prefer it when present.
		if ic.ImageOutputOptions != nil && ic.ImageOutputOptions.MimeType != "" {
			input.OutputFormat = ic.ImageOutputOptions.MimeType
		} else if ic.OutputFormat != "" {
			input.OutputFormat = ic.OutputFormat
		}
	}

	kieReq := &model.KieAICreateTaskRequest{
		Model: defaults.KieAIModel,
		Input: input,
	}
	
	slog.Info("requestTransformer: created KIE.AI request", 
		"model", kieReq.Model,
		"promptLength", len(input.Prompt),
		"imageInputCount", len(input.ImageInput),
		"aspectRatio", input.AspectRatio,
		"resolution", input.Resolution)
	
	return kieReq, nil
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

	for contentIdx, content := range req.Contents {
		for partIdx, part := range content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
				slog.Info("extractPartsContent: found text part", 
					"contentIdx", contentIdx,
					"partIdx", partIdx,
					"textLength", len(part.Text))
			}
			
			// Support both REST API (inline_data) and Vertex AI (inlineData) formats
			inlineData := part.InlineData
			if inlineData == nil {
				inlineData = part.InlineDataAlt
			}
			if inlineData != nil {
				// Support both mime_type and mimeType
				mimeType := inlineData.MimeType
				if mimeType == "" {
					mimeType = inlineData.MimeTypeAlt
				}
				tasks = append(tasks, uploadTask{slot: len(imageURLs), inlineData: inlineData})
				imageURLs = append(imageURLs, "") // placeholder, filled after upload
				slog.Info("extractPartsContent: found inlineData part", 
					"contentIdx", contentIdx,
					"partIdx", partIdx,
					"mimeType", mimeType,
					"dataLength", len(inlineData.Data))
			}
			
			// Support both REST API (file_data) and Vertex AI (fileData) formats
			fileData := part.FileData
			if fileData == nil {
				fileData = part.FileDataAlt
			}
			if fileData != nil {
				// Support both file_uri and fileUri
				uri := fileData.FileURI
				if uri == "" {
					uri = fileData.FileURIAlt
				}
				if uri != "" {
					if err := security.ValidateImageURL(uri); err != nil {
						return "", nil, fmt.Errorf("kieai: invalid fileUri: %w", err)
					}
					imageURLs = append(imageURLs, uri)
					slog.Info("extractPartsContent: found fileData part", 
						"contentIdx", contentIdx,
						"partIdx", partIdx,
						"fileUri", uri)
				}
			}
		}
	}

	if len(tasks) > 0 {
		if t.uploader == nil {
			return "", nil, fmt.Errorf("kieai: uploader not configured but inlineData present")
		}
		slog.Info("extractPartsContent: starting concurrent image uploads", "taskCount", len(tasks))
		g, gctx := errgroup.WithContext(ctx)
		for _, task := range tasks {
			task := task
			g.Go(func() error {
				// Support both mime_type and mimeType
				mimeType := task.inlineData.MimeType
				if mimeType == "" {
					mimeType = task.inlineData.MimeTypeAlt
				}
				slog.Info("extractPartsContent: uploading image", "slot", task.slot, "mimeType", mimeType)
				url, err := t.uploader.UploadBase64(gctx, apiKey, task.inlineData.Data, mimeType)
				if err != nil {
					slog.Error("extractPartsContent: upload failed", "slot", task.slot, "err", err)
					return fmt.Errorf("kieai: upload inline image (slot %d): %w", task.slot, err)
				}
				imageURLs[task.slot] = url
				slog.Info("extractPartsContent: upload completed", "slot", task.slot, "url", url)
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return "", nil, err
		}
		slog.Info("extractPartsContent: all uploads completed successfully")
	} else {
		slog.Info("extractPartsContent: no inlineData images to upload")
	}

	return strings.Join(textParts, " "), imageURLs, nil
}

// mapResolution maps pixel dimensions or Google API size names to KIE.AI resolution options.
// Returns empty string if the input is not recognized, indicating the caller should use defaults.
func mapResolution(size string) string {
	// Already in KIE.AI format
	switch size {
	case "1K", "2K", "4K":
		return size
	}
	
	// Map common pixel dimensions to KIE.AI options
	// Based on typical aspect ratios and resolutions
	switch size {
	// 1K resolutions (~1024-1280 range)
	case "1024x1024", "1280x720", "720x1280":
		return "1K"
	
	// 2K resolutions (~1920-2560 range)
	case "1920x1080", "1080x1920", "2048x2048", "2560x1440", "1440x2560":
		return "2K"
	
	// 4K resolutions (~3840-4096 range)
	case "3840x2160", "2160x3840", "4096x4096":
		return "4K"
	}
	
	// Not recognized, return empty to use default
	return ""
}
