package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	
	"goloop/internal/cache"
	"goloop/internal/core"
	"goloop/internal/database"
)

// AdminCRUDHandler handles database-driven CRUD operations
type AdminCRUDHandler struct {
	repo        *database.Repository
	cache       *cache.APIKeyCache
	configMgr   *core.ConfigManager
	registry    *core.PluginRegistry
	health      *core.HealthTracker
	requireAuth func(http.HandlerFunc) http.HandlerFunc
}

// NewAdminCRUDHandler creates a new CRUD handler
func NewAdminCRUDHandler(repo *database.Repository, cache *cache.APIKeyCache, configMgr *core.ConfigManager, registry *core.PluginRegistry, health *core.HealthTracker, requireAuth func(http.HandlerFunc) http.HandlerFunc) *AdminCRUDHandler {
	return &AdminCRUDHandler{
		repo:        repo,
		cache:       cache,
		configMgr:   configMgr,
		registry:    registry,
		health:      health,
		requireAuth: requireAuth,
	}
}

// RegisterRoutes registers all CRUD routes
func (h *AdminCRUDHandler) RegisterRoutes(mux *http.ServeMux) {
	// Channel CRUD
	mux.HandleFunc("GET /admin/api/channels", h.requireAuth(h.handleGetChannels))
	mux.HandleFunc("POST /admin/api/channels", h.requireAuth(h.handleCreateChannel))
	mux.HandleFunc("GET /admin/api/channels/{id}", h.requireAuth(h.handleGetChannel))
	mux.HandleFunc("PUT /admin/api/channels/{id}", h.requireAuth(h.handleUpdateChannel))
	mux.HandleFunc("DELETE /admin/api/channels/{id}", h.requireAuth(h.handleDeleteChannel))
	mux.HandleFunc("POST /admin/api/channels/{id}/toggle", h.requireAuth(h.handleToggleChannel))
	
	// Account CRUD
	mux.HandleFunc("GET /admin/api/channels/{channelId}/accounts", h.requireAuth(h.handleGetAccounts))
	mux.HandleFunc("POST /admin/api/channels/{channelId}/accounts", h.requireAuth(h.handleCreateAccount))
	mux.HandleFunc("PUT /admin/api/accounts/{id}", h.requireAuth(h.handleUpdateAccount))
	mux.HandleFunc("DELETE /admin/api/accounts/{id}", h.requireAuth(h.handleDeleteAccount))
	mux.HandleFunc("POST /admin/api/accounts/{id}/toggle", h.requireAuth(h.handleToggleAccount))
	
	// Model Mapping CRUD
	mux.HandleFunc("GET /admin/api/channels/{channelId}/mappings", h.requireAuth(h.handleGetMappings))
	mux.HandleFunc("POST /admin/api/channels/{channelId}/mappings", h.requireAuth(h.handleCreateMapping))
	mux.HandleFunc("PUT /admin/api/mappings/{id}", h.requireAuth(h.handleUpdateMapping))
	mux.HandleFunc("DELETE /admin/api/mappings/{id}", h.requireAuth(h.handleDeleteMapping))
	
	// API Key CRUD
	mux.HandleFunc("GET /admin/api/api-keys", h.requireAuth(h.handleGetAPIKeys))
	mux.HandleFunc("POST /admin/api/api-keys", h.requireAuth(h.handleCreateAPIKey))
	mux.HandleFunc("GET /admin/api/api-keys/{id}", h.requireAuth(h.handleGetAPIKey))
	mux.HandleFunc("PUT /admin/api/api-keys/{id}", h.requireAuth(h.handleUpdateAPIKey))
	mux.HandleFunc("DELETE /admin/api/api-keys/{id}", h.requireAuth(h.handleDeleteAPIKey))
	mux.HandleFunc("POST /admin/api/api-keys/{id}/toggle", h.requireAuth(h.handleToggleAPIKey))
	
	// Usage Logs
	mux.HandleFunc("GET /admin/api/api-keys/{id}/logs", h.requireAuth(h.handleGetUsageLogs))
	mux.HandleFunc("GET /admin/api/api-keys/{id}/stats", h.requireAuth(h.handleGetAPIKeyStats))
	
	// Error Logs
	mux.HandleFunc("GET /admin/api/error-logs", h.requireAuth(h.handleGetErrorLogs))
	
	// Global Stats
	mux.HandleFunc("GET /admin/api/global-stats", h.requireAuth(h.handleGetGlobalStats))
	
	// Config Reload
	mux.HandleFunc("POST /admin/api/reload", h.requireAuth(h.handleReload))
}

