// internal/model/google.go
package model

// --- Google API Request ---

type GoogleRequest struct {
	Contents         []Content         `json:"contents"`
	GenerationConfig *GenerationConfig `json:"generationConfig,omitempty"`
	SafetySettings   []SafetySetting   `json:"safetySettings,omitempty"`
	SystemInstruction *Content         `json:"systemInstruction,omitempty"`
	Tools            []Tool            `json:"tools,omitempty"`
	ToolConfig       *ToolConfig       `json:"toolConfig,omitempty"`
	CachedContent    string            `json:"cachedContent,omitempty"`
}

type Content struct {
	Parts []Part `json:"parts"`
	Role  string `json:"role,omitempty"`
}

type Part struct {
	Text       string      `json:"text,omitempty"`
	// InlineData supports both REST API (inline_data) and Vertex AI (inlineData) formats
	InlineData     *InlineData `json:"inline_data,omitempty"`
	InlineDataAlt  *InlineData `json:"inlineData,omitempty"` // Vertex AI format alias
	// FileData supports both REST API (file_data) and Vertex AI (fileData) formats
	FileData       *FileData   `json:"file_data,omitempty"`
	FileDataAlt    *FileData   `json:"fileData,omitempty"`   // Vertex AI format alias
}

type InlineData struct {
	// MimeType supports both REST API (mime_type) and Vertex AI (mimeType) formats
	MimeType    string `json:"mime_type,omitempty"`
	MimeTypeAlt string `json:"mimeType,omitempty"` // Vertex AI format alias
	Data        string `json:"data"` // base64 encoded
}

type FileData struct {
	// MimeType supports both REST API (mime_type) and Vertex AI (mimeType) formats
	MimeType    string `json:"mime_type,omitempty"`
	MimeTypeAlt string `json:"mimeType,omitempty"` // Vertex AI format alias
	// FileURI supports both REST API (file_uri) and Vertex AI (fileUri) formats
	FileURI     string `json:"file_uri,omitempty"`
	FileURIAlt  string `json:"fileUri,omitempty"`  // Vertex AI format alias
}

// SafetySetting controls blocking of harmful content for a specific category.
type SafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// Tool represents a tool the model may use to generate a response.
type Tool struct {
	FunctionDeclarations []FunctionDeclaration `json:"functionDeclarations,omitempty"`
	GoogleSearch         *GoogleSearch         `json:"googleSearch,omitempty"`
	// REST API uses snake_case key; both are kept to accept either form.
	GoogleSearchSnake *GoogleSearch `json:"google_search,omitempty"`
}

// FunctionDeclaration is a minimal representation of a callable function tool.
type FunctionDeclaration struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// GoogleSearch configures the Grounding with Google Search tool.
type GoogleSearch struct {
	SearchTypes *SearchTypes `json:"searchTypes,omitempty"`
}

// SearchTypes enables specific search modes within the Google Search tool.
type SearchTypes struct {
	// WebSearch enables standard web grounding.
	WebSearch   *WebSearch   `json:"webSearch,omitempty"`
	// ImageSearch enables Google Image Search grounding (3.1 Flash only).
	ImageSearch *ImageSearch `json:"imageSearch,omitempty"`
}

// WebSearch is an empty object that enables web search grounding.
type WebSearch struct{}

// ImageSearch is an empty object that enables image search grounding.
type ImageSearch struct{}

