// internal/model/openai.go
package model

// --- OpenAI Error Response ---

// OpenAIError represents the error response format for OpenAI API.
// Reference: https://platform.openai.com/docs/guides/error-codes
type OpenAIError struct {
	Error OpenAIErrorDetail `json:"error"`
}

// OpenAIErrorDetail contains the details of an OpenAI API error.
type OpenAIErrorDetail struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`              // e.g., "invalid_request_error", "api_error", "authentication_error"
	Param   *string `json:"param,omitempty"`   // The parameter that caused the error (if applicable)
	Code    *string `json:"code,omitempty"`    // Error code (if applicable)
}

// NewOpenAIError creates a standard OpenAI error response.
func NewOpenAIError(message, errType string) OpenAIError {
	return OpenAIError{
		Error: OpenAIErrorDetail{
			Message: message,
			Type:    errType,
		},
	}
}

// --- OpenAI Chat Completions API ---

// ChatCompletionsRequest represents a request to /v1/chat/completions.
// Note: This structure is primarily for documentation. The gpt-image channel
// passes the raw request body verbatim without parsing.
type ChatCompletionsRequest struct {
	Model            string         `json:"model"`
	Messages         []ChatMessage  `json:"messages"`
	Stream           bool           `json:"stream,omitempty"`
	Temperature      *float64       `json:"temperature,omitempty"`
	TopP             *float64       `json:"top_p,omitempty"`
	N                *int           `json:"n,omitempty"`
	MaxTokens        *int           `json:"max_tokens,omitempty"`
	Stop             interface{}    `json:"stop,omitempty"` // string or []string
	PresencePenalty  *float64       `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64       `json:"frequency_penalty,omitempty"`
	User             string         `json:"user,omitempty"`
}

// ChatMessage represents a single message in the conversation.
type ChatMessage struct {
	Role    string      `json:"role"`    // "system", "user", "assistant"
	Content interface{} `json:"content"` // string or []ContentPart for multi-modal
}

// ContentPart is an element of a multi-modal message.
type ContentPart struct {
	Type     string    `json:"type"`               // "text" or "image_url"
	Text     string    `json:"text,omitempty"`     // For type="text"
	ImageURL *ImageURL `json:"image_url,omitempty"` // For type="image_url"
}

// ImageURL wraps an image URL or base64 data URL.
type ImageURL struct {
	URL    string `json:"url"`               // "data:image/png;base64,..." or https URL
	Detail string `json:"detail,omitempty"`  // "auto", "low", "high"
}

// ChatCompletionsResponse represents a non-streaming response from /v1/chat/completions.
type ChatCompletionsResponse struct {
	ID      string                `json:"id"`
	Object  string                `json:"object"` // "chat.completion"
	Created int64                 `json:"created"`
	Model   string                `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   *Usage                `json:"usage,omitempty"`
}

// ChatCompletionChoice is one completion candidate.
type ChatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"` // "stop", "length", "content_filter", etc.
}

// Usage contains token consumption statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// --- OpenAI Images API ---

// ImageGenerationRequest represents a request to /v1/images/generations.
type ImageGenerationRequest struct {
	Prompt         string `json:"prompt"`
	Model          string `json:"model,omitempty"`           // e.g., "dall-e-3", "gpt-image-2-all"
	N              *int   `json:"n,omitempty"`               // Number of images (default 1)
	Quality        string `json:"quality,omitempty"`         // "standard" or "hd"
	ResponseFormat string `json:"response_format,omitempty"` // "url" or "b64_json"
	Size           string `json:"size,omitempty"`            // e.g., "1024x1024"
	Style          string `json:"style,omitempty"`           // "vivid" or "natural"
	User           string `json:"user,omitempty"`
}

// ImageGenerationResponse represents a response from /v1/images/generations.
type ImageGenerationResponse struct {
	Created int64       `json:"created"`
	Data    []ImageData `json:"data"`
}

// ImageData contains a single generated image.
type ImageData struct {
	URL           string `json:"url,omitempty"`            // Image URL (if response_format="url")
	B64JSON       string `json:"b64_json,omitempty"`       // Base64-encoded image (if response_format="b64_json")
	RevisedPrompt string `json:"revised_prompt,omitempty"` // The prompt that was actually used (for safety)
}

// ImageEditRequest represents a request to /v1/images/edits.
// Note: This endpoint uses multipart/form-data. This structure is for documentation only.
// The actual request is passed as raw bytes with the multipart boundary preserved.
type ImageEditRequest struct {
	// Images are uploaded as multipart form fields named "image[]" (array support)
	Prompt         string `json:"prompt"`                    // Required
	Model          string `json:"model,omitempty"`           // e.g., "gpt-image-2-all"
	ResponseFormat string `json:"response_format,omitempty"` // "url" or "b64_json"
	// Note: Mask, N, Size are also supported but omitted for brevity
}

// ImageEditResponse has the same structure as ImageGenerationResponse.
type ImageEditResponse = ImageGenerationResponse
