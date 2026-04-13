package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"goloop/internal/core"
	"goloop/internal/kieai"
	"goloop/internal/model"
	"goloop/internal/storage"
	"goloop/internal/transformer"
)

const maxRequestBodyBytes = 10 * 1024 * 1024 // 10MB

// GeminiHandler handles POST /v1beta/models/{model}:generateContent
type GeminiHandler struct {
	router          *core.Router
	registry        *core.PluginRegistry
	issuer          *core.JWTIssuer
	storage         *storage.Store
	taskManager     *kieai.TaskManager
	reqTransformer  *transformer.RequestTransformer
	respTransformer *transformer.ResponseTransformer
}

func NewGeminiHandler(
	router *core.Router,
	registry *core.PluginRegistry,
	issuer *core.JWTIssuer,
	storage *storage.Store,
	taskManager *kieai.TaskManager,
	reqTransformer *transformer.RequestTransformer,
	respTransformer *transformer.ResponseTransformer,
) *GeminiHandler {
	return &GeminiHandler{
		router:          router,
		registry:        registry,
		issuer:          issuer,
		storage:         storage,
		taskManager:     taskManager,
		reqTransformer:  reqTransformer,
		respTransformer: respTransformer,
	}
}

// RegisterRoutes mounts the handler onto the provided mux.
// Route: POST /v1beta/models/{model}:generateContent (JWT-protected)
// Route: GET /v1beta/models (public)
// Route: GET /health (public)
func (h *GeminiHandler) RegisterRoutes(mux *http.ServeMux) {
	protected := core.NewJWTMiddleware(h.issuer, h.handleProtected)
	mux.Handle("POST /v1beta/models/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protected.ServeHTTP(w, r)
	}))
	mux.HandleFunc("GET /v1beta/models", h.handleListModels)
	mux.HandleFunc("GET /health", h.handleHealth)
}

func (h *GeminiHandler) handleListModels(w http.ResponseWriter, r *http.Request) {
	models := h.reqTransformer.ListModels()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"models": models})
}

func (h *GeminiHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// handleProtected is called by JWTMiddleware after JWT validation succeeds.
// JWT only carries channel restriction (optional). Account selection is done
// internally by the channel's pool.
func (h *GeminiHandler) handleProtected(ctx context.Context, claims *core.JWTClaims, w http.ResponseWriter, r *http.Request) {
	if claims.Channel != "" {
		ctx = core.WithChannelRestriction(ctx, claims.Channel)
		r = r.WithContext(ctx)
	}
	h.handleGenerateContent(w, r)
}

// isStreamingRequest detects whether the client expects an SSE streaming response.
func isStreamingRequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/event-stream") ||
		strings.Contains(accept, "multipart/x-mixed-replace")
}