// ==================== Channel CRUD ====================

func (h *AdminCRUDHandler) handleGetChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := h.repo.GetAllChannels()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to get channels: "+err.Error())
		return
	}
	writeJSON(w, channels)
}

func (h *AdminCRUDHandler) handleGetChannel(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}
	
	channel, err := h.repo.GetChannelByID(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "channel not found")
		return
	}
	writeJSON(w, channel)
}

type createChannelRequest struct {
	Name                   string                   `json:"name"`
	Type                   string                   `json:"type"`
	BaseURL                string                   `json:"base_url"`
	Weight                 int                      `json:"weight"`
	TimeoutSeconds         int                      `json:"timeout_seconds"`
	InitialIntervalSeconds *int                     `json:"initial_interval_seconds"`
	MaxIntervalSeconds     *int                     `json:"max_interval_seconds"`
	MaxWaitTimeSeconds     *int                     `json:"max_wait_time_seconds"`
	RetryAttempts          *int                     `json:"retry_attempts"`
	ProbeModel             *string                  `json:"probe_model"`
	Accounts               []createAccountRequest   `json:"accounts"`
	ModelMappings          []createMappingRequest   `json:"model_mappings"`
}

type createAccountRequest struct {
	APIKey string `json:"api_key"`
	Weight int    `json:"weight"`
}

type createMappingRequest struct {
	SourceModel string `json:"source_model"`
	TargetModel string `json:"target_model"`
}

func (h *AdminCRUDHandler) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	var req createChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	
	// Validate required fields
	if req.Name == "" || req.Type == "" || req.BaseURL == "" {
		writeJSONError(w, http.StatusBadRequest, "name, type, and base_url are required")
		return
	}
	
	// Create channel with transaction
	channel := &database.Channel{
		Name:                   req.Name,
		Type:                   req.Type,
		BaseURL:                req.BaseURL,
		Weight:                 req.Weight,
		TimeoutSeconds:         req.TimeoutSeconds,
		InitialIntervalSeconds: req.InitialIntervalSeconds,
		MaxIntervalSeconds:     req.MaxIntervalSeconds,
		MaxWaitTimeSeconds:     req.MaxWaitTimeSeconds,
		RetryAttempts:          req.RetryAttempts,
		ProbeModel:             req.ProbeModel,
		Enabled:                true,
	}
	
	accounts := make([]database.Account, len(req.Accounts))
	for i, acc := range req.Accounts {
		accounts[i] = database.Account{
			APIKey:  acc.APIKey,
			Weight:  acc.Weight,
			Enabled: true,
		}
	}
	
	mappings := make([]database.ModelMapping, len(req.ModelMappings))
	for i, m := range req.ModelMappings {
		mappings[i] = database.ModelMapping{
			SourceModel: m.SourceModel,
			TargetModel: m.TargetModel,
		}
	}
	
	if err := h.repo.CreateChannelWithAccounts(channel, accounts, mappings); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create channel: "+err.Error())
		return
	}
	
	// Reload config (only updates config cache, does not register new channels)
	if err := h.configMgr.Reload(); err != nil {
		slog.Warn("failed to reload config after channel creation", "err", err)
	}
	
	// Note: New channels require service restart to take effect
	// TODO: Implement hot reload mechanism to register new channels without restart
	writeJSON(w, map[string]interface{}{
		"channel": channel,
		"message": "渠道已创建成功。注意：新渠道需要重启服务才能生效。",
		"requires_restart": true,
	})
}

func (h *AdminCRUDHandler) handleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}
	
	var req createChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	
	// Get existing channel
	channel, err := h.repo.GetChannelByID(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "channel not found")
		return
	}
	
	// Update fields
	channel.Name = req.Name
	channel.Type = req.Type
	channel.BaseURL = req.BaseURL
	channel.Weight = req.Weight
	channel.TimeoutSeconds = req.TimeoutSeconds
	channel.InitialIntervalSeconds = req.InitialIntervalSeconds
	channel.MaxIntervalSeconds = req.MaxIntervalSeconds
	channel.MaxWaitTimeSeconds = req.MaxWaitTimeSeconds
	channel.RetryAttempts = req.RetryAttempts
	channel.ProbeModel = req.ProbeModel
	
	if err := h.repo.UpdateChannel(channel); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update channel: "+err.Error())
		return
	}
	
	// Reload config (only updates config cache, does not re-register channels)
	if err := h.configMgr.Reload(); err != nil {
		slog.Warn("failed to reload config after channel update", "err", err)
	}
	
	// Note: Channel configuration updates require service restart to take effect
	// TODO: Implement hot reload mechanism to update channel configs without restart
	writeJSON(w, map[string]interface{}{
		"channel": channel,
		"message": "渠道已更新成功。注意：配置变更需要重启服务才能生效。",
		"requires_restart": true,
	})
}

