// internal/handler/gemini_handler.go
package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"goloop/internal/kieai"
	"goloop/internal/model"
	"goloop/internal/transformer"
)

const maxRequestBodyBytes = 10 * 1024 * 1024 // 10MB

// GeminiHandler handles POST /v1beta/models/{model}:generateContent
type GeminiHandler struct {
	reqTransformer  *transformer.RequestTransformer
	respTransformer *transformer.ResponseTransformer
	client          *kieai.Client
	taskManager     *kieai.TaskManager
}

func NewGeminiHandler(
	reqTransformer *transformer.RequestTransformer,
	respTransformer *transformer.ResponseTransformer,
	client *kieai.Client,
	taskManager *kieai.TaskManager,
) *GeminiHandler {
	return &GeminiHandler{
		reqTransformer:  reqTransformer,
		respTransformer: respTransformer,
		client:          client,
		taskManager:     taskManager,
	}
}

// RegisterRoutes mounts the handler onto the provided mux.
// Route: POST /v1beta/models/{model}:generateContent
// Because Go 1.22+ path patterns require wildcard segments to be entire path segments,
// we register the parent prefix and extract the model from the URL path manually.
func (h *GeminiHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1beta/models/", h.handleGenerateContent)
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

// isStreamingRequest 检测请求是否期望 SSE 流式响应
func isStreamingRequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return accept == "text/event-stream" ||
		strings.Contains(accept, "text/event-stream") ||
		strings.Contains(accept, "multipart/x-mixed-replace")
}

// handleGenerateContentStreaming 处理 SSE 流式响应
func (h *GeminiHandler) handleGenerateContentStreaming(w http.ResponseWriter, r *http.Request, googleModel string, apiKey string, googleReq *model.GoogleRequest, requestID string) {
	ctx := r.Context()
	log := slog.With("requestId", requestID, "googleModel", googleModel)

	// Transform Google request → KIE.AI request
	kieaiReq, err := h.reqTransformer.Transform(ctx, googleReq, googleModel)
	if err != nil {
		log.Warn("request transform failed", "err", err)
		gErr, code := transformer.ToGoogleError(422, err.Error())
		h.writeSSEError(w, gErr, code)
		return
	}

	log = log.With("kieaiModel", kieaiReq.Model)

	// Submit task to KIE.AI
	taskID, err := h.client.CreateTask(ctx, apiKey, kieaiReq)
	if err != nil {
		log.Error("createTask failed", "err", err)
		code := resolveKieAIErrorCode(err)
		gErr, httpCode := transformer.ToGoogleError(code, err.Error())
		h.writeSSEError(w, gErr, httpCode)
		return
	}

	log = log.With("taskId", taskID)
	log.Info("task created, polling for result")

	// 非阻塞提交到 worker pool
	resultCh := h.taskManager.SubmitTaskStreaming(ctx, apiKey, taskID)

	// 设置 SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Request-Id", requestID)
	w.WriteHeader(http.StatusOK)

	// 确保 flush 可用
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeSSEError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 500, Message: "streaming not supported", Status: "INTERNAL"},
		}, http.StatusInternalServerError)
		return
	}

	// 发送初始 connection 事件
	h.writeSSEEvent(w, flusher, "event: connection\ndata: {\"status\":\"connected\"}\n\n")

	// 等待任务结果
	select {
	case result := <-resultCh:
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
			return
		}

		record := result.Record
		if record.ResultJSON() == nil || len(record.ResultJSON().ResultURLs) == 0 {
			log.Error("task succeeded but no result URLs")
			gErr, _ := transformer.ToGoogleError(500, "no result URLs")
			h.writeSSEError(w, gErr, 500)
			return
		}

		// Transform KIE.AI result → Google streaming response
		googleResp, err := h.respTransformer.ToGoogleStreamingResponse(ctx, record.ResultJSON().ResultURLs, requestID)
		if err != nil {
			log.Error("response transform failed", "err", err)
			gErr, _ := transformer.ToGoogleError(500, err.Error())
			h.writeSSEError(w, gErr, 500)
			return
		}

		// 发送最终结果
		h.writeSSEData(w, flusher, googleResp)
		h.writeSSEEvent(w, flusher, "data: [DONE]\n\n")

	case <-ctx.Done():
		log.Info("request cancelled")
		h.writeSSEError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 499, Message: "client closed request", Status: "CANCELLED"},
		}, 499)
	}
}

