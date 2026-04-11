package kieai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"goloop/internal/model"
)

// Config holds KIE.AI channel configuration.
type Config struct {
	BaseURL         string
	Timeout         time.Duration
	InitialInterval time.Duration
	MaxInterval     time.Duration
	MaxWaitTime     time.Duration
	RetryAttempts   int
}

func defaultConfig(baseURL string) Config {
	return Config{
		BaseURL:         baseURL,
		Timeout:         120 * time.Second,
		InitialInterval: 2 * time.Second,
		MaxInterval:     10 * time.Second,
		MaxWaitTime:     120 * time.Second,
		RetryAttempts:   3,
	}
}

// Channel implements core.Channel for KIE.AI.
type Channel struct {
	name          string
	baseURL       string
	httpClient    *http.Client
	pool          *AccountPool
	reqTransform  *RequestTransformer
	respTransform *ResponseTransformer
	cfg           Config
}

// --- RequestTransformer ---

// ModelDefaults holds default parameters for a model.
type ModelDefaults struct {
	KieAIModel    string
	AspectRatio   string
	Resolution    string
	OutputFormat  string
}

// RequestTransformer transforms GoogleRequests to KIE.AI requests.
type RequestTransformer struct {
	modelMapping map[string]ModelDefaults
}

// NewRequestTransformer creates a new RequestTransformer.
func NewRequestTransformer(modelMapping map[string]ModelDefaults) *RequestTransformer {
	return &RequestTransformer{modelMapping: modelMapping}
}

// Transform transforms a GoogleRequest to a model.KieAICreateTaskRequest.
func (t *RequestTransformer) Transform(ctx context.Context, req *model.GoogleRequest, modelName string) (*model.KieAICreateTaskRequest, error) {
	// Extract text from the first content's first part
	var promptText string
	if len(req.Contents) > 0 && len(req.Contents[0].Parts) > 0 {
		promptText = req.Contents[0].Parts[0].Text
	}

	defaults, ok := t.modelMapping[modelName]
	if !ok {
		return nil, fmt.Errorf("kieai: unknown model %q", modelName)
	}

	return &model.KieAICreateTaskRequest{
		Model: defaults.KieAIModel,
		Input: model.KieAIInput{
			Prompt:       promptText,
			AspectRatio:  defaults.AspectRatio,
			Resolution:   defaults.Resolution,
			OutputFormat: defaults.OutputFormat,
		},
	}, nil
}

// --- ResponseTransformer ---

// ResponseTransformer transforms KIE.AI responses to GoogleResponses.
type ResponseTransformer struct{}

// NewResponseTransformer creates a new ResponseTransformer.
func NewResponseTransformer() *ResponseTransformer {
	return &ResponseTransformer{}
}

// ToGoogleResponse converts result URLs to a GoogleResponse.
func (t *ResponseTransformer) ToGoogleResponse(ctx context.Context, urls []string) (*model.GoogleResponse, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("kieai: no result URLs")
	}
	// Build a simple GoogleResponse with the first URL as an inline data image
	return &model.GoogleResponse{
		Candidates: []model.Candidate{{
			Content: model.Content{
				Parts: []model.Part{{
					InlineData: &model.InlineData{
						MimeType: "image/png",
						Data:     "", // URLs are not base64 data
					},
				}},
			},
		}},
	}, nil
}

// NewChannel creates a new KIE.AI channel plugin.
func NewChannel(baseURL string, pool *AccountPool, cfg Config) *Channel {
	if cfg.InitialInterval == 0 {
		cfg = defaultConfig(baseURL)
	}
	ch := &Channel{
		name:       "kieai",
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: cfg.Timeout},
		pool:       pool,
		cfg:        cfg,
	}

	modelMapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
		"gemini-3-pro-image-preview":     {KieAIModel: "nano-banana-pro", AspectRatio: "1:1", Resolution: "2K", OutputFormat: "png"},
		"gemini-2.5-flash-image":         {KieAIModel: "google/nano-banana", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}
	ch.reqTransform = NewRequestTransformer(modelMapping)
	ch.respTransform = NewResponseTransformer()
	return ch
}