func (h *AdminCRUDHandler) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}
	
	if err := h.repo.DeleteChannel(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete channel: "+err.Error())
		return
	}
	
	// Reload config
	if err := h.configMgr.Reload(); err != nil {
		slog.Warn("failed to reload config after channel deletion", "err", err)
	}
	
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminCRUDHandler) handleToggleChannel(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}
	
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	
	if err := h.repo.ToggleChannel(id, req.Enabled); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to toggle channel: "+err.Error())
		return
	}
	
	// Reload config
	if err := h.configMgr.Reload(); err != nil {
		slog.Warn("failed to reload config after channel toggle", "err", err)
	}
	
	writeJSON(w, map[string]bool{"enabled": req.Enabled})
}

// ==================== Account CRUD ====================

func (h *AdminCRUDHandler) handleGetAccounts(w http.ResponseWriter, r *http.Request) {
	channelID, err := parseID(r.PathValue("channelId"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}
	
	accounts, err := h.repo.GetChannelAccounts(channelID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to get accounts: "+err.Error())
		return
	}
	writeJSON(w, accounts)
}

func (h *AdminCRUDHandler) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	channelID, err := parseID(r.PathValue("channelId"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}
	
	var req struct {
		APIKey string `json:"api_key"`
		Weight int    `json:"weight"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	
	account := &database.Account{
		ChannelID: channelID,
		APIKey:    req.APIKey,
		Weight:    req.Weight,
		Enabled:   true,
	}
	
	if err := h.repo.CreateAccount(account); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create account: "+err.Error())
		return
	}
	
	// Reload config
	if err := h.configMgr.Reload(); err != nil {
		slog.Warn("failed to reload config after account creation", "err", err)
	}
	
	writeJSON(w, account)
}

func (h *AdminCRUDHandler) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid account ID")
		return
	}
	
	var req struct {
		APIKey  string `json:"api_key"`
		Weight  int    `json:"weight"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	
	account := &database.Account{
		ID:      id,
		APIKey:  req.APIKey,
		Weight:  req.Weight,
		Enabled: req.Enabled,
	}
	
	if err := h.repo.UpdateAccount(account); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update account: "+err.Error())
		return
	}
	
	// Reload config
	if err := h.configMgr.Reload(); err != nil {
		slog.Warn("failed to reload config after account update", "err", err)
	}
	
	writeJSON(w, account)
}

func (h *AdminCRUDHandler) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid account ID")
		return
	}
	
	if err := h.repo.DeleteAccount(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete account: "+err.Error())
		return
	}
	
	// Reload config
	if err := h.configMgr.Reload(); err != nil {
		slog.Warn("failed to reload config after account deletion", "err", err)
	}
	
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminCRUDHandler) handleToggleAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid account ID")
		return
	}
	
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	
	if err := h.repo.ToggleAccount(id, req.Enabled); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to toggle account: "+err.Error())
		return
	}
	
	// Reload config
	if err := h.configMgr.Reload(); err != nil {
		slog.Warn("failed to reload config after account toggle", "err", err)
	}
	
	writeJSON(w, map[string]bool{"enabled": req.Enabled})
}

// ==================== Model Mapping CRUD ====================

