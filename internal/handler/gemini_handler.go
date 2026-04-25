package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"goloop/internal/core"
	"goloop/internal/database"
	"goloop/internal/kieai"
	"goloop/internal/middleware"
	"goloop/internal/model"
	"goloop/internal/storage"
	"goloop/internal/transformer"
)

// GeminiHandler handles POST /v1beta/models/{model}:generateContent
type GeminiHandler struct {
	router              *core.Router
	registry            *core.PluginRegistry
	issuer              *core.JWTIssuer
	storage             *storage.Store
	taskManager         *kieai.TaskManager
	reqTransformer      *transformer.RequestTransformer
	respTransformer     *transformer.ResponseTransformer
	maxRequestBodyBytes int64
	usageLogger         *core.UsageLogger
}

func NewGeminiHandler(
	router *core.Router,
	registry *core.PluginRegistry,
	issuer *core.JWTIssuer,
	storage *storage.Store,
	taskManager *kieai.TaskManager,
	reqTransformer *transformer.RequestTransformer,
	respTransformer *transformer.ResponseTransformer,
	maxRequestBodyBytes int64,
	usageLogger *core.UsageLogger,
) *GeminiHandler {
	if maxRequestBodyBytes <= 0 {
		maxRequestBodyBytes = 50 * 1024 * 1024 // 50MB default
	}
	return &GeminiHandler{
		router:              router,
		registry:            registry,
		issuer:              issuer,
		storage:             storage,
		taskManager:         taskManager,
		reqTransformer:      reqTransformer,
		respTransformer:     respTransformer,
		maxRequestBodyBytes: maxRequestBodyBytes,
		usageLogger:         usageLogger,
	}
}

// RegisterRoutes mounts the handler onto the provided mux.
// Route: POST /v1beta/models/{model}:generateContent (API Key protected)
// Route: GET /v1beta/models (public)
// Route: GET /health (public)
// Note: API Key middleware should be applied at the mux level in main.go
func (h *GeminiHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1beta/models/", h.handleGenerateContent)
	mux.HandleFunc("GET /v1beta/models", h.handleListModels)
	mux.HandleFunc("GET /health", h.handleHealth)
}

func (h *GeminiHandler) handleListModels(w http.ResponseWriter, r *http.Request) {
	// Return a static list of supported models
	// In the future, this could be dynamic based on available channels
	models := []map[string]any{
		{
			"name":        "gemini-3.1-flash-image-preview",
			"description": "Fast image generation model",
		},
		{
			"name":        "gemini-3-pro-image-preview",
			"description": "High quality image generation model",
		},
		{
			"name":        "gemini-2.5-flash-image",
			"description": "Latest flash image generation model",
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"models": models})
}

func (h *GeminiHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// isStreamingRequest detects whether the client expects an SSE streaming response.
func isStreamingRequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/event-stream") ||
		strings.Contains(accept, "multipart/x-mixed-replace")
}

// httpResponseWriter adapts http.ResponseWriter + http.Flusher into
// the core.ResponseWriter interface expected by streaming channel methods.
type httpResponseWriter struct {
	http.ResponseWriter
	flusher http.Flusher
}

func (rw *httpResponseWriter) Flush() { rw.flusher.Flush() }

