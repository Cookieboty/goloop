package gemini_callback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"goloop/internal/core"
	"goloop/internal/model"
	"goloop/internal/storage"
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
		Timeout:         1300 * time.Second,
		InitialInterval: 2 * time.Second,
		MaxInterval:     10 * time.Second,
		MaxWaitTime:     1200 * time.Second,
		RetryAttempts:   3,
	}
}

// Channel implements core.Channel for KIE.AI.
// It embeds core.BaseChannel to inherit all boilerplate methods; only the
// KIE.AI-specific async task flow (SubmitTask + PollTask) and Probe are
// overridden here.
type Channel struct {
	core.BaseChannel                   // provides Name/Weight/IsAvailable/HealthScore/Admin methods
	reqTransform   *RequestTransformer
	respTransform  *ResponseTransformer
	cfg            Config
	activeAccounts sync.Map // taskID -> core.Account; used to call pool.Return on completion
	imageOnlyTasks sync.Map // taskID -> bool; true when request has responseModalities=["image"] only
}

// NewChannel creates a new KIE.AI channel plugin.
// name is the unique channel identifier (e.g. "kie").
func NewChannel(name, baseURL string, weight int, pool *AccountPool, cfg Config, store *storage.Store) *Channel {
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

	slog.Info("creating gemini_callback channel",
		"name", name,
		"baseURL", baseURL,
		"weight", weight,
		"timeout", cfg.Timeout,
		"maxWaitTime", cfg.MaxWaitTime,
		"initialInterval", cfg.InitialInterval,
		"maxInterval", cfg.MaxInterval,
		"retryAttempts", cfg.RetryAttempts)

	modelMapping := map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
		"gemini-3-pro-image-preview":     {KieAIModel: "nano-banana-pro", AspectRatio: "1:1", Resolution: "2K", OutputFormat: "png"},
		"gemini-2.5-flash-image":         {KieAIModel: "google/nano-banana", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
	}

	uploader := NewUploader(baseURL, cfg.Timeout, cfg.RetryAttempts)

	ch := &Channel{
		BaseChannel:   core.NewBaseChannel(name, "gemini_callback", baseURL, weight, pool, cfg.Timeout),
		cfg:           cfg,
		reqTransform:  NewRequestTransformer(modelMapping, uploader),
		respTransform: NewResponseTransformer(store),
	}
	return ch
}

// Generate is not supported by KIE.AI (async-only); returns ErrNotSupported.
// BaseChannel already provides this default, but we keep it explicit for clarity.
func (ch *Channel) Generate(_ context.Context, _ *model.GoogleRequest, _ string) (*model.GoogleResponse, error) {
	return nil, core.ErrNotSupported
}