func (h *AdminCRUDHandler) handleGetMappings(w http.ResponseWriter, r *http.Request) {
	channelID, err := parseID(r.PathValue("channelId"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}
	
	mappings, err := h.repo.GetModelMappings(channelID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to get model mappings: "+err.Error())
		return
	}
	writeJSON(w, mappings)
}

func (h *AdminCRUDHandler) handleCreateMapping(w http.ResponseWriter, r *http.Request) {
	channelID, err := parseID(r.PathValue("channelId"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid channel ID")
		return
	}
	
	var req struct {
		SourceModel string `json:"source_model"`
		TargetModel string `json:"target_model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	
	mapping := &database.ModelMapping{
		ChannelID:   channelID,
		SourceModel: req.SourceModel,
		TargetModel: req.TargetModel,
	}
	
	if err := h.repo.CreateModelMapping(mapping); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create model mapping: "+err.Error())
		return
	}
	
	// Reload config
	if err := h.configMgr.Reload(); err != nil {
		slog.Warn("failed to reload config after mapping creation", "err", err)
	}
	
	writeJSON(w, mapping)
}

func (h *AdminCRUDHandler) handleUpdateMapping(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid mapping ID")
		return
	}
	
	var req struct {
		SourceModel string `json:"source_model"`
		TargetModel string `json:"target_model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	
	mapping := &database.ModelMapping{
		ID:          id,
		SourceModel: req.SourceModel,
		TargetModel: req.TargetModel,
	}
	
	if err := h.repo.UpdateModelMapping(mapping); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update model mapping: "+err.Error())
		return
	}
	
	// Reload config
	if err := h.configMgr.Reload(); err != nil {
		slog.Warn("failed to reload config after mapping update", "err", err)
	}
	
	writeJSON(w, mapping)
}

func (h *AdminCRUDHandler) handleDeleteMapping(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid mapping ID")
		return
	}
	
	if err := h.repo.DeleteModelMapping(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete model mapping: "+err.Error())
		return
	}
	
	// Reload config
	if err := h.configMgr.Reload(); err != nil {
		slog.Warn("failed to reload config after mapping deletion", "err", err)
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// ==================== API Key CRUD ====================

func (h *AdminCRUDHandler) handleGetAPIKeys(w http.ResponseWriter, r *http.Request) {
	apiKeys, err := h.repo.GetAllAPIKeys()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to get API keys: "+err.Error())
		return
	}
	writeJSON(w, apiKeys)
}

func (h *AdminCRUDHandler) handleGetAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid API key ID")
		return
	}
	
	apiKey, err := h.repo.GetAPIKeyByID(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "API key not found")
		return
	}
	writeJSON(w, apiKey)
}

func (h *AdminCRUDHandler) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name               string     `json:"name"`
		ChannelRestriction *string    `json:"channel_restriction"`
		ExpiresAt          *time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	
	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	
	apiKey := &database.APIKey{
		Name:               req.Name,
		ChannelRestriction: req.ChannelRestriction,
		ExpiresAt:          req.ExpiresAt,
		Enabled:            true,
	}
	
	if err := h.repo.CreateAPIKey(apiKey); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create API key: "+err.Error())
		return
	}
	
	writeJSON(w, apiKey)
}

