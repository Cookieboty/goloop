package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	
	"goloop/internal/cache"
	"goloop/internal/core"
	"goloop/internal/database"
)

// ContextKey for storing API Key ID in request context
type contextKey string

const apiKeyIDKey contextKey = "api_key_id"

// WithAPIKeyID adds API Key ID to context
func WithAPIKeyID(ctx context.Context, id uint) context.Context {
	return context.WithValue(ctx, apiKeyIDKey, id)
}

// GetAPIKeyID retrieves API Key ID from context
func GetAPIKeyID(ctx context.Context) (uint, bool) {
	id, ok := ctx.Value(apiKeyIDKey).(uint)
	return id, ok
}

// APIKeyMiddleware provides API Key authentication for client requests
type APIKeyMiddleware struct {
	cache      *cache.APIKeyCache
	repo       *database.Repository
	configMgr  *core.ConfigManager
	next       http.Handler
}

// NewAPIKeyMiddleware creates API Key authentication middleware
func NewAPIKeyMiddleware(cache *cache.APIKeyCache, repo *database.Repository, configMgr *core.ConfigManager, next http.Handler) *APIKeyMiddleware {
	return &APIKeyMiddleware{
		cache:     cache,
		repo:      repo,
		configMgr: configMgr,
		next:      next,
	}
}

func (m *APIKeyMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract API Key from Authorization header
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "missing or invalid authorization header")
		return
	}
	
	apiKey := strings.TrimPrefix(authHeader, "Bearer ")
	if !strings.HasPrefix(apiKey, "goloop_") {
		writeError(w, http.StatusUnauthorized, "invalid API key format")
		return
	}
	
	// Verify API Key
	ctx := r.Context()
	apiKeyInfo, err := m.verifyAPIKey(ctx, apiKey)
	if err != nil {
		if errors.Is(err, ErrRedisUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "authentication service temporarily unavailable")
			return
		}
		writeError(w, http.StatusUnauthorized, "invalid or expired API key")
		return
	}
	
	// Check channel restriction
	if apiKeyInfo.ChannelRestriction != nil && *apiKeyInfo.ChannelRestriction != "" {
		ctx = core.WithChannelRestriction(ctx, *apiKeyInfo.ChannelRestriction)
	}
	
	// Add API Key ID to context for usage logging
	ctx = WithAPIKeyID(ctx, apiKeyInfo.ID)
	
	m.next.ServeHTTP(w, r.WithContext(ctx))
}

var ErrRedisUnavailable = errors.New("redis unavailable")

func (m *APIKeyMiddleware) verifyAPIKey(ctx context.Context, key string) (*cache.APIKeyInfo, error) {
	// Try Redis cache first
	if m.cache != nil && m.cache.IsAvailable(ctx) {
		apiKeyInfo, err := m.cache.Get(ctx, key)
		if err == nil {
			// Cache hit - validate
			return apiKeyInfo, m.validateAPIKey(apiKeyInfo)
		}
		// Cache miss - query database
	} else if m.cache != nil {
		// Redis is unavailable - return error (security first)
		slog.Error("redis unavailable, rejecting api key request")
		return nil, ErrRedisUnavailable
	}
	
	// Query database
	dbAPIKey, err := m.repo.GetAPIKeyByKey(key)
	if err != nil {
		return nil, err
	}
	
	apiKeyInfo := &cache.APIKeyInfo{
		ID:                 dbAPIKey.ID,
		Enabled:            dbAPIKey.Enabled,
		ExpiresAt:          dbAPIKey.ExpiresAt,
		ChannelRestriction: dbAPIKey.ChannelRestriction,
	}
	
	// Write to cache
	if m.cache != nil && m.cache.IsAvailable(ctx) {
		if err := m.cache.Set(ctx, key, apiKeyInfo); err != nil {
			slog.Warn("failed to cache api key", "err", err)
		}
	}
	
	return apiKeyInfo, m.validateAPIKey(apiKeyInfo)
}

func (m *APIKeyMiddleware) validateAPIKey(info *cache.APIKeyInfo) error {
	if !info.Enabled {
		return fmt.Errorf("API key is disabled")
	}
	
	if info.ExpiresAt != nil && time.Now().After(*info.ExpiresAt) {
		return fmt.Errorf("API key has expired")
	}
	
	return nil
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
