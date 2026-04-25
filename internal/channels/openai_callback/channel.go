package openai_callback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
	
	"goloop/internal/core"
	"goloop/internal/model"
)

// Config holds openai_callback channel configuration.
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
		Timeout:         1300 * time.Second,
		InitialInterval: 2 * time.Second,
		MaxInterval:     10 * time.Second,
		MaxWaitTime:     1200 * time.Second,
		RetryAttempts:   3,
	}
}

// AccountPool wraps DefaultAccountPool with the same name for backward compatibility.
type AccountPool = core.DefaultAccountPool

// NewAccountPool creates a new account pool.
func NewAccountPool() *AccountPool {
	return core.NewDefaultAccountPool()
}

// Channel implements core.Channel for OpenAI async callback API.
// It embeds core.BaseChannel and implements async task flow (SubmitTask + PollTask).
type Channel struct {
	core.BaseChannel
	cfg            Config
	activeAccounts sync.Map // taskID -> core.Account
}

// NewChannel creates a new OpenAI callback channel plugin.
// name is the unique channel identifier (e.g. "my-openai").
func NewChannel(name, baseURL string, weight int, pool *AccountPool, cfg Config) *Channel {
	// Apply defaults for any zero-value fields
	defaults := defaultConfig(baseURL)
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaults.BaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaults.Timeout
	}
	if cfg.InitialInterval == 0 {
		cfg.InitialInterval = defaults.InitialInterval
	}
	if cfg.MaxInterval == 0 {
		cfg.MaxInterval = defaults.MaxInterval
	}
	if cfg.MaxWaitTime == 0 {
		cfg.MaxWaitTime = defaults.MaxWaitTime
	}
	if cfg.RetryAttempts == 0 {
		cfg.RetryAttempts = defaults.RetryAttempts
	}
	
	ch := &Channel{
		BaseChannel: core.NewBaseChannel(name, "openai_callback", baseURL, weight, pool, cfg.Timeout),
		cfg:         cfg,
	}
	return ch
}

// Generate is not supported by OpenAI async API; returns ErrNotSupported.
func (ch *Channel) Generate(_ context.Context, _ *model.GoogleRequest, _ string) (*model.GoogleResponse, error) {
	return nil, core.ErrNotSupported
}

// SubmitTask submits an async task to OpenAI and returns taskID + apiKey.
// This is a simplified implementation - actual OpenAI async API may differ.
func (ch *Channel) SubmitTask(ctx context.Context, req *model.GoogleRequest, modelName string) (string, string, error) {
	acc, err := ch.Pool.Select()
	if err != nil {
		return "", "", fmt.Errorf("openai_callback: no account available: %w", err)
	}
	acc.IncUsage()
	
	// Convert GoogleRequest to OpenAI format (simplified)
	openAIReq := convertToOpenAI(req, modelName)
	body, err := json.Marshal(openAIReq)
	if err != nil {
		ch.Pool.Return(acc, false)
		return "", "", fmt.Errorf("openai_callback: marshal request: %w", err)
	}
	
	// Submit task to OpenAI async endpoint
	url := ch.BaseURL + "/v1/chat/completions" // Simplified endpoint
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		ch.Pool.Return(acc, false)
		return "", "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+acc.APIKey())
	
	resp, err := ch.HTTPClient.Do(httpReq)
	if err != nil {
		ch.Pool.Return(acc, false)
		return "", "", fmt.Errorf("openai_callback: submit request failed: %w", err)
	}
	defer resp.Body.Close()
	
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		ch.Pool.Return(acc, false)
		return "", "", fmt.Errorf("openai_callback: read response: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		ch.Pool.Return(acc, false)
		return "", "", fmt.Errorf("openai_callback: HTTP %d: %s", resp.StatusCode, string(data))
	}
	
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		ch.Pool.Return(acc, false)
		return "", "", fmt.Errorf("openai_callback: unmarshal response: %w", err)
	}
	
	taskID := result.ID
	apiKey := acc.APIKey()
	
	// Store account for polling
	ch.activeAccounts.Store(taskID, acc)
	
	slog.Debug("openai_callback: task submitted", "taskID", taskID)
	return taskID, apiKey, nil
}

// PollTask polls for task completion.
func (ch *Channel) PollTask(ctx context.Context, apiKey, taskID string) (*model.GoogleResponse, error) {
	// Retrieve the account that submitted the task
	accValue, ok := ch.activeAccounts.Load(taskID)
	if !ok {
		return nil, fmt.Errorf("openai_callback: task %s not found", taskID)
	}
	acc := accValue.(core.Account)
	
	var success bool
	defer func() {
		ch.Pool.Return(acc, success)
		if success {
			ch.activeAccounts.Delete(taskID)
		}
	}()
	
	// Poll for result (simplified - actual implementation depends on OpenAI's async API)
	url := ch.BaseURL + "/v1/chat/completions/" + taskID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	
	resp, err := ch.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai_callback: poll request failed: %w", err)
	}
	defer resp.Body.Close()
	
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("openai_callback: read response: %w", err)
	}
	
	if resp.StatusCode == http.StatusAccepted {
		// Still processing
		return nil, fmt.Errorf("openai_callback: task still processing")
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai_callback: HTTP %d: %s", resp.StatusCode, string(data))
	}
	
	// Convert OpenAI response to Google format
	googleResp, err := convertFromOpenAI(data)
	if err != nil {
		return nil, fmt.Errorf("openai_callback: convert response: %w", err)
	}
	
	success = true
	return googleResp, nil
}

// Probe performs a lightweight health check.
func (ch *Channel) Probe(account core.Account) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ch.BaseURL+"/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+account.APIKey())
	
	resp, err := ch.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Helper functions for conversion (simplified)
func convertToOpenAI(req *model.GoogleRequest, modelName string) map[string]any {
	messages := make([]map[string]any, 0)
	
	for _, content := range req.Contents {
		role := "user"
		if content.Role == "model" {
			role = "assistant"
		}
		
		message := map[string]any{
			"role":    role,
			"content": "",
		}
		
		// Simplified: just concat text parts
		var textParts []string
		for _, part := range content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
		}
		if len(textParts) > 0 {
			message["content"] = textParts[0]
		}
		
		messages = append(messages, message)
	}
	
	return map[string]any{
		"model":    modelName,
		"messages": messages,
	}
}

func convertFromOpenAI(data []byte) (*model.GoogleResponse, error) {
	var openAIResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	
	if err := json.Unmarshal(data, &openAIResp); err != nil {
		return nil, err
	}
	
	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}
	
	return &model.GoogleResponse{
		Candidates: []model.Candidate{
			{
				Content: model.Content{
					Parts: []model.Part{
						{Text: openAIResp.Choices[0].Message.Content},
					},
					Role: "model",
				},
				FinishReason: "STOP",
			},
		},
	}, nil
}
