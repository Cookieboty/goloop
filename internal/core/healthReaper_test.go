package core

import (
    "testing"
    "time"
)

func TestHealthReaper_DoesNotProbeHealthy(t *testing.T) {
    reg := NewPluginRegistry()
    ht := NewHealthTracker()
    reaper := NewHealthReaper(reg, ht, 1*time.Second, 2)

    ch := &mockChannel{name: "ch1"}
    reg.Register(ch)

    reaper.Start()

    // Wait 2 probe cycles
    time.Sleep(2500 * time.Millisecond)

    reaper.Stop()

    // reaper should not panic and channel should remain healthy
    if !ht.IsHealthy("ch1") { t.Errorf("healthy channel should stay healthy") }
}