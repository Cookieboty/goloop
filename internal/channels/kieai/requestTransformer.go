package kieai

import (
	"context"
	"strings"

	"goloop/internal/model"
)

type ModelDefaults struct {
	KieAIModel   string
	AspectRatio  string
	Resolution   string
	OutputFormat string
}

type RequestTransformer struct {
	modelMapping map[string]ModelDefaults
}

func NewRequestTransformer(mapping map[string]ModelDefaults) *RequestTransformer {
	return &RequestTransformer{modelMapping: mapping}
}

func (t *RequestTransformer) Transform(ctx context.Context, req *model.GoogleRequest, googleModel string) (*model.KieAICreateTaskRequest, error) {
	defaults, ok := t.modelMapping[googleModel]
	if !ok {
		return nil, nil // passthrough to let caller handle
	}

	var textParts []string
	for _, content := range req.Contents {
		for _, part := range content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
		}
	}

	input := model.KieAIInput{
		Prompt:       strings.Join(textParts, " "),
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

	return &model.KieAICreateTaskRequest{
		Model: defaults.KieAIModel,
		Input: input,
	}, nil
}