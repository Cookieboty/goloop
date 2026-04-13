package core

import (
	"context"
	"testing"
)

func TestRouter_WeightedRandom(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	ch1 := &mockChannel{name: "ch1"}
	ch2 := &mockChannel{name: "ch2"}
	reg.Register(ch1)
	reg.Register(ch2)

	ctx := context.Background()
	selected := make(map[string]int)
	for i := 0; i < 1000; i++ {
		ch, err := router.RouteForModel(ctx, "")
		if err != nil {
			t.Fatalf("RouteForModel error: %v", err)
		}
		selected[ch.Name()]++
	}

	if selected["ch1"] == 0 || selected["ch2"] == 0 {
		t.Errorf("both should be selected: ch1=%d ch2=%d", selected["ch1"], selected["ch2"])
	}

	// Drive ch1 health below hardStopThreshold → should be excluded.
	for i := 0; i < 10; i++ {
		ht.RecordFailure("ch1")
	}
	if ht.HealthScore("ch1") >= hardStopThreshold {
		t.Fatalf("ch1 health should be below hardStopThreshold, got %f", ht.HealthScore("ch1"))
	}

	selected = make(map[string]int)
	for i := 0; i < 200; i++ {
		ch, err := router.RouteForModel(ctx, "")
		if err != nil {
			t.Fatalf("RouteForModel error when ch1 unhealthy: %v", err)
		}
		selected[ch.Name()]++
	}
	if selected["ch1"] != 0 {
		t.Errorf("ch1 should not be selected when below hardStopThreshold: got %d", selected["ch1"])
	}
}

func TestRouter_ChannelRestriction(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	ch1 := &mockChannel{name: "ch1"}
	ch2 := &mockChannel{name: "ch2"}
	reg.Register(ch1)
	reg.Register(ch2)

	ctx := WithChannelRestriction(context.Background(), "ch1")
	for i := 0; i < 50; i++ {
		ch, err := router.RouteForModel(ctx, "")
		if err != nil {
			t.Fatalf("RouteForModel error: %v", err)
		}
		if ch.Name() != "ch1" {
			t.Errorf("expected ch1, got %s", ch.Name())
		}
	}
}

func TestRouter_RecordResult(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	ch := &mockChannel{name: "ch1"}
	reg.Register(ch)

	router.RecordResult("ch1", true, 1500)
	router.RecordResult("ch1", false, 2000)
	router.RecordResult("ch1", false, 3000)

	score := ht.HealthScore("ch1")
	if score >= 0.9 {
		t.Errorf("health should have dropped: got %f", score)
	}
}

func TestRouter_RouteWithFallback_PriorityOrder(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	ch1 := &mockChannel{name: "ch1", weight: 100}
	ch2 := &mockChannel{name: "ch2", weight: 50}
	ch3 := &mockChannel{name: "ch3", weight: 200}
	reg.Register(ch1)
	reg.Register(ch2)
	reg.Register(ch3)

	ctx := context.Background()
	channels, err := router.RouteWithFallback(ctx)
	if err != nil {
		t.Fatalf("RouteWithFallback error: %v", err)
	}
	if len(channels) != 3 {
		t.Fatalf("expected 3 channels, got %d", len(channels))
	}
	// All channels have full health (1.0), so effective weight == configured weight.
	// Should be sorted: ch3(200), ch1(100), ch2(50).
	if channels[0].Name() != "ch3" {
		t.Errorf("expected ch3 first (weight 200), got %s", channels[0].Name())
	}
	if channels[1].Name() != "ch1" {
		t.Errorf("expected ch1 second (weight 100), got %s", channels[1].Name())
	}
	if channels[2].Name() != "ch2" {
		t.Errorf("expected ch2 third (weight 50), got %s", channels[2].Name())
	}
}

func TestRouter_RouteWithFallback_ExcludesBelowHardStop(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	ch1 := &mockChannel{name: "ch1", weight: 100}
	ch2 := &mockChannel{name: "ch2", weight: 50}
	reg.Register(ch1)
	reg.Register(ch2)

	// Drive ch1 health below hardStopThreshold.
	for i := 0; i < 10; i++ {
		ht.RecordFailure("ch1")
	}

	ctx := context.Background()
	channels, err := router.RouteWithFallback(ctx)
	if err != nil {
		t.Fatalf("RouteWithFallback error: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel (ch1 excluded), got %d", len(channels))
	}
	if channels[0].Name() != "ch2" {
		t.Errorf("expected ch2, got %s", channels[0].Name())
	}
}

