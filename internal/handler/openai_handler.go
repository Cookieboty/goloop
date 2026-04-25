package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"goloop/internal/channels/openai_original"
	"goloop/internal/core"
	"goloop/internal/database"
	"goloop/internal/middleware"
	"goloop/internal/model"
)

// OpenAIHandler handles OpenAI-compatible API endpoints:
// - POST /v1/chat/completions
// - POST /v1/images/generations
// - POST /v1/images/edits
// - GET /v1/models
type OpenAIHandler struct {
	router              *core.Router
	registry            *core.PluginRegistry
	issuer              *core.JWTIssuer
	configMgr           *core.ConfigManager
	maxRequestBodyBytes int64
	usageLogger         *core.UsageLogger
}

func NewOpenAIHandler(
	router *core.Router,
	registry *core.PluginRegistry,
	issuer *core.JWTIssuer,
	configMgr *core.ConfigManager,
	maxRequestBodyBytes int64,
	usageLogger *core.UsageLogger,
) *OpenAIHandler {
	if maxRequestBodyBytes <= 0 {
		maxRequestBodyBytes = 50 * 1024 * 1024
	}
	return &OpenAIHandler{
		router:              router,
		registry:            registry,
		issuer:              issuer,
		configMgr:           configMgr,
		maxRequestBodyBytes: maxRequestBodyBytes,
		usageLogger:         usageLogger,
	}
}

// RegisterRoutes mounts the handler onto the provided mux.
// Note: API Key middleware should be applied at the mux level in main.go
func (h *OpenAIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/chat/completions", h.handleChatCompletions)
	mux.HandleFunc("POST /v1/images/generations", h.handleImagesGenerations)
	mux.HandleFunc("POST /v1/images/edits", h.handleImagesEdits)
	mux.HandleFunc("GET /v1/models", h.handleListModels)
}

// handleChatCompletions handles POST /v1/chat/completions (streaming or not).
func (h *OpenAIHandler) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// API Key ID is injected by APIKeyMiddleware
	apiKeyID, ok := middleware.GetAPIKeyID(ctx)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}
	_ = apiKeyID // API Key ID will be used for usage logging
	bodyBytes, ok := h.readBody(w, r)
	if !ok {
		return
	}

	// Peek at stream field. Parsing error is non-fatal — default to false.
	var streamReq struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(bodyBytes, &streamReq)

	candidates, ok := h.selectCandidates(w, r)
	if !ok {
		return
	}

	endpoint := "/v1/chat/completions"
	contentType := clientContentType(r)

	if streamReq.Stream {
		h.dispatchStream(w, r, candidates, contentType, bodyBytes, endpoint)
		return
	}
	h.dispatchNonStream(w, r, candidates, contentType, bodyBytes, endpoint)
}

// handleImagesGenerations handles POST /v1/images/generations (non-streaming).
func (h *OpenAIHandler) handleImagesGenerations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// API Key ID is injected by APIKeyMiddleware
	apiKeyID, ok := middleware.GetAPIKeyID(ctx)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}
	_ = apiKeyID // API Key ID will be used for usage logging
	
	bodyBytes, ok := h.readBody(w, r)
	if !ok {
		return
	}
	candidates, ok := h.selectCandidates(w, r)
	if !ok {
		return
	}
	h.dispatchNonStream(w, r, candidates, clientContentType(r), bodyBytes, "/v1/images/generations")
}

// handleImagesEdits handles POST /v1/images/edits (multipart/form-data).
// The raw multipart body is forwarded with its original Content-Type (which
// contains the boundary) so the upstream can parse it.
func (h *OpenAIHandler) handleImagesEdits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// API Key ID is injected by APIKeyMiddleware
	apiKeyID, ok := middleware.GetAPIKeyID(ctx)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}
	_ = apiKeyID // API Key ID will be used for usage logging
	
	bodyBytes, ok := h.readBody(w, r)
	if !ok {
		return
	}
	candidates, ok := h.selectCandidates(w, r)
	if !ok {
		return
	}
	h.dispatchNonStream(w, r, candidates, clientContentType(r), bodyBytes, "/v1/images/edits")
}

// readBody reads and size-checks the raw request body.
func (h *OpenAIHandler) readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	limited := io.LimitReader(r.Body, h.maxRequestBodyBytes+1)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		h.writeOpenAIError(w, "Failed to read request body", "invalid_request_error", http.StatusBadRequest)
		return nil, false
	}
	if int64(len(bodyBytes)) > h.maxRequestBodyBytes {
		h.writeOpenAIError(w, "Request body too large", "invalid_request_error", http.StatusRequestEntityTooLarge)
		return nil, false
	}
	return bodyBytes, true
}