func (ch *Channel) Name() string                                              { return ch.name }
func (ch *Channel) IsAvailable() bool                                          { return ch.pool != nil && len(ch.pool.List()) > 0 }
func (ch *Channel) HealthScore() float64 {
	accounts := ch.pool.List()
	if len(accounts) == 0 {
		return 0
	}
	var total float64
	for _, acc := range accounts {
		total += acc.HealthScore()
	}
	return total / float64(len(accounts))
}

func (ch *Channel) Generate(ctx context.Context, apiKey string, req *model.GoogleRequest, modelName string) (*model.GoogleResponse, error) {
	return nil, fmt.Errorf("kieai: Generate not supported, use SubmitTask + PollTask")
}

func (ch *Channel) SubmitTask(ctx context.Context, apiKey string, req *model.GoogleRequest, modelName string) (string, error) {
	kieReq, err := ch.reqTransform.Transform(ctx, req, modelName)
	if err != nil {
		return "", fmt.Errorf("kieai: transform: %w", err)
	}

	body, err := json.Marshal(kieReq)
	if err != nil {
		return "", fmt.Errorf("kieai: marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		ch.baseURL+"/api/v1/jobs/createTask", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := ch.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kieai: HTTP %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Code int    `json:"code"`
		Data struct {
			TaskID string `json:"taskId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if result.Data.TaskID == "" {
		return "", fmt.Errorf("kieai: empty taskId")
	}
	return result.Data.TaskID, nil
}

func (ch *Channel) PollTask(ctx context.Context, apiKey, taskID string) (*model.GoogleResponse, error) {
	deadline := time.Now().Add(ch.cfg.MaxWaitTime)
	interval := ch.cfg.InitialInterval
	consecutiveFails := 0

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("kieai: task %q timed out", taskID)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		record, err := ch.getTaskStatus(ctx, apiKey, taskID)
		if err != nil {
			consecutiveFails++
			slog.Warn("kieai: poll failed", "taskId", taskID, "fails", consecutiveFails, "err", err)
			if consecutiveFails >= ch.cfg.RetryAttempts {
				return nil, fmt.Errorf("kieai: task %q: %d consecutive failures: %w", taskID, consecutiveFails, err)
			}
			interval = min(interval*2, ch.cfg.MaxInterval)
			continue
		}
		consecutiveFails = 0

		switch record.State {
		case "success":
			if record.ResultJSON() == nil || len(record.ResultJSON().ResultURLs) == 0 {
				return nil, fmt.Errorf("kieai: no result URLs")
			}
			return ch.respTransform.ToGoogleResponse(ctx, record.ResultJSON().ResultURLs)
		case "fail":
			return nil, fmt.Errorf("kieai: task %q failed: %s", taskID, record.FailReason)
		case "waiting", "queuing", "generating":
			interval = min(interval*2, ch.cfg.MaxInterval)
		}
	}
}

func (ch *Channel) getTaskStatus(ctx context.Context, apiKey, taskID string) (*model.KieAIRecordData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		ch.baseURL+"/api/v1/jobs/recordInfo?taskId="+taskID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := ch.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kieai: HTTP %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Data struct {
			TaskID        string `json:"taskId"`
			State         string `json:"state"`
			ResultJSONRaw string `json:"resultJson,omitempty"`
			FailReason    string `json:"failReason,omitempty"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &model.KieAIRecordData{
		TaskID:        result.Data.TaskID,
		State:         result.Data.State,
		ResultJSONRaw: result.Data.ResultJSONRaw,
		FailReason:    result.Data.FailReason,
	}, nil
}

// Probe sends a lightweight health check for a specific account.
func (ch *Channel) Probe(account Account) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		ch.baseURL+"/api/v1/user/info", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+account.APIKey())

	resp, err := ch.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}