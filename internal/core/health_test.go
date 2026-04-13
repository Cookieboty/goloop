package core

import (
	"testing"
	"time"
)

func TestHealthTracker(t *testing.T) {
	ht := NewHealthTracker()

	if !ht.IsHealthy("ch1") {
		t.Errorf("ch1 should be healthy initially")
	}

	ht.RecordFailure("ch1")
	ht.RecordFailure("ch1")
	if ht.HealthScore("ch1") >= 0.7 {
		t.Errorf("health should drop after failures")
	}

	ht.RecordSuccess("ch1")
	if ht.HealthScore("ch1") < 0.5 {
		t.Errorf("health should improve after success")
	}

	// 5 consecutive failures → unhealthy
	for i := 0; i < 6; i++ {
		ht.RecordFailure("ch2")
	}
	if ht.IsHealthy("ch2") {
		t.Errorf("ch2 should be unhealthy after 6 failures")
	}

	// Latency tracking
	ht.RecordLatency("ch1", 200*time.Millisecond)
	ht.RecordLatency("ch1", 400*time.Millisecond)
	avg := ht.AverageLatency("ch1")
	if avg < 250*time.Millisecond || avg > 350*time.Millisecond {
		t.Errorf("average latency out of range: got %v", avg)
	}
}

func TestHealthTracker_LastFailureTime(t *testing.T) {
	ht := NewHealthTracker()

	// No failure recorded yet — zero time.
	if !ht.LastFailureTime("ch1").IsZero() {
		t.Errorf("expected zero time before any failure")
	}

	before := time.Now()
	ht.RecordFailure("ch1")
	after := time.Now()

	ts := ht.LastFailureTime("ch1")
	if ts.Before(before) || ts.After(after) {
		t.Errorf("LastFailureTime %v not in expected range [%v, %v]", ts, before, after)
	}

	// Success should not clear the last-failure timestamp.
	ht.RecordSuccess("ch1")
	if ht.LastFailureTime("ch1").IsZero() {
		t.Errorf("LastFailureTime should persist after success")
	}
}

func TestHealthTracker_ResetHealthTo(t *testing.T) {
	ht := NewHealthTracker()

	// Drive channel into hard-stop territory.
	for i := 0; i < 10; i++ {
		ht.RecordFailure("ch1")
	}
	if ht.HealthScore("ch1") >= hardStopThreshold {
		t.Fatalf("expected health below hardStopThreshold after 10 failures, got %f", ht.HealthScore("ch1"))
	}

	// Reset to 50%.
	ht.ResetHealthTo("ch1", 0.5)

	if score := ht.HealthScore("ch1"); score != 0.5 {
		t.Errorf("expected health 0.5 after reset, got %f", score)
	}

	// Consecutive failures should be cleared.
	if ht.consecutive["ch1"] != 0 {
		t.Errorf("expected consecutive=0 after reset, got %d", ht.consecutive["ch1"])
	}

	// LastFailureTime should be cleared so the recovery timer won't fire again immediately.
	if !ht.LastFailureTime("ch1").IsZero() {
		t.Errorf("expected LastFailureTime to be zero after reset")
	}

	// Score should be clamped to [0, 1].
	ht.ResetHealthTo("ch1", 1.5)
	if ht.HealthScore("ch1") != 1.0 {
		t.Errorf("expected clamped score 1.0, got %f", ht.HealthScore("ch1"))
	}
	ht.ResetHealthTo("ch1", -0.5)
	if ht.HealthScore("ch1") != 0.0 {
		t.Errorf("expected clamped score 0.0, got %f", ht.HealthScore("ch1"))
	}
}

func TestHealthTracker_RecordFailure_SetsLastFailure(t *testing.T) {
	ht := NewHealthTracker()

	t1 := time.Now()
	ht.RecordFailure("ch1")
	t2 := time.Now()

	ts := ht.LastFailureTime("ch1")
	if ts.Before(t1) || ts.After(t2) {
		t.Errorf("LastFailureTime %v not between %v and %v", ts, t1, t2)
	}

	// Second failure updates the timestamp.
	time.Sleep(5 * time.Millisecond)
	ht.RecordFailure("ch1")
	ts2 := ht.LastFailureTime("ch1")
	if !ts2.After(ts) {
		t.Errorf("second failure should update LastFailureTime: first=%v second=%v", ts, ts2)
	}
}