// selectCandidates returns OpenAI image channel candidates (or writes an error response).
// Only includes OpenAI channel types (openai_original, openai_callback).
func (h *OpenAIHandler) selectCandidates(w http.ResponseWriter, r *http.Request) ([]core.Channel, bool) {
	filter := &core.ChannelTypeFilter{Include: []string{"openai_original", "openai_callback"}}
	candidates, err := h.router.RouteWithTypeFilter(r.Context(), filter)
	
	if err != nil {
		slog.Error("no OpenAI image channels available", "error", err)
		h.writeOpenAIError(w, "No OpenAI image channels available", "api_error", http.StatusServiceUnavailable)
		return nil, false
	}
	return candidates, true
}

// dispatchNonStream iterates candidates and writes the first non-fallbackable
// response directly to the client. Fallback-worthy statuses (5xx/429/408/401)
// trigger the next candidate; all other statuses (including 2xx and most 4xx)
// are returned verbatim to the client.
func (h *OpenAIHandler) dispatchNonStream(
	w http.ResponseWriter, r *http.Request,
	candidates []core.Channel, contentType string, bodyBytes []byte, endpoint string,
) {
	ctx := r.Context()
	var lastErr error
	var lastChannel string
	var totalLatency int64
	requestIP := extractClientIP(r)
	
	// 从请求体中提取 model（如果可能）
	model := extractModelFromBody(bodyBytes)

	for _, ch := range candidates {
		chLog := slog.With("channel", ch.Name(), "endpoint", endpoint)
		lastChannel = ch.Name()

		rawGen, ok := ch.(core.OpenAIRawGenerator)
		if !ok {
			chLog.Warn("candidate does not implement OpenAIRawGenerator; skipping")
			continue
		}

		// Apply model mapping for this channel
		transformedBody := bodyBytes
		if h.configMgr != nil {
			var reqBody map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &reqBody); err == nil {
				if sourceModel, ok := reqBody["model"].(string); ok && sourceModel != "" {
					if targetModel := h.configMgr.GetModelMapping(ch.Name(), sourceModel); targetModel != "" {
						reqBody["model"] = targetModel
						if newBody, err := json.Marshal(reqBody); err == nil {
							transformedBody = newBody
							chLog.Debug("applied model mapping", "source", sourceModel, "target", targetModel)
						}
					}
				}
			}
		}
		
		start := time.Now()
		resp, err := rawGen.GenerateOpenAIRaw(ctx, contentType, transformedBody, endpoint)
		latencyMs := time.Since(start).Milliseconds()
		totalLatency += latencyMs

		// Client cancellation must not count as a channel failure.
		if ctx.Err() != nil {
			chLog.Info("request cancelled by client, not recording failure")
			return
		}

		if err != nil {
			h.router.RecordResult(ch.Name(), false, latencyMs)
			// 中间渠道失败 - 记录日志但不更新统计
			errMsg := ""
			if err != nil {
				errMsg = err.Error()
			}
			h.logUsage(ctx, ch.Name(), model, false, 0, errMsg, latencyMs, requestIP, false)
			chLog.Warn("channel transport error, trying next", "err", err)
			lastErr = err
			continue
		}

		// Upstream returned — decide whether to fall back.
		if shouldFallbackOnStatus(resp.Status) {
			h.router.RecordResult(ch.Name(), false, latencyMs)
			// 中间渠道失败（retriable status）- 记录日志但不更新统计
			h.logUsage(ctx, ch.Name(), model, false, resp.Status, http.StatusText(resp.Status), latencyMs, requestIP, false)
			chLog.Warn("channel returned retriable status, trying next", "status", resp.Status)
			lastErr = statusError(resp.Status)
			continue
		}

		// Propagate verbatim.
		h.writeRawResponse(w, resp)
		// 2xx → success; 4xx client-fault → also record as "success" for
		// health purposes (the channel is working; the client sent a bad request).
		h.router.RecordResult(ch.Name(), true, latencyMs)
		// 最终成功/响应 - 记录使用日志并更新统计
		isSuccess := resp.Status >= 200 && resp.Status < 300
		h.logUsage(ctx, ch.Name(), model, isSuccess, resp.Status, "", totalLatency, requestIP, true)
		return
	}

	// 所有渠道都失败 - 记录使用日志并更新统计
	slog.Error("all OpenAI image channels failed", "endpoint", endpoint, "lastErr", lastErr)
	errMsg := "all OpenAI image channels failed"
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	h.logUsage(ctx, lastChannel, model, false, http.StatusBadGateway, errMsg, totalLatency, requestIP, true)
	h.writeOpenAIError(w, "All OpenAI image channels failed", "api_error", http.StatusBadGateway)
}

