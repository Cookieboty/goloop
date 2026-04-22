package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"goloop/internal/channels/gptimage"
	"goloop/internal/core"
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
	maxRequestBodyBytes int64
}

func NewOpenAIHandler(
	router *core.Router,
	registry *core.PluginRegistry,
	issuer *core.JWTIssuer,
	maxRequestBodyBytes int64,
) *OpenAIHandler {
	if maxRequestBodyBytes <= 0 {
		maxRequestBodyBytes = 50 * 1024 * 1024
	}
	return &OpenAIHandler{
		router:              router,
		registry:            registry,
		issuer:              issuer,
		maxRequestBodyBytes: maxRequestBodyBytes,
	}
}

// RegisterRoutes mounts the handler onto the provided mux.
func (h *OpenAIHandler) RegisterRoutes(mux *http.ServeMux) {
	chatProtected := core.NewJWTMiddleware(h.issuer, h.handleChatCompletionsProtected)
	imagesGenProtected := core.NewJWTMiddleware(h.issuer, h.handleImagesGenerationsProtected)
	imagesEditProtected := core.NewJWTMiddleware(h.issuer, h.handleImagesEditsProtected)

	mux.Handle("POST /v1/chat/completions", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatProtected.ServeHTTP(w, r)
	}))
	mux.Handle("POST /v1/images/generations", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		imagesGenProtected.ServeHTTP(w, r)
	}))
	mux.Handle("POST /v1/images/edits", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		imagesEditProtected.ServeHTTP(w, r)
	}))

	mux.HandleFunc("GET /v1/models", h.handleListModels)
}

func (h *OpenAIHandler) handleChatCompletionsProtected(ctx context.Context, claims *core.JWTClaims, w http.ResponseWriter, r *http.Request) {
	if claims.Channel != "" {
		ctx = core.WithChannelRestriction(ctx, claims.Channel)
		r = r.WithContext(ctx)
	}
	h.handleChatCompletions(w, r)
}

func (h *OpenAIHandler) handleImagesGenerationsProtected(ctx context.Context, claims *core.JWTClaims, w http.ResponseWriter, r *http.Request) {
	if claims.Channel != "" {
		ctx = core.WithChannelRestriction(ctx, claims.Channel)
		r = r.WithContext(ctx)
	}
	h.handleImagesGenerations(w, r)
}

func (h *OpenAIHandler) handleImagesEditsProtected(ctx context.Context, claims *core.JWTClaims, w http.ResponseWriter, r *http.Request) {
	if claims.Channel != "" {
		ctx = core.WithChannelRestriction(ctx, claims.Channel)
		r = r.WithContext(ctx)
	}
	h.handleImagesEdits(w, r)
}

// handleChatCompletions handles POST /v1/chat/completions (streaming or not).
func (h *OpenAIHandler) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
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

// selectCandidates returns gpt-image channel candidates (or writes an error response).
func (h *OpenAIHandler) selectCandidates(w http.ResponseWriter, r *http.Request) ([]core.Channel, bool) {
	filter := &core.ChannelTypeFilter{Include: []string{"gpt-image"}}
	candidates, err := h.router.RouteWithTypeFilter(r.Context(), filter)
	if err != nil {
		slog.Error("no gpt-image channels available", "error", err)
		h.writeOpenAIError(w, "No gpt-image channels available", "api_error", http.StatusServiceUnavailable)
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

	for _, ch := range candidates {
		chLog := slog.With("channel", ch.Name(), "endpoint", endpoint)

		rawGen, ok := ch.(core.OpenAIRawGenerator)
		if !ok {
			chLog.Warn("candidate does not implement OpenAIRawGenerator; skipping")
			continue
		}

		start := time.Now()
		resp, err := rawGen.GenerateOpenAIRaw(ctx, contentType, bodyBytes, endpoint)
		latencyMs := time.Since(start).Milliseconds()

		// Client cancellation must not count as a channel failure.
		if ctx.Err() != nil {
			chLog.Info("request cancelled by client, not recording failure")
			return
		}

		if err != nil {
			h.router.RecordResult(ch.Name(), false, latencyMs)
			chLog.Warn("channel transport error, trying next", "err", err)
			lastErr = err
			continue
		}

		// Upstream returned — decide whether to fall back.
		if shouldFallbackOnStatus(resp.Status) {
			h.router.RecordResult(ch.Name(), false, latencyMs)
			chLog.Warn("channel returned retriable status, trying next", "status", resp.Status)
			lastErr = statusError(resp.Status)
			continue
		}

		// Propagate verbatim.
		h.writeRawResponse(w, resp)
		// 2xx → success; 4xx client-fault → also record as "success" for
		// health purposes (the channel is working; the client sent a bad request).
		h.router.RecordResult(ch.Name(), true, latencyMs)
		return
	}

	slog.Error("all gpt-image channels failed", "endpoint", endpoint, "lastErr", lastErr)
	h.writeOpenAIError(w, "All gpt-image channels failed", "api_error", http.StatusBadGateway)
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
	for _, ch := range candidates {
		chLog := slog.With("channel", ch.Name(), "endpoint", endpoint)

		rawStream, ok := ch.(core.OpenAIRawStreamGenerator)
		if !ok {
			chLog.Warn("candidate does not implement OpenAIRawStreamGenerator; skipping")
			continue
		}

		start := time.Now()
		err := rawStream.StreamOpenAIRaw(ctx, contentType, bodyBytes, endpoint, rw)
		latencyMs := time.Since(start).Milliseconds()

		if ctx.Err() != nil {
			chLog.Info("request cancelled by client, not recording failure")
			return
		}

		if err == nil {
			h.router.RecordResult(ch.Name(), true, latencyMs)
			return
		}

		// Pre-commit upstream non-2xx. Decide: propagate directly (4xx client
		// fault) or fall back (5xx/429/408/401).
		var statusErr *gptimage.UpstreamStatusError
		if errors.As(err, &statusErr) {
			if shouldFallbackOnStatus(statusErr.Status) {
				h.router.RecordResult(ch.Name(), false, latencyMs)
				chLog.Warn("upstream retriable status, trying next", "status", statusErr.Status)
				lastErr = err
				continue
			}
			// Client-fault status — propagate verbatim. Channel is healthy.
			h.writeRawResponse(w, &core.OpenAIRawResponse{
				Status: statusErr.Status, Headers: statusErr.Headers, Body: statusErr.Body,
			})
			h.router.RecordResult(ch.Name(), true, latencyMs)
			return
		}

		// Transport-level failure.
		h.router.RecordResult(ch.Name(), false, latencyMs)
		chLog.Warn("channel stream failed, trying next", "err", err)
		lastErr = err
	}

	slog.Error("all gpt-image streaming channels failed", "endpoint", endpoint, "lastErr", lastErr)
	h.writeOpenAIError(w, "All gpt-image channels failed", "api_error", http.StatusBadGateway)
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

// handleListModels lists configured gpt-image channels as "models".
// NOTE: Clients that use the returned "id" values directly in subsequent
// requests will NOT get a valid OpenAI model name — this endpoint exists
// for inventory/observability, not SDK model discovery.
func (h *OpenAIHandler) handleListModels(w http.ResponseWriter, r *http.Request) {
	filter := &core.ChannelTypeFilter{Include: []string{"gpt-image"}}
	candidates, _ := h.router.RouteWithTypeFilter(r.Context(), filter)

	models := make([]map[string]any, 0, len(candidates))
	for _, ch := range candidates {
		models = append(models, map[string]any{
			"id":       ch.Name(),
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "gpt-image",
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
