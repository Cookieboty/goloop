package core

import (
	"errors"
	"math/rand"
	"sync"
	"time"
)

// Router selects the best channel using weighted random + health awareness.
type Router struct {
	reg    *PluginRegistry
	health *HealthTracker
	mu     sync.RWMutex
}

func NewRouter(reg *PluginRegistry, health *HealthTracker) *Router {
	return &Router{reg: reg, health: health}
}

// Route selects a healthy channel using weighted random selection.
func (r *Router) Route() (Channel, error) {
	channels := r.reg.List()

	var candidates []Channel
	var weights []int
	var totalWeight int

	for _, ch := range channels {
		if !ch.IsAvailable() {
			continue
		}
		score := r.health.HealthScore(ch.Name())
		if score <= 0 {
			continue
		}
		weight := int(score * 100)
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

// RouteForModel routes for a specific model (currently just delegates to Route).
func (r *Router) RouteForModel(modelName string) (Channel, error) {
	return r.Route()
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
