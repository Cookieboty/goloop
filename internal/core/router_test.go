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

	reg.Register(&mockChannel{name: "ch1"})
	reg.Register(&mockChannel{name: "ch2"})

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

	reg.Register(&mockChannel{name: "ch1"})

	router.RecordResult("ch1", true, 1500)
	router.RecordResult("ch1", false, 2000)
	router.RecordResult("ch1", false, 3000)

	if score := ht.HealthScore("ch1"); score >= 0.9 {
		t.Errorf("health should have dropped: got %f", score)
	}
}

func TestRouter_RouteWithFallback_PriorityOrder(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	reg.Register(&mockChannel{name: "ch1", weight: 100})
	reg.Register(&mockChannel{name: "ch2", weight: 50})
	reg.Register(&mockChannel{name: "ch3", weight: 200})

	channels, err := router.RouteWithFallback(context.Background())
	if err != nil {
		t.Fatalf("RouteWithFallback error: %v", err)
	}
	if len(channels) != 3 {
		t.Fatalf("expected 3 channels, got %d", len(channels))
	}
	// All healthy (1.0), effective weight == configured weight.
	// Expected order: ch3(200) > ch1(100) > ch2(50).
	want := []string{"ch3", "ch1", "ch2"}
	for i, name := range want {
		if channels[i].Name() != name {
			t.Errorf("position %d: got %s, want %s", i, channels[i].Name(), name)
		}
	}
}

func TestRouter_RouteWithFallback_ExcludesBelowHardStop(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	reg.Register(&mockChannel{name: "ch1", weight: 100})
	reg.Register(&mockChannel{name: "ch2", weight: 50})

	for i := 0; i < 10; i++ {
		ht.RecordFailure("ch1")
	}

	channels, err := router.RouteWithFallback(context.Background())
	if err != nil {
		t.Fatalf("RouteWithFallback error: %v", err)
	}
	if len(channels) != 1 || channels[0].Name() != "ch2" {
		t.Fatalf("expected only ch2, got %v", channels)
	}
}

func TestRouter_RouteWithFallback_ChannelRestriction(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	reg.Register(&mockChannel{name: "ch1", weight: 100})
	reg.Register(&mockChannel{name: "ch2", weight: 50})

	ctx := WithChannelRestriction(context.Background(), "ch2")
	channels, err := router.RouteWithFallback(ctx)
	if err != nil {
		t.Fatalf("RouteWithFallback error: %v", err)
	}
	if len(channels) != 1 || channels[0].Name() != "ch2" {
		t.Errorf("expected only ch2 due to restriction, got %v", channels)
	}
}

// --- RouteWithTypeFilter ---