// writeSSEEvent 写入原始 SSE 事件
func (h *GeminiHandler) writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, data string) {
	w.Write([]byte(data))
	flusher.Flush()
}

// writeSSEData 写入 JSON 格式的 SSE data 事件
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

// writeSSEError 写入 SSE 错误事件并关闭连接
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

	// Extract model from path: /v1beta/models/{model}:generateContent
	// Path matched via prefix /v1beta/models/
	suffix := strings.TrimPrefix(r.URL.Path, "/v1beta/models/")
	googleModel, action, found := strings.Cut(suffix, ":")
	if !found || action != "generateContent" || googleModel == "" {
		http.NotFound(w, r)
		return
	}

	requestID := generateRequestID()
	log := slog.With("requestId", requestID, "googleModel", googleModel)

	// Extract API key from x-goog-api-key or Authorization: Bearer
	apiKey := extractAPIKey(r)
	if apiKey == "" {
		writeGoogleError(w, model.GoogleError{
			Error: model.GoogleErrorDetail{Code: 401, Message: "API key not provided", Status: "UNAUTHENTICATED"},
		}, http.StatusUnauthorized)
		return
	}

	// Parse request body (max 10MB)
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

	// 检测是否 streaming 请求
	if isStreamingRequest(r) {
		h.handleGenerateContentStreaming(w, r, googleModel, apiKey, &googleReq, requestID)
		return
	}

	// Transform Google request → KIE.AI request
	kieaiReq, err := h.reqTransformer.Transform(ctx, &googleReq, googleModel)
	if err != nil {
		log.Warn("request transform failed", "err", err)
		gErr, code := transformer.ToGoogleError(422, err.Error())
		writeGoogleError(w, gErr, code)
		return
	}

	log = log.With("kieaiModel", kieaiReq.Model)

	// Submit task to KIE.AI
	taskID, err := h.client.CreateTask(ctx, apiKey, kieaiReq)
	if err != nil {
		log.Error("createTask failed", "err", err)
		code := resolveKieAIErrorCode(err)
		gErr, httpCode := transformer.ToGoogleError(code, err.Error())
		writeGoogleError(w, gErr, httpCode)
		return
	}

	log = log.With("taskId", taskID)
	log.Info("task created, submitting to worker pool")

	// Submit to worker pool for polling
	result, err := h.taskManager.SubmitTask(ctx, apiKey, taskID)
	if err != nil {
		log.Error("poll failed", "err", err)
		var tErr *kieai.TaskFailedError
		if errors.As(err, &tErr) {
			gErr, httpCode := transformer.ToGoogleError(500, tErr.Reason)
			writeGoogleError(w, gErr, httpCode)
			return
		}
		gErr, httpCode := transformer.ToGoogleError(500, err.Error())
		writeGoogleError(w, gErr, httpCode)
		return
	}

	record := result.Record

	if record.ResultJSON() == nil || len(record.ResultJSON().ResultURLs) == 0 {
		log.Error("task succeeded but no result URLs")
		gErr, httpCode := transformer.ToGoogleError(500, "no result URLs in successful task")
		writeGoogleError(w, gErr, httpCode)
		return
	}

	log.Info("task completed", "imageCount", len(record.ResultJSON().ResultURLs))

	// Transform KIE.AI result → Google response
	googleResp, err := h.respTransformer.ToGoogleResponse(ctx, record.ResultJSON().ResultURLs)
	if err != nil {
		log.Error("response transform failed", "err", err)
		gErr, httpCode := transformer.ToGoogleError(500, err.Error())
		writeGoogleError(w, gErr, httpCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(googleResp)
}

func extractAPIKey(r *http.Request) string {
	if key := r.Header.Get("x-goog-api-key"); key != "" {
		return key
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func writeGoogleError(w http.ResponseWriter, e model.GoogleError, httpCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	json.NewEncoder(w).Encode(e)
}

func resolveKieAIErrorCode(err error) int {
	var kErr *kieai.ErrKieAI
	if errors.As(err, &kErr) {
		return kErr.Code
	}
	return 500
}

func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
