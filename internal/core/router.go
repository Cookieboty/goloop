package core

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
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

// RouteForModel selects a healthy channel for the given model.
// If the context carries a channel restriction (set by JWT middleware), only
// that channel will be considered.
func (r *Router) RouteForModel(ctx context.Context, modelName string) (Channel, error) {
	// Honor JWT channel restriction if present.
	if restricted, ok := ChannelRestrictionFromContext(ctx); ok {
		ch, found := r.reg.Get(restricted)
		if !found {
			return nil, fmt.Errorf("router: restricted channel %q not found", restricted)
		}
		if !ch.IsAvailable() {
			return nil, fmt.Errorf("router: restricted channel %q is not available", restricted)
		}
		return ch, nil
	}

	return r.route()
}

// route selects a healthy channel using weighted random selection across all channels.
func (r *Router) route() (Channel, error) {
	channels := r.reg.List()

	var candidates []Channel
	var weights []int
	var totalWeight int

	for _, ch := range channels {
		if !ch.IsAvailable() {
			continue
		}
		if r.health.HealthScore(ch.Name()) <= 0 {
			continue
		}
		weight := ch.Weight()
		if weight <= 0 {
			weight = 1
		}
		candidates = append(candidates, ch)
		weights = append(weights, weight)
		totalWeight += weight
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
