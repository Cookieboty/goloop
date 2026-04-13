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
	// Should be sorted by weight descending: ch3(200), ch1(100), ch2(50)
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

func TestRouter_RouteWithFallback_ExcludesUnhealthy(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	ch1 := &mockChannel{name: "ch1", weight: 100}
	ch2 := &mockChannel{name: "ch2", weight: 50}
	reg.Register(ch1)
	reg.Register(ch2)

	// Drive ch1 health to 0
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
