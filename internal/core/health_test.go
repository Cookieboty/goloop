package core

import (
	"testing"
	"time"
)

func TestHealthTracker(t *testing.T) {
	ht := NewHealthTracker()

	if !ht.IsHealthy("ch1") { t.Errorf("ch1 should be healthy initially") }

	ht.RecordFailure("ch1")
	ht.RecordFailure("ch1")
	if ht.HealthScore("ch1") >= 0.7 { t.Errorf("health should drop after failures") }

	ht.RecordSuccess("ch1")
	if ht.HealthScore("ch1") < 0.5 { t.Errorf("health should improve after success") }

	// 5 consecutive failures → unhealthy
	for i := 0; i < 6; i++ {
		ht.RecordFailure("ch2")
	}
	if ht.IsHealthy("ch2") { t.Errorf("ch2 should be unhealthy after 6 failures") }

	// Latency tracking
	ht.RecordLatency("ch1", 200*time.Millisecond)
	ht.RecordLatency("ch1", 400*time.Millisecond)
	avg := ht.AverageLatency("ch1")
	if avg < 250*time.Millisecond || avg > 350*time.Millisecond {
		t.Errorf("average latency out of range: got %v", avg)
	}
}