// handleGenerateContentStreaming handles SSE streaming responses.
// It tries each channel in priority order, falling back on failure.
// Note: once SSE headers are written (after the first successful SubmitTask),
// we cannot fall back further — errors are reported via SSE error events.
func (h *GeminiHandler) handleGenerateContentStreaming(w http.ResponseWriter, r *http.Request, googleModel string, googleReq *model.GoogleRequest, requestID string) {
	ctx := r.Context()
	log := slog.With("requestId", requestID, "googleModel", googleModel)

	channels, err := h.router.RouteWithFallback(ctx)
	if err != nil {
		log.Error("router error", "err", err)
		h.writeSSEError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 503, Message: "no healthy channels", Status: "UNAVAILABLE"},
		}, http.StatusServiceUnavailable)
		return
	}

	// Try each channel in priority order. For streaming we must commit to a
	// channel before writing headers, so we attempt SubmitTask first and only
	// fall back if it fails before headers are sent.
	var (
		ch      core.Channel
		taskID  string
		apiKey  string
		submitErr error
	)
	for _, candidate := range channels {
		taskID, apiKey, submitErr = candidate.SubmitTask(ctx, googleReq, googleModel)
		if submitErr == nil {
			ch = candidate
			break
		}
		if ctx.Err() != nil {
			log.Info("request cancelled by client during submitTask, not recording failure", "channel", candidate.Name(), "err", submitErr)
			break
		}
		log.Warn("channel submitTask failed, trying next", "channel", candidate.Name(), "err", submitErr)
		h.router.RecordResult(candidate.Name(), false, 0)
	}

	if ch == nil {
		log.Error("all channels failed at submitTask", "err", submitErr)
		h.writeSSEError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 503, Message: "all channels failed: " + submitErr.Error(), Status: "UNAVAILABLE"},
		}, http.StatusServiceUnavailable)
		return
	}

	log = log.With("channel", ch.Name(), "taskId", taskID)
	log.Info("task created, polling for result")

	resultCh := h.taskManager.SubmitTaskStreaming(ctx, apiKey, taskID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Request-Id", requestID)
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeSSEError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 500, Message: "streaming not supported", Status: "INTERNAL"},
		}, http.StatusInternalServerError)
		return
	}

	h.writeSSEEvent(w, flusher, "event: connection\ndata: {\"status\":\"connected\"}\n\n")

	select {
	case result := <-resultCh:
		start := time.Now()
		if result.Error != nil {
			log.Error("task failed", "err", result.Error)
			var tErr *kieai.TaskFailedError
			if errors.As(result.Error, &tErr) {
				gErr, _ := transformer.ToGoogleError(500, tErr.Reason)
				h.writeSSEError(w, gErr, 500)
			} else {
				gErr, _ := transformer.ToGoogleError(500, result.Error.Error())
				h.writeSSEError(w, gErr, 500)
			}
			h.router.RecordResult(ch.Name(), false, time.Since(start).Milliseconds())
			return
		}

		record := result.Record
		if record.ResultJSON() == nil || len(record.ResultJSON().ResultURLs) == 0 {
			log.Error("task succeeded but no result URLs")
			gErr, _ := transformer.ToGoogleError(500, "no result URLs")
			h.writeSSEError(w, gErr, 500)
			h.router.RecordResult(ch.Name(), false, time.Since(start).Milliseconds())
			return
		}

		googleResp, err := h.respTransformer.ToGoogleStreamingResponse(ctx, record.ResultJSON().ResultURLs, requestID)
		if err != nil {
			log.Error("response transform failed", "err", err)
			gErr, _ := transformer.ToGoogleError(500, err.Error())
			h.writeSSEError(w, gErr, 500)
			h.router.RecordResult(ch.Name(), false, time.Since(start).Milliseconds())
			return
		}

		h.writeSSEData(w, flusher, googleResp)
		h.writeSSEEvent(w, flusher, "data: [DONE]\n\n")
		h.router.RecordResult(ch.Name(), true, time.Since(start).Milliseconds())

	case <-ctx.Done():
		log.Info("request cancelled")
		h.writeSSEError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 499, Message: "client closed request", Status: "CANCELLED"},
		}, 499)
	}
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

	suffix := strings.TrimPrefix(r.URL.Path, "/v1beta/models/")
	googleModel, action, found := strings.Cut(suffix, ":")
	if !found || action != "generateContent" || googleModel == "" {
		http.NotFound(w, r)
		return
	}

	requestID := generateRequestID()
	log := slog.With("requestId", requestID, "googleModel", googleModel)

	limited := io.LimitReader(r.Body, maxRequestBodyBytes+1)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		log.Error("read request body", "err", err)
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 400, Message: "failed to read request body", Status: "INVALID_ARGUMENT"},
		}, http.StatusBadRequest)
		return
	}
	if len(bodyBytes) > maxRequestBodyBytes {
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 400, Message: "request body too large", Status: "INVALID_ARGUMENT"},
		}, http.StatusBadRequest)
		return
	}

	var googleReq model.GoogleRequest
	if err := json.Unmarshal(bodyBytes, &googleReq); err != nil {
		log.Error("unmarshal request", "err", err)
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 400, Message: "invalid JSON: " + err.Error(), Status: "INVALID_ARGUMENT"},
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
		"contents", fmt.Sprintf("%v", googleReq.Contents),
	)

	if isStreamingRequest(r) {
		h.handleGenerateContentStreaming(w, r, googleModel, &googleReq, requestID)
		return
	}

	// Get ordered fallback list — honours JWT channel restriction if present.
	channels, err := h.router.RouteWithFallback(ctx)
	if err != nil {
		log.Error("router error", "err", err)
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 503, Message: "no healthy channels", Status: "UNAVAILABLE"},
		}, http.StatusServiceUnavailable)
		return
	}

	// Try each channel in priority order, falling back on failure.
	var lastErr error
	for _, ch := range channels {
		chLog := log.With("channel", ch.Name())
		start := time.Now()

		googleResp, err := h.tryChannel(ctx, ch, &googleReq, googleModel)
		latency := time.Since(start).Milliseconds()

		if err == nil {
			h.router.RecordResult(ch.Name(), true, latency)
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
		chLog.Warn("channel failed, trying next", "err", err)
		lastErr = err
	}

	// All channels failed.
	log.Error("all channels failed", "err", lastErr)
	var tErr *kieai.TaskFailedError
	if errors.As(lastErr, &tErr) {
		gErr, httpCode := transformer.ToGoogleError(500, tErr.Reason)
		writeGoogleError(w, gErr, httpCode)
	} else {
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 503, Message: "all channels failed: " + lastErr.Error(), Status: "UNAVAILABLE"},
		}, http.StatusServiceUnavailable)
	}
}

// tryChannel attempts to complete a request on a single channel.
// It first tries Generate (synchronous path); if the channel returns
// ErrNotSupported it falls back to SubmitTask + PollTask (async path).
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