// dispatchStream is the streaming variant. A channel either fails before any
// bytes reach the client (handler may fall back) or commits headers and
// streams to completion (handler returns without fallback).
func (h *OpenAIHandler) dispatchStream(
	w http.ResponseWriter, r *http.Request,
	candidates []core.Channel, contentType string, bodyBytes []byte, endpoint string,
) {
	ctx := r.Context()
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeOpenAIError(w, "Streaming not supported by server", "api_error", http.StatusInternalServerError)
		return
	}
	rw := &responseWriter{ResponseWriter: w, flusher: flusher}

	var lastErr error
	var lastChannel string
	var totalLatency int64
	requestIP := extractClientIP(r)
	model := extractModelFromBody(bodyBytes)
	
	for _, ch := range candidates {
		chLog := slog.With("channel", ch.Name(), "endpoint", endpoint)
		lastChannel = ch.Name()

		rawStream, ok := ch.(core.OpenAIRawStreamGenerator)
		if !ok {
			chLog.Warn("candidate does not implement OpenAIRawStreamGenerator; skipping")
			continue
		}

		start := time.Now()
		err := rawStream.StreamOpenAIRaw(ctx, contentType, bodyBytes, endpoint, rw)
		latencyMs := time.Since(start).Milliseconds()
		totalLatency += latencyMs

		if ctx.Err() != nil {
			chLog.Info("request cancelled by client, not recording failure")
			return
		}

		if err == nil {
			h.router.RecordResult(ch.Name(), true, latencyMs)
			// 最终成功 - 记录使用日志并更新统计
			h.logUsage(ctx, ch.Name(), model, true, http.StatusOK, "", totalLatency, requestIP, true)
			return
		}

		// Pre-commit upstream non-2xx. Decide: propagate directly (4xx client
		// fault) or fall back (5xx/429/408/401).
		var statusErr *openai_original.UpstreamStatusError
		if errors.As(err, &statusErr) {
			if shouldFallbackOnStatus(statusErr.Status) {
				h.router.RecordResult(ch.Name(), false, latencyMs)
				// 中间渠道失败（retriable status）- 记录日志但不更新统计
				h.logUsage(ctx, ch.Name(), model, false, statusErr.Status, http.StatusText(statusErr.Status), latencyMs, requestIP, false)
				chLog.Warn("upstream retriable status, trying next", "status", statusErr.Status)
				lastErr = err
				continue
			}
			// Client-fault status — propagate verbatim. Channel is healthy.
			h.writeRawResponse(w, &core.OpenAIRawResponse{
				Status: statusErr.Status, Headers: statusErr.Headers, Body: statusErr.Body,
			})
			h.router.RecordResult(ch.Name(), true, latencyMs)
			// 最终响应（可能是 4xx 客户端错误）- 记录使用日志并更新统计
			isSuccess := statusErr.Status >= 200 && statusErr.Status < 300
			h.logUsage(ctx, ch.Name(), model, isSuccess, statusErr.Status, "", totalLatency, requestIP, true)
			return
		}

		// Transport-level failure.
		h.router.RecordResult(ch.Name(), false, latencyMs)
		// 中间渠道失败（transport error）- 记录日志但不更新统计
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		h.logUsage(ctx, ch.Name(), model, false, 0, errMsg, latencyMs, requestIP, false)
		chLog.Warn("channel stream failed, trying next", "err", err)
		lastErr = err
	}

	// 所有渠道都失败 - 记录使用日志并更新统计
	slog.Error("all OpenAI image streaming channels failed", "endpoint", endpoint, "lastErr", lastErr)
	errMsg := "all OpenAI image channels failed"
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	h.logUsage(ctx, lastChannel, model, false, http.StatusBadGateway, errMsg, totalLatency, requestIP, true)
	h.writeOpenAIError(w, "All OpenAI image channels failed", "api_error", http.StatusBadGateway)
}

