package core

import (
	"math"
	"sync"
	"time"
)

const (
	failureDecay       = 0.2
	failureThreshold    = 5
	maxHealth           = 1.0
	minHealth           = 0.0
)

// HealthTracker records per-channel success/failure/latency.
type HealthTracker struct {
	mu            sync.RWMutex
	consecutive   map[string]int
	totalFail     map[string]int
	totalSuccess  map[string]int
	latencies     map[string][]time.Duration
	health        map[string]float64
}

func NewHealthTracker() *HealthTracker {
	return &HealthTracker{
		consecutive:  make(map[string]int),
		totalFail:    make(map[string]int),
		totalSuccess: make(map[string]int),
		latencies:    make(map[string][]time.Duration),
		health:       make(map[string]float64),
	}
}

func (h *HealthTracker) RecordFailure(channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.consecutive[channel]++
	h.totalFail[channel]++
	h.recalc(channel)
}

func (h *HealthTracker) RecordSuccess(channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.consecutive[channel] = 0
	h.totalSuccess[channel]++
	h.recalc(channel)
}

func (h *HealthTracker) RecordLatency(channel string, d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.latencies[channel] = append(h.latencies[channel], d)
	if len(h.latencies[channel]) > 100 {
		h.latencies[channel] = h.latencies[channel][1:]
	}
}

func (h *HealthTracker) HealthScore(channel string) float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	score, exists := h.health[channel]
	if !exists {
		return maxHealth
	}
	return score
}

func (h *HealthTracker) AverageLatency(channel string) time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	lats := h.latencies[channel]
	if len(lats) == 0 {
		return 0
	}
	var sum int64
	for _, d := range lats {
		sum += int64(d)
	}
	return time.Duration(sum / int64(len(lats)))
}

func (h *HealthTracker) IsHealthy(channel string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	// A channel that has no health record is considered healthy
	score, exists := h.health[channel]
	if !exists {
		return true
	}
	return score >= 0.5 && h.consecutive[channel] < failureThreshold
}

func (h *HealthTracker) TotalStats(channel string) (fail, success int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.totalFail[channel], h.totalSuccess[channel]
}

func (h *HealthTracker) recalc(channel string) {
	fail := h.totalFail[channel]
	success := h.totalSuccess[channel]
	total := fail + success
	if total == 0 {
		h.health[channel] = maxHealth
		return
	}
	ratio := float64(fail) / float64(total)
	consecutive := h.consecutive[channel]
	h.health[channel] = math.Max(minHealth, math.Min(maxHealth,
		1.0 - (ratio*0.5) - (float64(consecutive)*0.1)))
}