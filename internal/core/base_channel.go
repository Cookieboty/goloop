package core

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"goloop/internal/model"
)

// BaseChannel provides a default implementation of the Channel interface.
// Embed it in a concrete channel struct and override only the methods that
// differ from the defaults (typically Generate or SubmitTask+PollTask, and
// optionally Probe).
//
// All admin helper methods (ListAccounts, ResetAccount, RetireAccount,
// ProbeAccount, SetWeight, SetChannelWeight, GetAccountPool) are provided
// automatically and delegate to the embedded Pool.
type BaseChannel struct {
	ChannelName string
	ChannelType string // Type of channel: "gemini", "kieai", "subrouter", "gpt-image"
	BaseURL     string
	HTTPClient  *http.Client
	Pool        *DefaultAccountPool
	weight      atomic.Int32
}

// NewBaseChannel constructs a BaseChannel with the given parameters.
func NewBaseChannel(name, ctype, baseURL string, weight int, pool *DefaultAccountPool, timeout time.Duration) BaseChannel {
	bc := BaseChannel{
		ChannelName: name,
		ChannelType: ctype,
		BaseURL:     baseURL,
		HTTPClient:  &http.Client{Timeout: timeout},
		Pool:        pool,
	}
	bc.weight.Store(int32(weight))
	return bc
}

// Name returns the channel's registered name.
func (b *BaseChannel) Name() string { return b.ChannelName }

// Type returns the channel's type identifier.
func (b *BaseChannel) Type() string { return b.ChannelType }

// Weight returns the channel's routing weight (priority).
func (b *BaseChannel) Weight() int { return int(b.weight.Load()) }

// SetChannelWeight updates the channel's routing weight at runtime.
func (b *BaseChannel) SetChannelWeight(w int) { b.weight.Store(int32(w)) }

// IsAvailable returns true when the pool has at least one account.
func (b *BaseChannel) IsAvailable() bool {
	return b.Pool != nil && len(b.Pool.List()) > 0
}

// HealthScore returns the average health score across all accounts in the pool.
func (b *BaseChannel) HealthScore() float64 {
	if b.Pool == nil {
		return 0
	}
	accounts := b.Pool.List()
	if len(accounts) == 0 {
		return 0
	}
	var total float64
	for _, acc := range accounts {
		total += acc.HealthScore()
	}
	return total / float64(len(accounts))
}

// Generate returns ErrNotSupported by default.
// Override this method in channels that use a synchronous API.
func (b *BaseChannel) Generate(_ context.Context, _ *model.GoogleRequest, _ string) (*model.GoogleResponse, error) {
	return nil, ErrNotSupported
}

// SubmitTask returns ErrNotSupported by default.
// Override this method in channels that use an async task API.
func (b *BaseChannel) SubmitTask(_ context.Context, _ *model.GoogleRequest, _ string) (string, string, error) {
	return "", "", ErrNotSupported
}

// PollTask returns ErrNotSupported by default.
// Override this method in channels that use an async task API.
func (b *BaseChannel) PollTask(_ context.Context, _, _ string) (*model.GoogleResponse, error) {
	return nil, ErrNotSupported
}

// Probe performs a lightweight health check by issuing a GET to BaseURL.
// Override this method to use a channel-specific probe endpoint.
func (b *BaseChannel) Probe(account Account) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.BaseURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+account.APIKey())
	resp, err := b.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// GetAccountPool implements ChannelWithPool so HealthReaper can probe
// unhealthy accounts automatically.
func (b *BaseChannel) GetAccountPool() AccountPool { return b.Pool }

// --- Admin helper methods ---
// These are not part of the core.Channel interface but are discovered via
// type assertion in admin_handler.go.

// ListAccounts returns a summary of all accounts for the admin API.
func (b *BaseChannel) ListAccounts() []map[string]any {
	if b.Pool == nil {
		return nil
	}
	accounts := b.Pool.ListRaw()
	result := make([]map[string]any, len(accounts))
	for i, acc := range accounts {
		status := "healthy"
		if !acc.IsHealthy() {
			status = "unhealthy"
		} else if acc.HealthScore() < 0.6 {
			status = "degraded"
		}
		// Mask API key: show first 4 and last 4 characters
		apiKey := acc.APIKey()
		maskedKey := apiKey
		if len(apiKey) > 8 {
			maskedKey = apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
		} else if len(apiKey) > 4 {
			maskedKey = apiKey[:4] + "..."
		}
		result[i] = map[string]any{
			"api_key":              maskedKey,
			"weight":               acc.Weight(),
			"status":               status,
			"usage_count":          acc.UsageCount(),
			"health_score":         acc.HealthScore(),
			"consecutive_failures": acc.ConsecutiveFailures(),
		}
	}
	return result
}

// ResetAccount clears failure counters for the named account.
func (b *BaseChannel) ResetAccount(apiKey string) bool {
	if b.Pool == nil {
		return false
	}
	for _, acc := range b.Pool.List() {
		if acc.APIKey() == apiKey {
			acc.RecordSuccess()
			return true
		}
	}
	return false
}

// RetireAccount removes an account from the pool permanently.
func (b *BaseChannel) RetireAccount(apiKey string) bool {
	if b.Pool == nil {
		return false
	}
	return b.Pool.Remove(apiKey)
}

// SetWeight updates the routing weight of a specific account.
func (b *BaseChannel) SetWeight(apiKey string, w int) bool {
	if b.Pool == nil {
		return false
	}
	return b.Pool.SetWeight(apiKey, w)
}

// ProbeAccount runs a health probe for the named account.
func (b *BaseChannel) ProbeAccount(apiKey string) bool {
	if b.Pool == nil {
		return false
	}
	for _, acc := range b.Pool.List() {
		if acc.APIKey() == apiKey {
			return b.Probe(acc)
		}
	}
	return false
}