// writeRawResponse writes status, whitelisted headers, and body to the client.
func (h *OpenAIHandler) writeRawResponse(w http.ResponseWriter, resp *core.OpenAIRawResponse) {
	for k, vs := range resp.Headers {
		if _, ok := passThroughHeaders[k]; !ok {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.Status)
	_, _ = w.Write(resp.Body)
}

// handleListModels lists configured OpenAI image channels as "models".
// NOTE: Clients that use the returned "id" values directly in subsequent
// requests will NOT get a valid OpenAI model name — this endpoint exists
// for inventory/observability, not SDK model discovery.
func (h *OpenAIHandler) handleListModels(w http.ResponseWriter, r *http.Request) {
	filter := &core.ChannelTypeFilter{Include: []string{"openai_original", "openai_callback"}}
	candidates, _ := h.router.RouteWithTypeFilter(r.Context(), filter)

	models := make([]map[string]any, 0, len(candidates))
	for _, ch := range candidates {
		models = append(models, map[string]any{
			"id":       ch.Name(),
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "openai",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   models,
	})
}

func (h *OpenAIHandler) writeOpenAIError(w http.ResponseWriter, message, errType string, statusCode int) {
	errResp := model.NewOpenAIError(message, errType)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(errResp)
}

// clientContentType returns the request's Content-Type header, defaulting to
// application/json when the client omitted it.
func clientContentType(r *http.Request) string {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return "application/json"
	}
	return ct
}

// shouldFallbackOnStatus decides whether a non-2xx upstream status warrants
// trying the next channel. 5xx/429/408/401 are channel-level issues; other
// 4xx are client-fault and should be propagated as-is.
func shouldFallbackOnStatus(status int) bool {
	if status >= 500 {
		return true
	}
	switch status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusUnauthorized:
		return true
	}
	return false
}

type statusError int

func (s statusError) Error() string {
	return http.StatusText(int(s))
}

// passThroughHeaders is the whitelist of response headers forwarded to the
// client from a non-streaming upstream response.
var passThroughHeaders = map[string]struct{}{
	"Content-Type":                   {},
	"Cache-Control":                  {},
	"X-Request-Id":                   {},
	"Openai-Version":                 {},
	"Openai-Processing-Ms":           {},
	"X-Ratelimit-Limit-Requests":     {},
	"X-Ratelimit-Limit-Tokens":       {},
	"X-Ratelimit-Remaining-Requests": {},
	"X-Ratelimit-Remaining-Tokens":   {},
	"X-Ratelimit-Reset-Requests":     {},
	"X-Ratelimit-Reset-Tokens":       {},
	"Retry-After":                    {},
}

// responseWriter adapts http.ResponseWriter + http.Flusher into core.ResponseWriter.
type responseWriter struct {
	http.ResponseWriter
	flusher http.Flusher
}

func (rw *responseWriter) Flush() { rw.flusher.Flush() }

// extractClientIP 从请求中提取客户端 IP
func extractClientIP(r *http.Request) string {
	// 检查 X-Forwarded-For 头
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// 取第一个 IP
		if idx := strings.Index(xff, ","); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	
	// 检查 X-Real-IP 头
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	
	// 使用 RemoteAddr
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx > 0 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}

// extractModelFromBody 从请求体中提取模型名称
func extractModelFromBody(bodyBytes []byte) string {
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return "unknown"
	}
	if model, ok := body["model"].(string); ok {
		return model
	}
	return "unknown"
}

// logUsage 记录 API Key 使用情况
// updateStats 为 true 时更新 TotalSuccess/TotalFail，false 时只记录日志不更新统计
func (h *OpenAIHandler) logUsage(ctx context.Context, channelName, model string, success bool, statusCode int, errorMsg string, latencyMs int64, requestIP string, updateStats bool) {
	if h.usageLogger == nil {
		return
	}
	
	// 从 context 获取 API Key ID
	apiKeyID, ok := middleware.GetAPIKeyID(ctx)
	if !ok {
		// 可能是使用 JWT 认证的请求，不记录
		return
	}
	
	var errMsg *string
	if errorMsg != "" {
		errMsg = &errorMsg
	}
	
	var status *int
	if statusCode > 0 {
		status = &statusCode
	}
	
	var latency *int
	if latencyMs > 0 {
		latencyInt := int(latencyMs)
		latency = &latencyInt
	}
	
	var ip *string
	if requestIP != "" {
		ip = &requestIP
	}
	
	entry := database.LogEntry{
		APIKeyID:     apiKeyID,
		ChannelName:  channelName,
		Model:        model,
		Success:      success,
		StatusCode:   status,
		ErrorMessage: errMsg,
		LatencyMs:    latency,
		RequestIP:    ip,
		UpdateStats:  updateStats, // 是否更新统计
	}
	
	h.usageLogger.Log(entry)
}