func TestRouter_RouteWithTypeFilter_Include(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	reg.Register(&mockChannel{name: "g1", ctype: "gemini"})
	reg.Register(&mockChannel{name: "g2", ctype: "gemini"})
	reg.Register(&mockChannel{name: "o1", ctype: "gpt-image"})

	filter := &ChannelTypeFilter{Include: []string{"gpt-image"}}
	channels, err := router.RouteWithTypeFilter(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(channels) != 1 || channels[0].Name() != "o1" {
		t.Errorf("expected only o1, got %+v", names(channels))
	}
}

func TestRouter_RouteWithTypeFilter_Exclude(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	reg.Register(&mockChannel{name: "g1", ctype: "gemini"})
	reg.Register(&mockChannel{name: "k1", ctype: "kieai"})
	reg.Register(&mockChannel{name: "o1", ctype: "gpt-image"})

	filter := &ChannelTypeFilter{Exclude: []string{"gpt-image"}}
	channels, err := router.RouteWithTypeFilter(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := names(channels)
	for _, n := range got {
		if n == "o1" {
			t.Errorf("o1 (gpt-image) must be excluded: got %v", got)
		}
	}
	if len(channels) != 2 {
		t.Errorf("expected 2 channels (g1, k1), got %d: %v", len(channels), got)
	}
}

// Include takes precedence: exclude list is checked first, then include.
// A channel mentioned in both should be excluded.
func TestRouter_RouteWithTypeFilter_IncludeAndExclude(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	reg.Register(&mockChannel{name: "a", ctype: "gpt-image"})
	reg.Register(&mockChannel{name: "b", ctype: "gemini"})

	filter := &ChannelTypeFilter{
		Include: []string{"gpt-image", "gemini"},
		Exclude: []string{"gpt-image"},
	}
	channels, err := router.RouteWithTypeFilter(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(channels) != 1 || channels[0].Name() != "b" {
		t.Errorf("expected only b, got %v", names(channels))
	}
}

// nil filter must behave like RouteWithFallback (no type restriction).
func TestRouter_RouteWithTypeFilter_NilFilter(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	reg.Register(&mockChannel{name: "a", ctype: "gemini"})
	reg.Register(&mockChannel{name: "b", ctype: "gpt-image"})

	channels, err := router.RouteWithTypeFilter(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(channels) != 2 {
		t.Errorf("expected both channels, got %v", names(channels))
	}
}

// No channel of the requested type is registered at all → hard error.
// (Degraded fallback only kicks in when channels of the right type exist but
// are all unhealthy — registering zero gpt-image channels is a config error,
// not a runtime degradation.)
func TestRouter_RouteWithTypeFilter_NoMatchingType(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	reg.Register(&mockChannel{name: "g1", ctype: "gemini"})

	filter := &ChannelTypeFilter{Include: []string{"gpt-image"}}
	if _, err := router.RouteWithTypeFilter(context.Background(), filter); err == nil {
		t.Fatal("expected error when no channels match filter type")
	}
}

// JWT channel restriction currently wins over the type filter. This documents
// that behavior; if the product semantics change to "JWT × filter must
// intersect" this test will need to be updated.
func TestRouter_RouteWithTypeFilter_JWTRestrictionWinsOverFilter(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	reg.Register(&mockChannel{name: "g1", ctype: "gemini"})
	reg.Register(&mockChannel{name: "o1", ctype: "gpt-image"})

	// Restrict to g1 (gemini) but ask for gpt-image.
	ctx := WithChannelRestriction(context.Background(), "g1")
	filter := &ChannelTypeFilter{Include: []string{"gpt-image"}}

	channels, err := router.RouteWithTypeFilter(ctx, filter)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(channels) != 1 || channels[0].Name() != "g1" {
		t.Errorf("JWT restriction should pin to g1, got %v", names(channels))
	}
}

func TestRouter_RouteWithTypeFilter_ExcludesBelowHardStop(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	reg.Register(&mockChannel{name: "o1", ctype: "gpt-image", weight: 100})
	reg.Register(&mockChannel{name: "o2", ctype: "gpt-image", weight: 50})

	for i := 0; i < 10; i++ {
		ht.RecordFailure("o1")
	}

	filter := &ChannelTypeFilter{Include: []string{"gpt-image"}}
	channels, err := router.RouteWithTypeFilter(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(channels) != 1 || channels[0].Name() != "o2" {
		t.Errorf("expected only o2 (o1 hard-stopped), got %v", names(channels))
	}
}

// --- Degraded fallback ---

// When every registered channel has crashed below hardStopThreshold, the
// router must not 503. It picks the lowest-weight AVAILABLE channel as an
// operator-configured last resort.
func TestRouter_RouteWithFallback_DegradedPicksLowestWeight(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	// weight=100 (primary), weight=10 (last-resort by operator convention)
	reg.Register(&mockChannel{name: "primary", weight: 100})
	reg.Register(&mockChannel{name: "backup", weight: 10})

	// Drive both channels below hardStopThreshold.
	for i := 0; i < 10; i++ {
		ht.RecordFailure("primary")
		ht.RecordFailure("backup")
	}

	channels, err := router.RouteWithFallback(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("degraded fallback must return exactly one channel, got %d", len(channels))
	}
	if channels[0].Name() != "backup" {
		t.Errorf("expected lowest-weight 'backup', got %s", channels[0].Name())
	}
}

// The degraded pick must still honour the type filter — when all gpt-image
// channels are unhealthy, pick the lowest-weight gpt-image (not some stray
// gemini channel that happens to be healthy).
func TestRouter_RouteWithTypeFilter_DegradedRespectsFilter(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	reg.Register(&mockChannel{name: "gem", ctype: "gemini", weight: 5})
	reg.Register(&mockChannel{name: "primary", ctype: "gpt-image", weight: 100})
	reg.Register(&mockChannel{name: "backup", ctype: "gpt-image", weight: 10})

	// Only the gpt-image channels crash.
	for i := 0; i < 10; i++ {
		ht.RecordFailure("primary")
		ht.RecordFailure("backup")
	}

	filter := &ChannelTypeFilter{Include: []string{"gpt-image"}}
	channels, err := router.RouteWithTypeFilter(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("degraded fallback must return exactly one channel, got %d", len(channels))
	}
	if channels[0].Name() != "backup" {
		t.Errorf("expected 'backup' (lowest-weight gpt-image), got %s", channels[0].Name())
	}
	// The healthy 'gem' channel must not leak through the filter.
	if channels[0].Type() != "gpt-image" {
		t.Errorf("degraded fallback violated filter: got type %q", channels[0].Type())
	}
}

// Next request after degraded fallback should also get the same channel if
// nothing recovered. (Stateless: each call re-evaluates.)
func TestRouter_RouteWithFallback_DegradedStableAcrossRequests(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	reg.Register(&mockChannel{name: "primary", weight: 100})
	reg.Register(&mockChannel{name: "backup", weight: 10})
	for i := 0; i < 10; i++ {
		ht.RecordFailure("primary")
		ht.RecordFailure("backup")
	}

	for i := 0; i < 3; i++ {
		channels, err := router.RouteWithFallback(context.Background())
		if err != nil {
			t.Fatalf("req %d: unexpected err: %v", i, err)
		}
		if len(channels) != 1 || channels[0].Name() != "backup" {
			t.Errorf("req %d: expected 'backup', got %v", i, names(channels))
		}
	}
}

// If the lowest-weight channel is not IsAvailable (e.g., its account pool is
// empty), it must be skipped — degraded fallback picks the next-lowest
// available channel instead.
func TestRouter_RouteWithFallback_DegradedSkipsUnavailable(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	// 'backup' is the lowest-weight but has IsAvailable=false.
	reg.Register(&mockChannel{name: "primary", weight: 100})
	reg.Register(&mockChannel{name: "backup", weight: 10, unavailable: true})
	reg.Register(&mockChannel{name: "mid", weight: 50})

	for _, n := range []string{"primary", "backup", "mid"} {
		for i := 0; i < 10; i++ {
			ht.RecordFailure(n)
		}
	}

	channels, err := router.RouteWithFallback(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(channels) != 1 || channels[0].Name() != "mid" {
		t.Errorf("expected 'mid' (backup is unavailable), got %v", names(channels))
	}
}

// When zero channels are available (all unhealthy + unavailable, or nothing
// registered), degraded fallback cannot help → error.
func TestRouter_RouteWithFallback_DegradedNoAvailableReturnsError(t *testing.T) {
	reg := NewPluginRegistry()
	ht := NewHealthTracker()
	router := NewRouter(reg, ht)

	reg.Register(&mockChannel{name: "dead", weight: 100, unavailable: true})

	if _, err := router.RouteWithFallback(context.Background()); err == nil {
		t.Fatal("expected error when no channel is available at all")
	}
}

func names(chs []Channel) []string {
	out := make([]string, len(chs))
	for i, ch := range chs {
		out[i] = ch.Name()
	}
	return out
}
