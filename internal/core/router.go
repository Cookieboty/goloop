package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"time"
)

type routerContextKey string

const ChannelRestrictionKey routerContextKey = "channel_restriction"

// WithChannelRestriction returns a context that restricts routing to a specific channel.
func WithChannelRestriction(ctx context.Context, channelName string) context.Context {
	return context.WithValue(ctx, ChannelRestrictionKey, channelName)
}

// ChannelRestrictionFromContext returns the channel restriction from the context, if any.
func ChannelRestrictionFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ChannelRestrictionKey).(string)
	return v, ok && v != ""
}

// Router selects the best channel using weighted random + health awareness.
type Router struct {
	reg    *PluginRegistry
	health *HealthTracker
}

func NewRouter(reg *PluginRegistry, health *HealthTracker) *Router {
	return &Router{reg: reg, health: health}
}

// hardStopThreshold is the health score below which a channel is excluded from routing.
const hardStopThreshold = 0.2

// ChannelTypeFilter defines channel type filtering conditions.
type ChannelTypeFilter struct {
	Include []string // Only include these types (empty means no restriction)
	Exclude []string // Exclude these types
}

// matches reports whether the given channel type passes this filter.
// A nil filter matches everything.
func (f *ChannelTypeFilter) matches(chType string) bool {
	if f == nil {
		return true
	}
	for _, ex := range f.Exclude {
		if ex == chType {
			return false
		}
	}
	if len(f.Include) == 0 {
		return true
	}
	for _, in := range f.Include {
		if in == chType {
			return true
		}
	}
	return false
}

// pickLowestWeightFallback returns the single lowest-weight available channel
// that passes the filter, or nil if none qualifies. Health score is ignored —
// this is a last-resort pick used when no channel is healthy enough to route
// to normally.
func (r *Router) pickLowestWeightFallback(filter *ChannelTypeFilter) Channel {
	var best Channel
	for _, ch := range r.reg.List() {
		if !ch.IsAvailable() {
			continue
		}
		if !filter.matches(ch.Type()) {
			continue
		}
		if best == nil || ch.Weight() < best.Weight() {
			best = ch
		}
	}
	return best
}

// RouteWithFallback returns all healthy channels sorted by effective weight descending.
// Effective weight = configured weight * health score, so degraded channels receive
// proportionally less traffic during recovery.
// Channels with HealthScore < hardStopThreshold or IsAvailable() == false are excluded.
// If the context carries a JWT channel restriction, only that channel is returned.
func (r *Router) RouteWithFallback(ctx context.Context) ([]Channel, error) {
	// Honor JWT channel restriction if present.
	if restricted, ok := ChannelRestrictionFromContext(ctx); ok {
		ch, found := r.reg.Get(restricted)
		if !found {
			return nil, fmt.Errorf("router: restricted channel %q not found", restricted)
		}
		if !ch.IsAvailable() {
			return nil, fmt.Errorf("router: restricted channel %q is not available", restricted)
		}
		return []Channel{ch}, nil
	}

	all := r.reg.List()
	var candidates []Channel
	for _, ch := range all {
		if !ch.IsAvailable() {
			continue
		}
		if r.health.HealthScore(ch.Name()) < hardStopThreshold {
			continue
		}
		candidates = append(candidates, ch)
	}

	if len(candidates) == 0 {
		// Degraded fallback: no channel passes hardStopThreshold. Instead
		// of 503, route this request to the lowest-weight available channel
		// (operator-configured last-resort). Health may recover on a later
		// request; if not, each subsequent request reuses the same fallback.
		if fallback := r.pickLowestWeightFallback(nil); fallback != nil {
			slog.Warn("router: no healthy channels, using lowest-weight fallback",
				"channel", fallback.Name(), "weight", fallback.Weight())
			return []Channel{fallback}, nil
		}
		return nil, errors.New("router: no available channels")
	}

	// Sort by effective weight descending: weight * healthScore.
	// Fully healthy channels (score=1.0) keep their full weight; recovering
	// channels (score=0.5) get half weight, limiting their traffic share.
	sort.Slice(candidates, func(i, j int) bool {
		wi := float64(candidates[i].Weight()) * r.health.HealthScore(candidates[i].Name())
		wj := float64(candidates[j].Weight()) * r.health.HealthScore(candidates[j].Name())
		return wi > wj
	})

	return candidates, nil
}