// ToolConfig is the shared configuration for all provided tools.
type ToolConfig struct {
	FunctionCallingConfig *FunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

// FunctionCallingConfig controls how the model calls functions.
type FunctionCallingConfig struct {
	Mode             string   `json:"mode,omitempty"`
	AllowedFunctions []string `json:"allowedFunctionNames,omitempty"`
}

type GenerationConfig struct {
	ResponseModalities []string     `json:"responseModalities,omitempty"`
	ImageConfig        *ImageConfig `json:"imageConfig,omitempty"`

	// Sampling parameters
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
	TopK        *float64 `json:"topK,omitempty"`

	// Output length
	CandidateCount  *int `json:"candidateCount,omitempty"`
	MaxOutputTokens *int `json:"maxOutputTokens,omitempty"`

	// Stop sequences
	StopSequences []string `json:"stopSequences,omitempty"`

	// Response format
	ResponseMimeType string `json:"responseMimeType,omitempty"`

	// Penalties
	PresencePenalty  *float64 `json:"presencePenalty,omitempty"`
	FrequencyPenalty *float64 `json:"frequencyPenalty,omitempty"`

	// Reproducibility
	Seed *int `json:"seed,omitempty"`

	// Log probabilities
	ResponseLogprobs *bool `json:"responseLogprobs,omitempty"`
	Logprobs         *int  `json:"logprobs,omitempty"`
}

type ImageConfig struct {
	AspectRatio      string              `json:"aspectRatio,omitempty"`
	ImageSize        string              `json:"imageSize,omitempty"`
	OutputFormat     string              `json:"outputFormat,omitempty"` // non-standard convenience field kept for backward compat
	PersonGeneration string              `json:"personGeneration,omitempty"`
	ProminentPeople  string              `json:"prominentPeople,omitempty"`
	ImageOutputOptions *ImageOutputOptions `json:"imageOutputOptions,omitempty"`
}

// ImageOutputOptions controls the format of generated images per the Google API spec.
type ImageOutputOptions struct {
	MimeType           string `json:"mimeType,omitempty"`
	CompressionQuality *int   `json:"compressionQuality,omitempty"`
}

// --- Google API Response ---

type GoogleResponse struct {
	Candidates []Candidate `json:"candidates"`
}

// StreamingResponse 是 SSE 流式响应的数据结构
type StreamingResponse struct {
	Candidates    []Candidate    `json:"candidates"`
	UsageMetadata *UsageMetadata `json:"usageMetadata,omitempty"`
	ModelVersion  string         `json:"modelVersion,omitempty"`
	ResponseId    string         `json:"responseId,omitempty"`
}

type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount int `json:"candidatesTokenCount,omitempty"`
	TotalTokenCount      int `json:"totalTokenCount,omitempty"`
}

type Candidate struct {
	Content          Content           `json:"content"`
	FinishReason     string            `json:"finishReason"`
	GroundingMetadata *GroundingMetadata `json:"groundingMetadata,omitempty"`
}

// GroundingMetadata is returned when Grounding with Google Search is used.
type GroundingMetadata struct {
	SearchEntryPoint  *SearchEntryPoint  `json:"searchEntryPoint,omitempty"`
	GroundingChunks   []GroundingChunk   `json:"groundingChunks,omitempty"`
	GroundingSupports []GroundingSupport `json:"groundingSupports,omitempty"`
	ImageSearchQueries []string          `json:"imageSearchQueries,omitempty"`
}

// SearchEntryPoint contains the rendered HTML for search suggestions.
type SearchEntryPoint struct {
	RenderedContent string `json:"renderedContent,omitempty"`
}

// GroundingChunk represents a single grounding source.
type GroundingChunk struct {
	Web   *GroundingChunkWeb   `json:"web,omitempty"`
	Image *GroundingChunkImage `json:"image,omitempty"`
}

// GroundingChunkWeb is a web source used for grounding.
type GroundingChunkWeb struct {
	URI   string `json:"uri,omitempty"`
	Title string `json:"title,omitempty"`
}

// GroundingChunkImage is an image source used for grounding (image search).
type GroundingChunkImage struct {
	URI      string `json:"uri,omitempty"`
	ImageURI string `json:"image_uri,omitempty"`
}

// GroundingSupport maps generated content to its grounding source chunks.
type GroundingSupport struct {
	GroundingChunkIndices []int   `json:"groundingChunkIndices,omitempty"`
	ConfidenceScores      []float64 `json:"confidenceScores,omitempty"`
}

// --- Google API Error ---

type GoogleError struct {
	Error GoogleErrorDetail `json:"error"`
}

type GoogleErrorDetail struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}
