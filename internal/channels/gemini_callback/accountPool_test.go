package gemini_callback

import (
	"testing"
)

func TestWeightedRandomAccountPool(t *testing.T) {
	pool := NewAccountPool()
	pool.AddAccount("key-1", 50)
	pool.AddAccount("key-2", 30)
	pool.AddAccount("key-3", 20)

	selected := make(map[string]int)
	for i := 0; i < 1000; i++ {
		acc, err := pool.Select()
		if err != nil {
			t.Fatalf("Select error: %v", err)
		}
		selected[acc.APIKey()]++
	}

	if selected["key-1"] <= selected["key-3"] {
		t.Errorf("key-1 should be selected more than key-3: %d vs %d", selected["key-1"], selected["key-3"])
	}

	// Return accounts with failures → mark unhealthy
	all := pool.List()
	acc1 := all[0]
	for i := 0; i < 5; i++ {
		pool.Return(acc1, false)
	}
	if acc1.IsHealthy() {
		t.Errorf("acc1 should be unhealthy after 5 failures")
	}

	// Unhealthy should be excluded from selection
	unhealthySelected := 0
	for i := 0; i < 200; i++ {
		acc, _ := pool.Select()
		if acc.APIKey() == acc1.APIKey() {
			unhealthySelected++
		}
	}
	if unhealthySelected > 0 {
		t.Errorf("unhealthy account should not be selected: got %d", unhealthySelected)
	}
}