// handleGenerateContentStreaming handles SSE streaming responses.
//
// Priority order for each channel candidate:
//  1. RawStreamGenerator  — zero-conversion pipe (gemini native)
//  2. StreamGenerator     — format-converted stream (openai -> google SSE)
//  3. SubmitTask+Poll     — async KIE task path (legacy)
//
// For path 1 and 2 we can fall back if the upstream request fails before
// headers are written. Once headers are committed (path 3), errors are
// reported via SSE error events instead.
func (h *GeminiHandler) handleGenerateContentStreaming(w http.ResponseWriter, r *http.Request, googleModel string, googleReq *model.GoogleRequest, bodyBytes []byte, requestID string) {
	ctx := r.Context()
	log := slog.With("requestId", requestID, "googleModel", googleModel)
	requestIP := extractClientIP(r)
	var totalLatency int64
	var lastChannel string

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 500, Message: "streaming not supported", Status: "INTERNAL"},
		}, http.StatusInternalServerError)
		return
	}

	// Only include Gemini channels for Gemini routes
	filter := &core.ChannelTypeFilter{
		Include: []string{
			"gemini_callback",
			"gemini_openai",
			"gemini_original",
		},
	}
	channels, err := h.router.RouteWithTypeFilter(ctx, filter)
	if err != nil {
		log.Error("router error", "err", err)
		h.writeSSEError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 503, Message: "no healthy channels", Status: "UNAVAILABLE"},
		}, http.StatusServiceUnavailable)
		return
	}

	rw := &httpResponseWriter{ResponseWriter: w, flusher: flusher}

	for _, candidate := range channels {
		chLog := log.With("channel", candidate.Name())
		start := time.Now()
		lastChannel = candidate.Name()

		// --- Path 1: RawStreamGenerator (gemini native pass-through) ---
		if rawStream, ok := candidate.(core.RawStreamGenerator); ok {
			chLog.Info("using raw stream path")
			if err := rawStream.StreamRaw(ctx, bodyBytes, googleModel, rw); err != nil {
				if ctx.Err() != nil {
					chLog.Info("request cancelled, not recording failure")
					return
				}
				latency := time.Since(start).Milliseconds()
				totalLatency += latency
				h.router.RecordResult(candidate.Name(), false, latency)
				// 中间渠道失败 - 记录日志但不更新统计
				errMsg := ""
				if err != nil {
					errMsg = err.Error()
				}
				h.logUsage(ctx, candidate.Name(), googleModel, false, 0, errMsg, latency, requestIP, false)
				chLog.Warn("raw stream failed, trying next channel", "err", err)
				continue
			}
			latency := time.Since(start).Milliseconds()
			totalLatency += latency
			h.router.RecordResult(candidate.Name(), true, latency)
			// 最终成功 - 记录使用日志并更新统计
			h.logUsage(ctx, candidate.Name(), googleModel, true, http.StatusOK, "", totalLatency, requestIP, true)
			return
		}

		// --- Path 2: StreamGenerator (format-converted stream, e.g. openai) ---
		if streamGen, ok := candidate.(core.StreamGenerator); ok {
			chLog.Info("using converted stream path")
			if err := streamGen.Stream(ctx, googleReq, googleModel, rw); err != nil {
				if ctx.Err() != nil {
					chLog.Info("request cancelled, not recording failure")
					return
				}
				latency := time.Since(start).Milliseconds()
				totalLatency += latency
				h.router.RecordResult(candidate.Name(), false, latency)
				// 中间渠道失败 - 记录日志但不更新统计
				errMsg := ""
				if err != nil {
					errMsg = err.Error()
				}
				h.logUsage(ctx, candidate.Name(), googleModel, false, 0, errMsg, latency, requestIP, false)
				chLog.Warn("stream failed, trying next channel", "err", err)
				continue
			}
			latency := time.Since(start).Milliseconds()
			totalLatency += latency
			h.router.RecordResult(candidate.Name(), true, latency)
			// 最终成功 - 记录使用日志并更新统计
			h.logUsage(ctx, candidate.Name(), googleModel, true, http.StatusOK, "", totalLatency, requestIP, true)
			return
		}

		// --- Path 3: legacy async SubmitTask + Poll (KIE) ---
		chLog.Info("using async task stream path")
		taskID, apiKey, submitErr := candidate.SubmitTask(ctx, googleReq, googleModel)
		if submitErr != nil {
			if ctx.Err() != nil {
				chLog.Info("request cancelled during submitTask, not recording failure", "err", submitErr)
				return
			}
			latency := time.Since(start).Milliseconds()
			totalLatency += latency
			h.router.RecordResult(candidate.Name(), false, 0)
			// 中间渠道失败 - 记录日志但不更新统计
			errMsg := ""
			if submitErr != nil {
				errMsg = submitErr.Error()
			}
			h.logUsage(ctx, candidate.Name(), googleModel, false, 0, errMsg, latency, requestIP, false)
			chLog.Warn("submitTask failed, trying next channel", "err", submitErr)
			continue
		}

		chLog.Info("task created, polling for result", "taskId", taskID)
		resultCh := h.taskManager.SubmitTaskStreaming(ctx, apiKey, taskID)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Request-Id", requestID)
		w.WriteHeader(http.StatusOK)
		h.writeSSEEvent(w, flusher, "event: connection\ndata: {\"status\":\"connected\"}\n\n")

		select {
		case result := <-resultCh:
			latency := time.Since(start).Milliseconds()
			totalLatency += latency
			
			if result.Error != nil {
				chLog.Error("task failed", "err", result.Error)
				var tErr *kieai.TaskFailedError
				var errMsg string
				if errors.As(result.Error, &tErr) {
					errMsg = tErr.Reason
					gErr, _ := transformer.ToGoogleError(500, tErr.Reason)
					h.writeSSEError(w, gErr, 500)
				} else {
					errMsg = result.Error.Error()
					gErr, _ := transformer.ToGoogleError(500, errMsg)
					h.writeSSEError(w, gErr, 500)
				}
				h.router.RecordResult(candidate.Name(), false, latency)
				// 最终失败 - 记录使用日志并更新统计
				h.logUsage(ctx, candidate.Name(), googleModel, false, 500, errMsg, totalLatency, requestIP, true)
				return
			}
			record := result.Record
			if record.ResultJSON() == nil || len(record.ResultJSON().ResultURLs) == 0 {
				chLog.Error("task succeeded but no result URLs")
				gErr, _ := transformer.ToGoogleError(500, "no result URLs")
				h.writeSSEError(w, gErr, 500)
				h.router.RecordResult(candidate.Name(), false, latency)
				// 最终失败 - 记录使用日志并更新统计
				h.logUsage(ctx, candidate.Name(), googleModel, false, 500, "no result URLs", totalLatency, requestIP, true)
				return
			}
			googleResp, err := h.respTransformer.ToGoogleStreamingResponse(ctx, record.ResultJSON().ResultURLs, requestID, isImageOnlyRequest(googleReq))
			if err != nil {
				chLog.Error("response transform failed", "err", err)
				gErr, _ := transformer.ToGoogleError(500, err.Error())
				h.writeSSEError(w, gErr, 500)
				h.router.RecordResult(candidate.Name(), false, latency)
				// 最终失败 - 记录使用日志并更新统计
				h.logUsage(ctx, candidate.Name(), googleModel, false, 500, err.Error(), totalLatency, requestIP, true)
				return
			}
			h.writeSSEData(w, flusher, googleResp)
			h.writeSSEEvent(w, flusher, "data: [DONE]\n\n")
			h.router.RecordResult(candidate.Name(), true, latency)
			// 最终成功 - 记录使用日志并更新统计
			h.logUsage(ctx, candidate.Name(), googleModel, true, http.StatusOK, "", totalLatency, requestIP, true)
		case <-ctx.Done():
			chLog.Info("request cancelled")
			h.writeSSEError(w, model.GoogleError{
				Error: model.GoogleErrorDetail{Code: 499, Message: "client closed request", Status: "CANCELLED"},
			}, 499)
		}
		return
	}

	// 所有渠道都失败 - 最终失败，记录使用日志并更新统计
	log.Error("all channels failed for streaming request")
	h.logUsage(ctx, lastChannel, googleModel, false, http.StatusServiceUnavailable, "all channels failed", totalLatency, requestIP, true)
	h.writeSSEError(w, model.GoogleError{
		Error: model.GoogleErrorDetail{Code: 503, Message: "all channels failed", Status: "UNAVAILABLE"},
	}, http.StatusServiceUnavailable)
}

