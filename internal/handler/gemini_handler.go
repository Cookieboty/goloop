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
// It injects the JWT api_key into the request header and the channel restriction
// into the context, then delegates to handleGenerateContent.
func (h *GeminiHandler) handleProtected(ctx context.Context, claims *core.JWTClaims, w http.ResponseWriter, r *http.Request) {
	// Use the API key embedded in the JWT if present.
	// This avoids clients having to pass x-goog-api-key separately.
	if claims.APIKey != "" {
		r = r.Clone(ctx)
		r.Header.Set("x-goog-api-key", claims.APIKey)
	}

	// If the JWT specifies a channel restriction, inject it into context
	// so the router honours it.
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
func (h *GeminiHandler) handleGenerateContentStreaming(w http.ResponseWriter, r *http.Request, googleModel, apiKey string, googleReq *model.GoogleRequest, requestID string) {
	ctx := r.Context()
	log := slog.With("requestId", requestID, "googleModel", googleModel)

	ch, err := h.router.RouteForModel(ctx, googleModel)
	if err != nil {
		log.Error("router error", "err", err)
		h.writeSSEError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 503, Message: "no healthy channels", Status: "UNAVAILABLE"},
		}, http.StatusServiceUnavailable)
		return
	}
	log = log.With("channel", ch.Name())

	taskID, err := ch.SubmitTask(ctx, apiKey, googleReq, googleModel)
	if err != nil {
		log.Error("submitTask failed", "err", err)
		h.writeSSEError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 500, Message: err.Error(), Status: "INTERNAL"},
		}, http.StatusInternalServerError)
		return
	}
	log = log.With("taskId", taskID)
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

	// API key resolution order:
	// 1. x-goog-api-key header (set by handleProtected from JWT claims.APIKey)
	// 2. x-goog-api-key header passed directly by the client
	// The JWT itself is NOT used as the upstream API key.
	apiKey := r.Header.Get("x-goog-api-key")
	if apiKey == "" {
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 401, Message: "API key not provided", Status: "UNAUTHENTICATED"},
		}, http.StatusUnauthorized)
		return
	}

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

	if isStreamingRequest(r) {
		h.handleGenerateContentStreaming(w, r, googleModel, apiKey, &googleReq, requestID)
		return
	}

	// Route with context — honours JWT channel restriction if present.
	ch, err := h.router.RouteForModel(ctx, googleModel)
	if err != nil {
		log.Error("router error", "err", err)
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 503, Message: "no healthy channels", Status: "UNAVAILABLE"},
		}, http.StatusServiceUnavailable)
		return
	}
	log = log.With("channel", ch.Name())

	taskID, err := ch.SubmitTask(ctx, apiKey, &googleReq, googleModel)
	if err != nil {
		log.Error("submitTask failed", "err", err)
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 500, Message: err.Error(), Status: "INTERNAL"},
		}, http.StatusInternalServerError)
		return
	}
	log = log.With("taskId", taskID)
	log.Info("task created, polling for result")

	start := time.Now()

	googleResp, err := ch.PollTask(ctx, apiKey, taskID)
	if err != nil {
		log.Error("poll failed", "err", err)
		var tErr *kieai.TaskFailedError
		if errors.As(err, &tErr) {
			gErr, httpCode := transformer.ToGoogleError(500, tErr.Reason)
			writeGoogleError(w, gErr, httpCode)
		} else {
			gErr, httpCode := transformer.ToGoogleError(500, err.Error())
			writeGoogleError(w, gErr, httpCode)
		}
		h.router.RecordResult(ch.Name(), false, time.Since(start).Milliseconds())
		return
	}

	h.router.RecordResult(ch.Name(), true, time.Since(start).Milliseconds())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(googleResp)
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
