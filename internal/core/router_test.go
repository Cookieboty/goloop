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

	// ch1 unhealthy → should be excluded
	for i := 0; i < 6; i++ {
		ht.RecordFailure("ch1")
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
		t.Errorf("ch1 should not be selected when unhealthy: got %d", selected["ch1"])
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