func (h *GeminiHandler) writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, data string) {
	w.Write([]byte(data))
	flusher.Flush()
}

func (h *GeminiHandler) writeSSEData(w http.ResponseWriter, flusher http.Flusher, resp *model.StreamingResponse) {
	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		slog.Error("marshal streaming response failed", "err", err)
		return
	}
	w.Write([]byte("data: "))
	w.Write(jsonBytes)
	w.Write([]byte("\n\n"))
	flusher.Flush()
}

func (h *GeminiHandler) writeSSEError(w http.ResponseWriter, e model.GoogleError, httpCode int) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(httpCode)
		return
	}
	w.WriteHeader(httpCode)
	w.Write([]byte("event: error\ndata: "))
	jsonBytes, _ := json.Marshal(e)
	w.Write(jsonBytes)
	w.Write([]byte("\n\n"))
	flusher.Flush()
}

func (h *GeminiHandler) handleGenerateContent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// API Key ID is injected by APIKeyMiddleware
	apiKeyID, ok := middleware.GetAPIKeyID(ctx)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}
	_ = apiKeyID // API Key ID will be used for usage logging

	suffix := strings.TrimPrefix(r.URL.Path, "/v1beta/models/")
	googleModel, action, found := strings.Cut(suffix, ":")
	if !found || action != "generateContent" || googleModel == "" {
		http.NotFound(w, r)
		return
	}

	requestID := generateRequestID()
	log := slog.With("requestId", requestID, "googleModel", googleModel)

	limited := io.LimitReader(r.Body, h.maxRequestBodyBytes+1)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		log.Error("read request body", "err", err)
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 400, Message: "failed to read request body", Status: "INVALID_ARGUMENT"},
		}, http.StatusBadRequest)
		return
	}
	if int64(len(bodyBytes)) > h.maxRequestBodyBytes {
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 400, Message: "request body too large", Status: "INVALID_ARGUMENT"},
		}, http.StatusBadRequest)
		return
	}

	var googleReq model.GoogleRequest
	if err := json.Unmarshal(bodyBytes, &googleReq); err != nil {
		log.Error("unmarshal request", "err", err)
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 400, Message: "invalid request body", Status: "INVALID_ARGUMENT"},
		}, http.StatusBadRequest)
		return
	}

	maskHeader := func(v string) string {
		if len(v) > 16 {
			return v[:8] + "..." + v[len(v)-4:]
		}
		return "***"
	}
	log.Info("request received",
		"method", r.Method,
		"path", r.URL.Path,
		"model", googleModel,
		"auth", maskHeader(r.Header.Get("Authorization")),
		"contentType", r.Header.Get("Content-Type"),
		"contentsCount", len(googleReq.Contents),
	)

	if isStreamingRequest(r) {
		h.handleGenerateContentStreaming(w, r, googleModel, &googleReq, bodyBytes, requestID)
		return
	}

	// Get ordered fallback list — honours JWT channel restriction if present.
	// Only include Gemini channels for Gemini routes.
	filter := &core.ChannelTypeFilter{
		Include: []string{
			"gemini_callback",
			"gemini_openai",
			"gemini_original",
		},
	}
	channels, err := h.router.RouteWithTypeFilter(ctx, filter)
	if err != nil {
		log.Error("router error", "err", err)
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 503, Message: "no healthy channels", Status: "UNAVAILABLE"},
		}, http.StatusServiceUnavailable)
		return
	}

	// Try each channel in priority order, falling back on failure.
	var lastErr error
	var lastChannel string
	var totalLatency int64
	requestIP := extractClientIP(r)
	
	for _, ch := range channels {
		chLog := log.With("channel", ch.Name())
		start := time.Now()
		lastChannel = ch.Name()

		// Fast path: channels implementing RawBodyGenerator bypass struct
		// conversion entirely and return raw bytes for direct pass-through.
		if rawGen, ok := ch.(core.RawBodyGenerator); ok {
			rawResp, err := rawGen.GenerateRaw(ctx, bodyBytes, googleModel)
			latency := time.Since(start).Milliseconds()
			totalLatency += latency
			
			if err == nil {
				h.router.RecordResult(ch.Name(), true, latency)
				// 最终成功 - 记录使用日志并更新统计
				h.logUsage(ctx, ch.Name(), googleModel, true, http.StatusOK, "", totalLatency, requestIP, true)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(rawResp)
				return
			}
			if ctx.Err() != nil {
				chLog.Info("request cancelled by client, not recording failure", "err", err)
				lastErr = err
				break
			}
			h.router.RecordResult(ch.Name(), false, latency)
			// 中间渠道失败 - 记录日志但不更新统计
			errMsg := ""
			if err != nil {
				errMsg = err.Error()
			}
			h.logUsage(ctx, ch.Name(), googleModel, false, 0, errMsg, latency, requestIP, false)
			chLog.Warn("channel failed, trying next", "err", err)
			lastErr = err
			continue
		}

		googleResp, err := h.tryChannel(ctx, ch, &googleReq, googleModel)
		latency := time.Since(start).Milliseconds()
		totalLatency += latency

		if err == nil {
			h.router.RecordResult(ch.Name(), true, latency)
			// 最终成功 - 记录使用日志并更新统计
			h.logUsage(ctx, ch.Name(), googleModel, true, http.StatusOK, "", totalLatency, requestIP, true)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(googleResp)
			return
		}

		if ctx.Err() != nil {
			chLog.Info("request cancelled by client, not recording failure", "err", err)
			lastErr = err
			break
		}

		h.router.RecordResult(ch.Name(), false, latency)
		// 中间渠道失败 - 记录日志但不更新统计
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		h.logUsage(ctx, ch.Name(), googleModel, false, 0, errMsg, latency, requestIP, false)
		chLog.Warn("channel failed, trying next", "err", err)
		lastErr = err
	}

	// All channels failed - 最终失败，记录使用日志并更新统计
	log.Error("all channels failed", "err", lastErr)
	var tErr *kieai.TaskFailedError
	if errors.As(lastErr, &tErr) {
		gErr, httpCode := transformer.ToGoogleError(500, tErr.Reason)
		h.logUsage(ctx, lastChannel, googleModel, false, httpCode, tErr.Reason, totalLatency, requestIP, true)
		writeGoogleError(w, gErr, httpCode)
	} else {
		errMsg := "all channels failed"
		if lastErr != nil {
			errMsg = lastErr.Error()
		}
		h.logUsage(ctx, lastChannel, googleModel, false, http.StatusServiceUnavailable, errMsg, totalLatency, requestIP, true)
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 503, Message: "all channels failed", Status: "UNAVAILABLE"},
		}, http.StatusServiceUnavailable)
	}
}

