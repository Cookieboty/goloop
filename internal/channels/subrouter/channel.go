package subrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"goloop/internal/core"
	"goloop/internal/model"
)

// Config holds subrouter channel configuration.
type Config struct {
	// ProbeModel is the model name used for lightweight health probes.
	// Defaults to "gpt-4o-mini" if empty.
	ProbeModel string
}

// Channel implements core.Channel for any OpenAI-compatible upstream API.
// It embeds core.BaseChannel to inherit all boilerplate methods; only
// Generate and Probe are overridden with subrouter-specific logic.
//
// Integrating a new OpenAI-compatible provider requires:
//  1. Create a *core.DefaultAccountPool and add API keys.
//  2. Call NewChannel with the provider's base URL.
//  3. Register the channel with the PluginRegistry in main.go.
//
// No other files need to change.
type Channel struct {
	core.BaseChannel // provides Name/Weight/IsAvailable/HealthScore/Admin/GetAccountPool
	cfg Config
}

// NewChannel creates a subrouter channel for an OpenAI-compatible API.
// name is the unique channel identifier (e.g. "subrouter", "openai-backup").
func NewChannel(name, baseURL string, weight int, pool *core.DefaultAccountPool, timeout time.Duration, cfg Config) *Channel {
	if cfg.ProbeModel == "" {
		cfg.ProbeModel = "gpt-4o-mini"
	}
	return &Channel{
		BaseChannel: core.NewBaseChannel(name, "subrouter", baseURL, weight, pool, timeout),
		cfg:         cfg,
	}
}

// Generate calls the OpenAI-compatible /v1/chat/completions endpoint
// synchronously and converts the response to Google format.
func (ch *Channel) Generate(ctx context.Context, req *model.GoogleRequest, modelName string) (*model.GoogleResponse, error) {
	acc, err := ch.Pool.Select()
	if err != nil {
		return nil, fmt.Errorf("subrouter: no account available: %w", err)
	}
	acc.IncUsage()

	var success bool
	defer func() { ch.Pool.Return(acc, success) }()

	openAIReq := googleToOpenAI(req, modelName)
	body, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, fmt.Errorf("subrouter: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		ch.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+acc.APIKey())

	resp, err := ch.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("subrouter: HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MiB limit
	if err != nil {
		return nil, fmt.Errorf("subrouter: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subrouter: HTTP %d: %s", resp.StatusCode, string(data))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(data, &chatResp); err != nil {
		return nil, fmt.Errorf("subrouter: unmarshal response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("subrouter: empty choices in response")
	}

	success = true
	return openAIToGoogle(&chatResp), nil
}

// Probe overrides BaseChannel's default probe with a minimal chat request
// to verify the account is working. Uses a cheap model to minimise token cost.
func (ch *Channel) Probe(account core.Account) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	probeReq := &ChatRequest{
		Model: ch.cfg.ProbeModel,
		Messages: []ChatMessage{
			{Role: "user", Content: "hi"},
		},
	}
	body, err := json.Marshal(probeReq)
	if err != nil {
		return false
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		ch.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return false
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+account.APIKey())

	resp, err := ch.HTTPClient.Do(httpReq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Stream sends a streaming request to the OpenAI /v1/chat/completions endpoint
// with stream:true, then converts each SSE chunk from OpenAI format to Google
// StreamingResponse format and writes it to w as a Google SSE event.
//
// OpenAI SSE format:   data: {"choices":[{"delta":{"content":"hello"},...}],...}
// Google SSE format:   data: {"candidates":[{"content":{"parts":[{"text":"hello"}],...},...}],...}
//
// Stream returns a non-nil error only if the upstream request itself fails
// before any bytes are written to w.
func (ch *Channel) Stream(ctx context.Context, req *model.GoogleRequest, modelName string, w core.ResponseWriter) error {
	acc, err := ch.Pool.Select()
	if err != nil {
		return fmt.Errorf("subrouter: no account available: %w", err)
	}
	acc.IncUsage()

	var success bool
	defer func() { ch.Pool.Return(acc, success) }()

	openAIReq := googleToOpenAI(req, modelName)
	openAIReq.Stream = true
	body, err := json.Marshal(openAIReq)
	if err != nil {
		return fmt.Errorf("subrouter: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		ch.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+acc.APIKey())
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := ch.HTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("subrouter: stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("subrouter: HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	// Headers committed — from this point errors are silently dropped.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	success = true

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// OpenAI sends blank lines as SSE separators — forward them.
		if line == "" {
			if _, err := w.Write([]byte("\n")); err != nil {
				return nil
			}
			w.Flush()
			continue
		}

		// Terminal sentinel — forward as-is and stop.
		if line == "data: [DONE]" {
			w.Write([]byte("data: [DONE]\n\n")) //nolint:errcheck
			w.Flush()
			return nil
		}

		if !strings.HasPrefix(line, "data: ") {
			// Non-data line (e.g. "event: ...") — skip.
			continue
		}

		payload := strings.TrimPrefix(line, "data: ")

		// Decode the OpenAI streaming chunk.
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
					Role    string `json:"role"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
				Index        int     `json:"index"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// Unparseable chunk — skip silently.
			continue
		}

		// Build a Google StreamingResponse from this chunk.
		googleResp := buildStreamingChunk(chunk.Choices)
		if googleResp == nil {
			continue
		}

		jsonBytes, err := json.Marshal(googleResp)
		if err != nil {
			continue
		}
		if _, err := w.Write(append([]byte("data: "), append(jsonBytes, '\n', '\n')...)); err != nil {
			return nil
		}
		w.Flush()
	}
	return nil
}

// buildStreamingChunk converts a single OpenAI SSE delta into a Google
// StreamingResponse. Returns nil when there is nothing meaningful to emit.
func buildStreamingChunk(choices []struct {
	Delta struct {
		Content string `json:"content"`
		Role    string `json:"role"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
	Index        int     `json:"index"`
}) *model.StreamingResponse {
	if len(choices) == 0 {
		return nil
	}
	choice := choices[0]

	text := choice.Delta.Content
	finishReason := ""
	if choice.FinishReason != nil {
		finishReason = mapFinishReasonToGoogle(*choice.FinishReason)
	}

	// Skip empty deltas with no finish reason (e.g. role-only first chunk).
	if text == "" && finishReason == "" {
		return nil
	}

	candidate := model.Candidate{
		Content: model.Content{
			Parts: []model.Part{{Text: text}},
			Role:  "model",
		},
		FinishReason: finishReason,
	}
	return &model.StreamingResponse{
		Candidates: []model.Candidate{candidate},
	}
}

// Compile-time assertion.
var _ core.StreamGenerator = (*Channel)(nil)

// SubmitTask and PollTask are NOT overridden; BaseChannel returns
// core.ErrNotSupported for both, causing the handler to use Generate/Stream instead.
