// internal/kieai/client.go
package kieai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"goloop/internal/model"
)

// ErrKieAI represents an API-level error from KIE.AI.
type ErrKieAI struct {
	Code    int
	Message string
}

func (e *ErrKieAI) Error() string {
	return fmt.Sprintf("kieai: HTTP %d: %s", e.Code, e.Message)
}

// Client communicates with the KIE.AI REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:    100,
				IdleConnTimeout: 90 * time.Second,
			},
		},
	}
}

// CreateTask submits an image generation task to KIE.AI.
// apiKey is the bearer token extracted from the client's request header.
func (c *Client) CreateTask(ctx context.Context, apiKey string, req *model.KieAICreateTaskRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("kieai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/jobs/createTask", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("kieai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("kieai: createTask request: %w", err)
	}
	defer resp.Body.Close()

	const maxRespSize = 1 << 20 // 1MB
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRespSize))
	if err != nil {
		return "", fmt.Errorf("kieai: read createTask response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", &ErrKieAI{Code: resp.StatusCode, Message: string(data)}
	}

	var result model.KieAICreateTaskResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("kieai: unmarshal createTask response: %w", err)
	}
	if result.Data.TaskID == "" {
		return "", fmt.Errorf("kieai: createTask returned empty taskId: %s", result.Msg)
	}

	return result.Data.TaskID, nil
}

// GetTaskStatus polls KIE.AI for the current status of a task.
func (c *Client) GetTaskStatus(ctx context.Context, apiKey, taskID string) (*model.KieAIRecordData, error) {
	url := c.baseURL + "/api/v1/jobs/recordInfo?taskId=" + taskID

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("kieai: build recordInfo request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("kieai: recordInfo request: %w", err)
	}
	defer resp.Body.Close()

	const maxRespSize = 1 << 20 // 1MB
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRespSize))
	if err != nil {
		return nil, fmt.Errorf("kieai: read recordInfo response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &ErrKieAI{Code: resp.StatusCode, Message: string(data)}
	}

	var result model.KieAIRecordInfoResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("kieai: unmarshal recordInfo: %w", err)
	}

	return &result.Data, nil
}
