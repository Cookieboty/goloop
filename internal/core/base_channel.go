package core

import (
	"context"
	"net/http"
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
	BaseURL     string
	HTTPClient  *http.Client
	Pool        *DefaultAccountPool
	weight      int64 // accessed via atomic-style helpers; stored as plain int64
	// weight is not atomic.Int64 so BaseChannel can be value-copied in NewBaseChannel.
	// Concrete channels should call SetChannelWeight/Weight via the pointer receiver.
	weightMu weightHolder
}

// weightHolder wraps the weight so BaseChannel can be returned by value from
// NewBaseChannel while still allowing atomic-like updates.
type weightHolder struct {
	mu  interface{} // unused placeholder; weight updates use BaseChannel.weightMu
	val int
}

// NewBaseChannel constructs a BaseChannel with the given parameters.
func NewBaseChannel(name, baseURL string, weight int, pool *DefaultAccountPool, timeout time.Duration) BaseChannel {
	return BaseChannel{
		ChannelName: name,
		BaseURL:     baseURL,
		HTTPClient:  &http.Client{Timeout: timeout},
		Pool:        pool,
		weightMu:    weightHolder{val: weight},
	}
}

// Name returns the channel's registered name.
func (b *BaseChannel) Name() string { return b.ChannelName }

// Weight returns the channel's routing weight (priority).
func (b *BaseChannel) Weight() int { return b.weightMu.val }

// SetChannelWeight updates the channel's routing weight at runtime.
func (b *BaseChannel) SetChannelWeight(w int) { b.weightMu.val = w }

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
		result[i] = map[string]any{
			"api_key":              acc.APIKey(),
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