// RouteWithTypeFilter returns all healthy channels filtered by type, sorted by effective weight descending.
// If filter is nil, behaves like RouteWithFallback (no type filtering).
// If the context carries a JWT channel restriction, only that channel is returned (ignoring type filter).
func (r *Router) RouteWithTypeFilter(ctx context.Context, filter *ChannelTypeFilter) ([]Channel, error) {
	// Honor JWT channel restriction if present (takes precedence over type filtering).
	if restricted, ok := ChannelRestrictionFromContext(ctx); ok {
		ch, found := r.reg.Get(restricted)
		if !found {
			return nil, fmt.Errorf("router: restricted channel %q not found", restricted)
		}
		if !ch.IsAvailable() {
			return nil, fmt.Errorf("router: restricted channel %q is not available", restricted)
		}
		return []Channel{ch}, nil
	}

	all := r.reg.List()
	var candidates []Channel
	for _, ch := range all {
		if !ch.IsAvailable() {
			continue
		}
		if !filter.matches(ch.Type()) {
			continue
		}
		if r.health.HealthScore(ch.Name()) < hardStopThreshold {
			continue
		}
		candidates = append(candidates, ch)
	}

	if len(candidates) == 0 {
		// Degraded fallback: no channel matching the filter passes
		// hardStopThreshold. Route to the lowest-weight available channel
		// matching the filter so the request isn't hard-failed as 503.
		if fallback := r.pickLowestWeightFallback(filter); fallback != nil {
			args := []any{
				"channel", fallback.Name(),
				"weight", fallback.Weight(),
			}
			if filter != nil {
				args = append(args, "include", filter.Include, "exclude", filter.Exclude)
			}
			slog.Warn("router: no healthy channels matching filter, using lowest-weight fallback", args...)
			return []Channel{fallback}, nil
		}
		return nil, errors.New("router: no available channels matching filter")
	}

	// Sort by effective weight descending: weight * healthScore.
	sort.Slice(candidates, func(i, j int) bool {
		wi := float64(candidates[i].Weight()) * r.health.HealthScore(candidates[i].Name())
		wj := float64(candidates[j].Weight()) * r.health.HealthScore(candidates[j].Name())
		return wi > wj
	})

	return candidates, nil
}

// RouteForModel returns the highest-priority healthy channel.
// Kept for backward compatibility; prefer RouteWithFallback for new code.
func (r *Router) RouteForModel(ctx context.Context, modelName string) (Channel, error) {
	channels, err := r.RouteWithFallback(ctx)
	if err != nil {
		return nil, err
	}
	return channels[0], nil
}

// route selects a healthy channel using weighted random selection across all channels.
// Uses effective weight (configured weight * health score) so degraded channels
// receive proportionally less traffic. Retained for internal use by legacy callers.
func (r *Router) route() (Channel, error) {
	channels := r.reg.List()

	var candidates []Channel
	var weights []int
	var totalWeight int

	for _, ch := range channels {
		if !ch.IsAvailable() {
			continue
		}
		score := r.health.HealthScore(ch.Name())
		if score < hardStopThreshold {
			continue
		}
		effectiveWeight := int(float64(ch.Weight()) * score)
		if effectiveWeight <= 0 {
			effectiveWeight = 1
		}
		candidates = append(candidates, ch)
		weights = append(weights, effectiveWeight)
		totalWeight += effectiveWeight
	}

	if len(candidates) == 0 {
		return nil, errors.New("router: no healthy channels available")
	}

	n := rand.Intn(totalWeight)
	var cumulative int
	for i, ch := range candidates {
		cumulative += weights[i]
		if n < cumulative {
			return ch, nil
		}
	}
	return candidates[len(candidates)-1], nil
}

// RecordResult updates health based on call outcome.
func (r *Router) RecordResult(channel string, success bool, latencyMs int64) {
	if success {
		r.health.RecordSuccess(channel)
	} else {
		r.health.RecordFailure(channel)
	}
	r.health.RecordLatency(channel, time.Duration(latencyMs*1e6))
}
