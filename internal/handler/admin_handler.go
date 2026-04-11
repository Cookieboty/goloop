package handler

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"goloop/internal/core"
	"goloop/internal/model"
)

// AdminHandler provides administrative operations.
type AdminHandler struct {
	issuer        *core.JWTIssuer
	registry      *core.PluginRegistry
	health        *core.HealthTracker
	adminPassword string
}

func NewAdminHandler(issuer *core.JWTIssuer, registry *core.PluginRegistry, health *core.HealthTracker, adminPassword string) *AdminHandler {
	return &AdminHandler{
		issuer:        issuer,
		registry:      registry,
		health:        health,
		adminPassword: adminPassword,
	}
}

func (h *AdminHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/issue-token", h.requireAuth(h.handleIssueToken))
	mux.HandleFunc("GET /admin/stats", h.requireAuth(h.handleStats))
	mux.HandleFunc("GET /admin/channel/", h.requireAuth(h.handleChannelAccounts))
	mux.HandleFunc("POST /admin/channel/", h.requireAuth(h.handleChannelOp))
}

// requireAuth wraps a handler with admin password authentication.
// Supports two mechanisms:
//   - Header: X-Admin-Key: <password>
//   - HTTP Basic Auth: any_username:<password>
// If adminPassword is empty (dev mode), auth is skipped.
func (h *AdminHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Dev mode: no password configured, skip auth
		if h.adminPassword == "" {
			next(w, r)
			return
		}

		pw := []byte(h.adminPassword)

		// Method 1: X-Admin-Key header
		if key := r.Header.Get("X-Admin-Key"); key != "" {
			if subtle.ConstantTimeCompare([]byte(key), pw) == 1 {
				next(w, r)
				return
			}
			// Wrong key — don't fall through to Basic Auth
			writeJSONError(w, http.StatusUnauthorized, "invalid admin key")
			return
		}

		// Method 2: HTTP Basic Auth (any username, password must match)
		_, pass, ok := r.BasicAuth()
		if ok && subtle.ConstantTimeCompare([]byte(pass), pw) == 1 {
			next(w, r)
			return
		}

		w.Header().Set("WWW-Authenticate", `Basic realm="goloop admin"`)
		writeJSONError(w, http.StatusUnauthorized, "admin authentication required")
	}
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
	if req.Subject == "" {
		writeJSONError(w, http.StatusBadRequest, "subject is required")
		return
	}
	if req.APIKey == "" {
		writeJSONError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	// If a channel is specified, validate it exists in the registry.
	// Empty channel means the token can access all channels (super token).
	if req.Channel != "" {
		if _, ok := h.registry.Get(req.Channel); !ok {
			writeJSONError(w, http.StatusBadRequest, "channel not found: "+req.Channel)
			return
		}
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (h *AdminHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := make(map[string]any)
	for _, ch := range h.registry.List() {
		fail, success := h.health.TotalStats(ch.Name())
		stats[ch.Name()] = map[string]any{
			"health_score":    h.health.HealthScore(ch.Name()),
			"is_healthy":      h.health.IsHealthy(ch.Name()),
			"total_fail":      fail,
			"total_success":   success,
			"avg_latency_ms":  h.health.AverageLatency(ch.Name()).Milliseconds(),
		}
	}
	w.Header().Set("Content-Type", "application/json")
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

	// Try to get accounts via ListAccounts interface.
	type accountLister interface {
		ListAccounts() []map[string]any
	}
	w.Header().Set("Content-Type", "application/json")
	if lister, ok := ch.(accountLister); ok {
		json.NewEncoder(w).Encode(lister.ListAccounts())
		return
	}
	json.NewEncoder(w).Encode([]any{})
}

func (h *AdminHandler) handleChannelOp(w http.ResponseWriter, r *http.Request) {
	// POST /admin/channel/{channel}/accounts/{apiKey}/{op}
	// ops: reset, retire, probe
	path := strings.TrimPrefix(r.URL.Path, "/admin/channel/")
	parts := strings.Split(path, "/")
	if len(parts) < 4 || parts[1] != "accounts" {
		http.NotFound(w, r)
		return
	}
	channelName := parts[0]
	apiKey := parts[2]
	op := parts[3]

	ch, ok := h.registry.Get(channelName)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "channel not found")
		return
	}

	// Delegate to ChannelWithAccountOps interface if available.
	type accountOps interface {
		ResetAccount(apiKey string) bool
		RetireAccount(apiKey string) bool
		ProbeAccount(apiKey string) bool
	}

	w.Header().Set("Content-Type", "application/json")

	ops, hasOps := ch.(accountOps)
	var ok2 bool
	switch op {
	case "reset":
		if hasOps {
			ok2 = ops.ResetAccount(apiKey)
		}
	case "retire":
		if hasOps {
			ok2 = ops.RetireAccount(apiKey)
		}
	case "probe":
		if hasOps {
			ok2 = ops.ProbeAccount(apiKey)
		}
	default:
		writeJSONError(w, http.StatusBadRequest, "unknown op: "+op)
		return
	}

	if !hasOps {
		writeJSONError(w, http.StatusNotImplemented, "channel does not support account operations")
		return
	}
	if !ok2 {
		writeJSONError(w, http.StatusNotFound, "account not found or operation failed")
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "channel": channelName, "op": op})
}

func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(model.GoogleError{
		Error: model.GoogleErrorDetail{Code: code, Message: message, Status: "INVALID_ARGUMENT"},
	})
}