func (h *AdminCRUDHandler) handleUpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid API key ID")
		return
	}
	
	var req struct {
		Name               string     `json:"name"`
		ChannelRestriction *string    `json:"channel_restriction"`
		ExpiresAt          *time.Time `json:"expires_at"`
		Enabled            bool       `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	
	// Get existing API key
	apiKey, err := h.repo.GetAPIKeyByID(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "API key not found")
		return
	}
	
	// Update fields
	apiKey.Name = req.Name
	apiKey.ChannelRestriction = req.ChannelRestriction
	apiKey.ExpiresAt = req.ExpiresAt
	apiKey.Enabled = req.Enabled
	
	if err := h.repo.UpdateAPIKey(apiKey); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update API key: "+err.Error())
		return
	}
	
	// Clear cache
	if h.cache != nil {
		_ = h.cache.Delete(r.Context(), apiKey.Key)
	}
	
	writeJSON(w, apiKey)
}

func (h *AdminCRUDHandler) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid API key ID")
		return
	}
	
	// Get API key for cache invalidation
	apiKey, err := h.repo.GetAPIKeyByID(id)
	if err == nil && h.cache != nil {
		_ = h.cache.Delete(r.Context(), apiKey.Key)
	}
	
	if err := h.repo.DeleteAPIKey(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete API key: "+err.Error())
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminCRUDHandler) handleToggleAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid API key ID")
		return
	}
	
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	
	// Get API key for cache invalidation
	apiKey, err := h.repo.GetAPIKeyByID(id)
	if err == nil && h.cache != nil {
		_ = h.cache.Delete(r.Context(), apiKey.Key)
	}
	
	if err := h.repo.ToggleAPIKey(id, req.Enabled); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to toggle API key: "+err.Error())
		return
	}
	
	writeJSON(w, map[string]bool{"enabled": req.Enabled})
}

// ==================== Usage Logs ====================

func (h *AdminCRUDHandler) handleGetUsageLogs(w http.ResponseWriter, r *http.Request) {
	apiKeyID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid API key ID")
		return
	}
	
	// Parse query params
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	
	logs, err := h.repo.GetUsageLogs(apiKeyID, limit, offset, nil, nil)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to get usage logs: "+err.Error())
		return
	}
	
	writeJSON(w, logs)
}

func (h *AdminCRUDHandler) handleGetAPIKeyStats(w http.ResponseWriter, r *http.Request) {
	apiKeyID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid API key ID")
		return
	}
	
	stats, err := h.repo.GetAPIKeyStatsByChannel(apiKeyID, nil, nil)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to get API key stats: "+err.Error())
		return
	}
	
	writeJSON(w, stats)
}

// ==================== Error Logs ====================

func (h *AdminCRUDHandler) handleGetErrorLogs(w http.ResponseWriter, r *http.Request) {
	// Parse query params
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	
	// Parse time range params
	var startDate, endDate *time.Time
	if s := r.URL.Query().Get("start_date"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			startDate = &t
		}
	}
	if e := r.URL.Query().Get("end_date"); e != "" {
		if t, err := time.Parse(time.RFC3339, e); err == nil {
			endDate = &t
		}
	}
	
	// Get error logs
	logs, err := h.repo.GetErrorLogs(limit, offset, startDate, endDate)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to get error logs: "+err.Error())
		return
	}
	
	// Get total count
	totalCount, err := h.repo.GetErrorLogsCount(startDate, endDate)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to count error logs: "+err.Error())
		return
	}
	
	writeJSON(w, map[string]interface{}{
		"logs":  logs,
		"total": totalCount,
	})
}

// ==================== Config Reload ====================

func (h *AdminCRUDHandler) handleReload(w http.ResponseWriter, r *http.Request) {
	if err := h.configMgr.Reload(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to reload config: "+err.Error())
		return
	}
	
	writeJSON(w, map[string]string{
		"message": "config reloaded successfully",
	})
}

// ==================== Global Stats ====================

func (h *AdminCRUDHandler) handleGetGlobalStats(w http.ResponseWriter, r *http.Request) {
	typeStats, channelDetails, err := h.repo.GetGlobalStats()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to get global stats: "+err.Error())
		return
	}
	
	// Get all channels from database for display names
	dbChannels, err := h.repo.GetAllChannels()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to get channels: "+err.Error())
		return
	}
	
	// Create bidirectional maps for channel name/type lookup
	// This handles legacy data where UsageLog stored type names instead of channel names
	channelDisplayNames := make(map[string]string) // channel_name -> display_name
	typeToName := make(map[string]string)           // channel_type -> channel_name (for legacy data)
	for _, ch := range dbChannels {
		channelDisplayNames[ch.Name] = ch.Name
		typeToName[ch.Type] = ch.Name // Map type to name for legacy data lookup
	}
	
	// Enrich channel details with health info and display name
	enrichedDetails := make([]map[string]interface{}, 0, len(channelDetails))
	for _, detail := range channelDetails {
		lookupName := detail.ChannelName
		
		// Try direct lookup first
		ch, found := h.registry.Get(lookupName)
		
		// If not found, check if this is a legacy type name and try mapping it
		if !found {
			if actualName, ok := typeToName[lookupName]; ok {
				ch, found = h.registry.Get(actualName)
			}
		}
		
		// Get display name - use actual name from registry if found
		displayName := detail.ChannelName
		if found {
			actualName := ch.Name()
			if dbName, ok := channelDisplayNames[actualName]; ok {
				displayName = dbName
			} else {
				displayName = actualName
			}
		} else if dbName, ok := channelDisplayNames[detail.ChannelName]; ok {
			displayName = dbName
		}
		
		enriched := map[string]interface{}{
			"channel_name":    detail.ChannelName,
			"channel_type":    detail.ChannelType,
			"display_name":    displayName,
			"total_requests":  detail.TotalRequests,
			"total_success":   detail.TotalSuccess,
			"total_fail":      detail.TotalFail,
			"success_rate":    detail.SuccessRate,
			"avg_latency_ms":  detail.AvgLatencyMs,
		}
		
		if found {
			// Add health info from channel - use actual channel name
			actualName := ch.Name()
			enriched["health_score"] = h.health.HealthScore(actualName)
			enriched["is_healthy"] = h.health.IsHealthy(actualName)
			
			// Also update channel_name to the actual name if it was a legacy type lookup
			if actualName != detail.ChannelName {
				enriched["channel_name"] = actualName
			}
		} else {
			// Channel not in registry (maybe deleted)
			enriched["health_score"] = 0.0
			enriched["is_healthy"] = false
		}
		
		enrichedDetails = append(enrichedDetails, enriched)
	}
	
	writeJSON(w, map[string]interface{}{
		"type_stats":      typeStats,
		"channel_details": enrichedDetails,
	})
}

// ==================== Helper Functions ====================

func parseID(s string) (uint, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	return uint(id), err
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