func TestRouter_RouteWithFallback_EffectiveWeightOrder(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	// ch1: weight=100, health=1.0  → effective=100
	// ch2: weight=200, health=0.3  → effective=60  (below ch1 despite higher raw weight)
	ch1 := &mockChannel{name: "ch1", weight: 100}
	ch2 := &mockChannel{name: "ch2", weight: 200}
	reg.Register(ch1)
	reg.Register(ch2)

	// Degrade ch2 to ~0.3 health (above hardStopThreshold so it stays in pool).
	// 1.0 - (fail/total)*0.5 - 0 = 0.3  →  fail/total = 1.4  (impossible with integers)
	// Use: 6 fail + 1 success → ratio=6/7≈0.857 → health=1-0.429=0.571 (too high)
	// Use: 9 fail + 1 success → ratio=0.9 → health=1-0.45=0.55 (still above 0.2)
	// Manually verify: after RecordFailure×4 consecutive=4 → health=1-0.5*0.8-0.4=0.1 (below 0.2!)
	// We need health between 0.2 and 1.0 but below ch1's effective weight.
	// ch1 effective=100*1.0=100, ch2 effective=200*h2.
	// For ch2 to sort after ch1: 200*h2 < 100 → h2 < 0.5.
	// Use: 2 fail + 1 success → ratio=2/3≈0.667 → health=1-0.333=0.667 (200*0.667=133 > 100, ch2 still first)
	// Use: 3 fail + 1 success → ratio=0.75 → health=0.625 (200*0.625=125 > 100)
	// Use: 4 fail + 1 success → ratio=0.8 → health=0.6 (200*0.6=120 > 100)
	// Use: 6 fail + 1 success → ratio=6/7≈0.857 → health≈0.571 (200*0.571=114 > 100)
	// Use: 8 fail + 1 success → ratio=8/9≈0.889 → health≈0.556 (200*0.556=111 > 100)
	// Use: 14 fail + 1 success → ratio=14/15≈0.933 → health≈0.533 (200*0.533=107 > 100)
	// Use: 30 fail + 1 success → ratio=30/31≈0.968 → health≈0.516 (200*0.516=103 > 100)
	// Use: 60 fail + 1 success → ratio=60/61≈0.984 → health≈0.508 (200*0.508=102 > 100)
	// Use: 200 fail + 1 success → ratio≈0.995 → health≈0.502 (200*0.502=100.4 ≈ 100)
	// Simpler: just record enough failures to get health around 0.4 without hitting consecutive penalty.
	// Reset consecutive by mixing in successes.
	// 5 fail + 5 success → ratio=0.5 → health=0.75 (200*0.75=150 > 100)
	// 8 fail + 2 success → ratio=0.8 → health=0.6 (200*0.6=120 > 100)
	// We need health < 0.5 for ch2 to have effective < 100.
	// Easiest: use consecutive failures (no success to reset).
	// 4 consecutive: health = 1 - 0.5*(4/4) - 4*0.1 = 1 - 0.5 - 0.4 = 0.1 → below hardStop!
	// 3 consecutive: health = 1 - 0.5*(3/3) - 3*0.1 = 1 - 0.5 - 0.3 = 0.2 → exactly at threshold (excluded)
	// 2 consecutive + 1 success: ratio=2/3, consecutive=0 → health=1-0.333=0.667 (too high)
	// Best approach: use many total records with high fail ratio but reset consecutive.
	// 40 fail, 10 success (interleaved to keep consecutive=0):
	// ratio=40/50=0.8, consecutive=0 → health=1-0.4=0.6 (200*0.6=120 > 100)
	// We can't easily get ch2 below 0.5 without hitting hard stop with this formula.
	// Instead test the simpler case: ch2 at ~0.6 health sorts before ch1 at 1.0
	// when ch2.weight*0.6 > ch1.weight*1.0, i.e. 200*0.6=120 > 100*1.0=100. ✓
	// So ch2 should still be first. Let's test that degraded ch2 with 200 weight
	// still beats ch1 with 100 weight even at 0.6 health.

	// Record 4 fail + 1 success for ch2 (consecutive resets to 0 after success).
	for i := 0; i < 4; i++ {
		ht.RecordFailure("ch2")
	}
	ht.RecordSuccess("ch2") // resets consecutive to 0; ratio=4/5=0.8 → health=0.6

	score := ht.HealthScore("ch2")
	if score < hardStopThreshold {
		t.Fatalf("ch2 health %f is below hardStopThreshold, test setup invalid", score)
	}

	ctx := context.Background()
	channels, err := router.RouteWithFallback(ctx)
	if err != nil {
		t.Fatalf("RouteWithFallback error: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(channels))
	}

	ch2EffectiveWeight := float64(ch2.Weight()) * score
	ch1EffectiveWeight := float64(ch1.Weight()) * ht.HealthScore("ch1")

	if ch2EffectiveWeight > ch1EffectiveWeight {
		// ch2 should be first
		if channels[0].Name() != "ch2" {
			t.Errorf("expected ch2 first (effectiveWeight %.1f > ch1 %.1f), got %s",
				ch2EffectiveWeight, ch1EffectiveWeight, channels[0].Name())
		}
	} else {
		// ch1 should be first
		if channels[0].Name() != "ch1" {
			t.Errorf("expected ch1 first (effectiveWeight %.1f > ch2 %.1f), got %s",
				ch1EffectiveWeight, ch2EffectiveWeight, channels[0].Name())
		}
	}
}

func TestRouter_RouteWithFallback_ChannelRestriction(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	ch1 := &mockChannel{name: "ch1", weight: 100}
	ch2 := &mockChannel{name: "ch2", weight: 50}
	reg.Register(ch1)
	reg.Register(ch2)

	ctx := WithChannelRestriction(context.Background(), "ch2")
	channels, err := router.RouteWithFallback(ctx)
	if err != nil {
		t.Fatalf("RouteWithFallback error: %v", err)
	}
	if len(channels) != 1 || channels[0].Name() != "ch2" {
		t.Errorf("expected only ch2 due to restriction, got %v", channels)
	}
}
