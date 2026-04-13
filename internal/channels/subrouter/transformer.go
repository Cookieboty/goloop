package subrouter

import (
	"fmt"

	"goloop/internal/model"
)

// --- OpenAI Chat API types ---

// ChatRequest is the OpenAI /v1/chat/completions request body.
type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
}

// ChatMessage represents a single message in the conversation.
// Content may be a plain string (text-only) or a []ContentPart (vision).
type ChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string | []ContentPart
}

// ContentPart is an element of a multi-modal message.
type ContentPart struct {
	Type     string    `json:"type"`               // "text" or "image_url"
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL wraps an image URL or base64 data URL.
type ImageURL struct {
	URL string `json:"url"` // "data:image/png;base64,..." or https URL
}

// ChatResponse is the OpenAI /v1/chat/completions non-streaming response.
type ChatResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice is one completion candidate in a ChatResponse.
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// Usage contains token consumption statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// --- Conversion functions ---

// googleToOpenAI converts a Google GenerateContent request to an OpenAI Chat
// completions request. The modelName is passed through as-is.
func googleToOpenAI(req *model.GoogleRequest, modelName string) *ChatRequest {
	var messages []ChatMessage

	for _, content := range req.Contents {
		role := mapRoleToOpenAI(content.Role)
		var parts []ContentPart

		for _, part := range content.Parts {
			if part.Text != "" {
				parts = append(parts, ContentPart{Type: "text", Text: part.Text})
			}
			if part.InlineData != nil {
				dataURL := fmt.Sprintf("data:%s;base64,%s",
					part.InlineData.MimeType, part.InlineData.Data)
				parts = append(parts, ContentPart{
					Type:     "image_url",
					ImageURL: &ImageURL{URL: dataURL},
				})
			}
			if part.FileData != nil && part.FileData.FileURI != "" {
				parts = append(parts, ContentPart{
					Type:     "image_url",
					ImageURL: &ImageURL{URL: part.FileData.FileURI},
				})
			}
		}

		// Use a plain string when the message is text-only (most common case).
		// Use []ContentPart for multi-modal messages.
		if len(parts) == 1 && parts[0].Type == "text" {
			messages = append(messages, ChatMessage{Role: role, Content: parts[0].Text})
		} else if len(parts) > 0 {
			messages = append(messages, ChatMessage{Role: role, Content: parts})
		}
	}

	return &ChatRequest{Model: modelName, Messages: messages}
}

// openAIToGoogle converts an OpenAI Chat completions response to a Google
// GenerateContent response.
func openAIToGoogle(resp *ChatResponse) *model.GoogleResponse {
	if len(resp.Choices) == 0 {
		return &model.GoogleResponse{}
	}
	choice := resp.Choices[0]

	var textContent string
	switch v := choice.Message.Content.(type) {
	case string:
		textContent = v
	}

	return &model.GoogleResponse{
		Candidates: []model.Candidate{{
			Content: model.Content{
				Parts: []model.Part{{Text: textContent}},
				Role:  "model",
			},
			FinishReason: mapFinishReasonToGoogle(choice.FinishReason),
		}},
	}
}

// mapRoleToOpenAI converts a Google role string to an OpenAI role string.
func mapRoleToOpenAI(googleRole string) string {
	switch googleRole {
	case "model":
		return "assistant"
	case "system":
		return "system"
	default:
		return "user"
	}
}

// mapFinishReasonToGoogle converts an OpenAI finish_reason to a Google
// FinishReason string.
func mapFinishReasonToGoogle(openaiReason string) string {
	switch openaiReason {
	case "stop":
		return "STOP"
	case "length":
		return "MAX_TOKENS"
	case "content_filter":
		return "SAFETY"
	case "tool_calls":
		return "STOP"
	default:
		return "FINISH_REASON_UNSPECIFIED"
	}
}