func (ch *Channel) SubmitTask(ctx context.Context, req *model.GoogleRequest, modelName string) (string, string, error) {
	log := slog.With("channel", "kieai", "model", modelName)

	acc, err := ch.Pool.Select()
	if err != nil {
		log.Warn("submitTask: no account available", "err", err)
		return "", "", fmt.Errorf("kieai: no account available: %w", err)
	}
	acc.IncUsage()

	kieReq, err := ch.reqTransform.Transform(ctx, req, modelName, acc.APIKey())
	if err != nil {
		log.Warn("submitTask: transform failed", "err", err)
		ch.Pool.Return(acc, false)
		return "", "", fmt.Errorf("kieai: transform: %w", err)
	}

	body, err := json.Marshal(kieReq)
	if err != nil {
		log.Warn("submitTask: marshal failed", "err", err)
		ch.Pool.Return(acc, false)
		return "", "", fmt.Errorf("kieai: marshal: %w", err)
	}

	log.Info("submitTask: creating job", "bodyLen", len(body), "requestBody", string(body))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		ch.BaseURL+"/api/v1/jobs/createTask", bytes.NewReader(body))
	if err != nil {
		log.Warn("submitTask: build request failed", "err", err)
		ch.Pool.Return(acc, false)
		return "", "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+acc.APIKey())

	resp, err := ch.HTTPClient.Do(httpReq)
	if err != nil {
		log.Warn("submitTask: HTTP request failed", "err", err)
		ch.Pool.Return(acc, false)
		return "", "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		log.Warn("submitTask: read response failed", "err", err)
		ch.Pool.Return(acc, false)
		return "", "", err
	}
	
	log.Info("submitTask: received response", "status", resp.StatusCode, "responseBody", string(data))
	
	if resp.StatusCode != http.StatusOK {
		log.Warn("submitTask: HTTP error", "status", resp.StatusCode, "body", string(data))
		ch.Pool.Return(acc, false)
		return "", "", fmt.Errorf("kieai: HTTP %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			TaskID string `json:"taskId"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		log.Warn("submitTask: unmarshal failed", "err", err, "body", string(data))
		ch.Pool.Return(acc, false)
		return "", "", err
	}
	
	log.Info("submitTask: parsed response", "code", result.Code, "taskId", result.Data.TaskID, "msg", result.Msg)
	
	if result.Data.TaskID == "" {
		log.Warn("submitTask: empty taskId received", "responseCode", result.Code, "msg", result.Msg, "fullResponse", string(data))
		ch.Pool.Return(acc, false)
		return "", "", fmt.Errorf("kieai: empty taskId (code=%d, msg=%s)", result.Code, result.Msg)
	}

	// Store account reference so PollTask can call Return when the task completes.
	ch.activeAccounts.Store(result.Data.TaskID, acc)
	// Store whether this task is image-only so PollTask can omit the text part.
	ch.imageOnlyTasks.Store(result.Data.TaskID, isImageOnly(req))
	log.Info("submitTask: task created successfully", "taskId", result.Data.TaskID)
	return result.Data.TaskID, acc.APIKey(), nil
}

// isImageOnly returns true when the request's responseModalities contains only
// "image" (and no "text"), indicating the caller does not want a text part.
func isImageOnly(req *model.GoogleRequest) bool {
	if req.GenerationConfig == nil || len(req.GenerationConfig.ResponseModalities) == 0 {
		return false
	}
	for _, m := range req.GenerationConfig.ResponseModalities {
		if strings.EqualFold(m, "text") {
			return false
		}
	}
	return true
}

func (ch *Channel) PollTask(ctx context.Context, apiKey, taskID string) (*model.GoogleResponse, error) {
	log := slog.With("taskId", taskID, "channel", "kieai")
	log.Info("pollTask: started")
	
	// Return the account to the pool when polling completes (success or failure).
	var pollSuccess bool
	defer func() {
		if raw, ok := ch.activeAccounts.LoadAndDelete(taskID); ok {
			ch.Pool.Return(raw.(core.Account), pollSuccess)
		}
	}()

	deadline := time.Now().Add(ch.cfg.MaxWaitTime)
	interval := ch.cfg.InitialInterval
	consecutiveFails := 0
	pollCount := 0

	log.Info("pollTask: config", "maxWaitTime", ch.cfg.MaxWaitTime, "initialInterval", ch.cfg.InitialInterval, "maxInterval", ch.cfg.MaxInterval)

	for {
		if time.Now().After(deadline) {
			log.Warn("pollTask: timeout reached", "pollCount", pollCount, "elapsed", time.Since(time.Now().Add(-ch.cfg.MaxWaitTime)))
			return nil, fmt.Errorf("kieai: task %q timed out after %d polls", taskID, pollCount)
		}
		select {
		case <-ctx.Done():
			log.Info("pollTask: context cancelled", "pollCount", pollCount)
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		pollCount++
		record, err := ch.getTaskStatus(ctx, apiKey, taskID)
		if err != nil {
			consecutiveFails++
			log.Warn("pollTask: status check failed", "pollCount", pollCount, "consecutiveFails", consecutiveFails, "err", err)
			if consecutiveFails >= ch.cfg.RetryAttempts {
				return nil, fmt.Errorf("kieai: task %q: %d consecutive failures: %w", taskID, consecutiveFails, err)
			}
			interval = min(interval*2, ch.cfg.MaxInterval)
			continue
		}
		consecutiveFails = 0

		log.Info("pollTask: status received", "pollCount", pollCount, "state", record.State)

		switch record.State {
		case "success":
			if record.ResultJSON() == nil || len(record.ResultJSON().ResultURLs) == 0 {
				log.Warn("pollTask: success but no result URLs")
				return nil, fmt.Errorf("kieai: no result URLs")
			}
			imageOnly := false
			if v, ok := ch.imageOnlyTasks.LoadAndDelete(taskID); ok {
				imageOnly, _ = v.(bool)
			}
		log.Info("pollTask: task completed successfully", "taskId", taskID, "channel", "kieai", "pollCount", pollCount, "resultCount", len(record.ResultJSON().ResultURLs))
		resp, err := ch.respTransform.ToGoogleResponse(ctx, record.ResultJSON().ResultURLs, imageOnly)
		if err != nil {
			log.Error("pollTask: failed to transform response", "taskId", taskID, "channel", "kieai", "err", err)
			return nil, err
		}
		pollSuccess = true
		log.Info("pollTask: response transformed successfully", "taskId", taskID, "channel", "kieai")
		return resp, nil
		case "fail":
			log.Warn("pollTask: task failed", "pollCount", pollCount, "reason", record.FailReason)
			return nil, fmt.Errorf("kieai: task %q failed: %s", taskID, record.FailReason)
		case "waiting", "queuing", "generating":
			log.Debug("pollTask: task still processing", "pollCount", pollCount, "state", record.State, "nextInterval", interval*2)
			interval = min(interval*2, ch.cfg.MaxInterval)
		default:
			log.Warn("pollTask: unknown state", "pollCount", pollCount, "state", record.State)
			interval = min(interval*2, ch.cfg.MaxInterval)
		}
	}
}

func (ch *Channel) getTaskStatus(ctx context.Context, apiKey, taskID string) (*model.KieAIRecordData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		ch.BaseURL+"/api/v1/jobs/recordInfo?taskId="+url.QueryEscape(taskID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := ch.HTTPClient.Do(req)
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

// Probe overrides BaseChannel's default probe with a KIE.AI-specific endpoint.
func (ch *Channel) Probe(account core.Account) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		ch.BaseURL+"/api/v1/user/info", nil)
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

// The following methods are inherited from core.BaseChannel and do NOT need
// to be re-implemented here:
//   - Name(), Weight(), SetChannelWeight()
//   - IsAvailable(), HealthScore()
//   - GetAccountPool()
//   - ListAccounts(), ResetAccount(), RetireAccount(), ProbeAccount(), SetWeight()
