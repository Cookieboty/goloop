package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"goloop/internal/core"
	"goloop/internal/model"
)

// AdminHandler provides administrative operations.
type AdminHandler struct {
	issuer   *core.JWTIssuer
	registry *core.PluginRegistry
	health   *core.HealthTracker
}

func NewAdminHandler(issuer *core.JWTIssuer, registry *core.PluginRegistry, health *core.HealthTracker) *AdminHandler {
	return &AdminHandler{issuer: issuer, registry: registry, health: health}
}

func (h *AdminHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/issue-token", h.handleIssueToken)
	mux.HandleFunc("GET /admin/stats", h.handleStats)
	mux.HandleFunc("GET /admin/channel/", h.handleChannelAccounts)
	mux.HandleFunc("POST /admin/channel/", h.handleChannelOp)
}

func (h *AdminHandler) handleIssueToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject string `json:"subject"`
		APIKey  string `json:"api_key"`
		Channel string `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	claims := &core.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: req.Subject,
		},
		APIKey:  req.APIKey,
		Channel: req.Channel,
	}
	token, err := h.issuer.Issue(claims)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (h *AdminHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := make(map[string]any)
	for _, ch := range h.registry.List() {
		fail, success := h.health.TotalStats(ch.Name())
		stats[ch.Name()] = map[string]any{
			"health_score":   h.health.HealthScore(ch.Name()),
			"is_healthy":    h.health.IsHealthy(ch.Name()),
			"total_fail":    fail,
			"total_success": success,
			"avg_latency_ms": h.health.AverageLatency(ch.Name()).Milliseconds(),
		}
	}
	json.NewEncoder(w).Encode(stats)
}

func (h *AdminHandler) handleChannelAccounts(w http.ResponseWriter, r *http.Request) {
	// GET /admin/channel/{channel}/accounts
	path := strings.TrimPrefix(r.URL.Path, "/admin/channel/")
	if !strings.HasSuffix(path, "/accounts") {
		http.NotFound(w, r)
		return
	}
	channelName := strings.TrimSuffix(path, "/accounts")

	ch, ok := h.registry.Get(channelName)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "channel not found")
		return
	}

	// Try to get accounts via ChannelWithPool interface
	if chwp, ok := ch.(interface{ ListAccounts() []map[string]any }); ok {
		json.NewEncoder(w).Encode(chwp.ListAccounts())
		return
	}
	json.NewEncoder(w).Encode([]any{})
}

func (h *AdminHandler) handleChannelOp(w http.ResponseWriter, r *http.Request) {
	// POST /admin/channel/{channel}/accounts/{apiKey}/{op}
	// ops: reset, retire, probe
	path := strings.TrimPrefix(r.URL.Path, "/admin/channel/")
	parts := strings.Split(path, "/")
	if len(parts) < 4 || parts[2] != "accounts" {
		http.NotFound(w, r)
		return
	}
	channelName := parts[0]
	apiKey := parts[1]
	op := parts[3]

	_, ok := h.registry.Get(channelName)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "channel not found")
		return
	}

	result := map[string]string{"status": "ok", "channel": channelName, "apiKey": apiKey, "op": op}
	json.NewEncoder(w).Encode(result)
}

func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(model.GoogleError{
		Error: model.GoogleErrorDetail{Code: code, Message: message, Status: "INVALID_ARGUMENT"},
	})
}
