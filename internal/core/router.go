package core

import (
	"context"
	"errors"
	"fmt"
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
		return nil, errors.New("router: no healthy channels available")
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
