package core

import (
	"testing"
	"time"
)

func TestHealthReaper_DoesNotProbeHealthy(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	reaper := NewHealthReaper(reg, ht, 1*time.Second, 30*time.Minute)

	ch := &mockChannel{name: "ch1"}
	reg.Register(ch)

	reaper.Start()

	// Wait 2 probe cycles
	time.Sleep(2500 * time.Millisecond)

	reaper.Stop()

	// reaper should not panic and channel should remain healthy
	if !ht.IsHealthy("ch1") {
		t.Errorf("healthy channel should stay healthy")
	}
}

func TestHealthReaper_RecoverStoppedChannels(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	// Use a very short recovery interval so the test doesn't have to wait 30 minutes.
	recoveryInterval := 100 * time.Millisecond
	reaper := NewHealthReaper(reg, ht, 200*time.Millisecond, recoveryInterval)

	ch := &mockChannel{name: "ch1"}
	reg.Register(ch)

	// Drive ch1 below hardStopThreshold.
	for i := 0; i < 10; i++ {
		ht.RecordFailure("ch1")
	}
	if ht.HealthScore("ch1") >= hardStopThreshold {
		t.Fatalf("expected ch1 health below hardStopThreshold, got %f", ht.HealthScore("ch1"))
	}

	// Wait for the recovery interval to elapse, then start the reaper.
	time.Sleep(recoveryInterval + 50*time.Millisecond)

	reaper.Start()
	// Wait for at least one ticker cycle.
	time.Sleep(400 * time.Millisecond)
	reaper.Stop()

	score := ht.HealthScore("ch1")
	if score < 0.5 {
		t.Errorf("expected ch1 health to be reset to 0.5, got %f", score)
	}
}

func TestHealthReaper_DoesNotRecoverTooEarly(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	// Long recovery interval — channel should NOT be recovered within the test.
	reaper := NewHealthReaper(reg, ht, 100*time.Millisecond, 10*time.Minute)

	ch := &mockChannel{name: "ch1"}
	reg.Register(ch)

	// Drive ch1 below hardStopThreshold.
	for i := 0; i < 10; i++ {
		ht.RecordFailure("ch1")
	}
	prevScore := ht.HealthScore("ch1")
	if prevScore >= hardStopThreshold {
		t.Fatalf("expected ch1 health below hardStopThreshold, got %f", prevScore)
	}

	reaper.Start()
	// Run for a few ticks — recovery interval is 10 minutes so nothing should happen.
	time.Sleep(350 * time.Millisecond)
	reaper.Stop()

	score := ht.HealthScore("ch1")
	if score != prevScore {
		t.Errorf("channel should not have recovered yet: before=%f after=%f", prevScore, score)
	}
}