// tryChannel attempts to complete a request on a single channel.
// It first tries Generate (synchronous path); if the channel returns
// ErrNotSupported it falls back to SubmitTask + PollTask (async path).
// Channels implementing RawBodyGenerator are handled before calling this
// function and never reach tryChannel.
func (h *GeminiHandler) tryChannel(ctx context.Context, ch core.Channel, req *model.GoogleRequest, googleModel string) (*model.GoogleResponse, error) {
	// 1. Try synchronous Generate path.
	resp, err := ch.Generate(ctx, req, googleModel)
	if err == nil {
		return resp, nil
	}
	if !errors.Is(err, core.ErrNotSupported) {
		return nil, err
	}

	// 2. Fall back to async SubmitTask + PollTask.
	taskID, apiKey, err := ch.SubmitTask(ctx, req, googleModel)
	if err != nil {
		return nil, err
	}
	slog.Debug("task submitted", "channel", ch.Name(), "taskId", taskID)
	return ch.PollTask(ctx, apiKey, taskID)
}

// isImageOnlyRequest returns true when the request's responseModalities
// contains only "image" (no "text"), so callers can omit the descriptive text part.
func isImageOnlyRequest(req *model.GoogleRequest) bool {
	if req == nil || req.GenerationConfig == nil || len(req.GenerationConfig.ResponseModalities) == 0 {
		return false
	}
	for _, m := range req.GenerationConfig.ResponseModalities {
		if strings.EqualFold(m, "text") {
			return false
		}
	}
	return true
}

func writeGoogleError(w http.ResponseWriter, e model.GoogleError, httpCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	json.NewEncoder(w).Encode(e)
}

func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

// logUsage 记录 API Key 使用情况
// updateStats 为 true 时更新 TotalSuccess/TotalFail，false 时只记录日志不更新统计
func (h *GeminiHandler) logUsage(ctx context.Context, channelName, model string, success bool, statusCode int, errorMsg string, latencyMs int64, requestIP string, updateStats bool) {
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
