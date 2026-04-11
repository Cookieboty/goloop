package core

import "testing"

func TestAccountInterface(t *testing.T) {
    var _ Account = &mockAccount{key: "k1", weight: 50}

    acc := &mockAccount{key: "k1", weight: 50}
    if acc.APIKey() != "k1" { t.Errorf("APIKey mismatch") }
    if acc.Weight() != 50 { t.Errorf("Weight mismatch") }
    if acc.UsageCount() != 0 { t.Errorf("UsageCount should start at 0") }
    if acc.HealthScore() != 1.0 { t.Errorf("HealthScore initial should be 1.0") }
    if !acc.IsHealthy() { t.Errorf("IsHealthy initial should be true") }

    acc.IncUsage()
    if acc.UsageCount() != 1 { t.Errorf("UsageCount should be 1 after IncUsage") }

    acc.RecordFailure()
    if acc.HealthScore() >= 1.0 { t.Errorf("HealthScore should decrease after failure") }
    if acc.consecutiveFailures != 1 { t.Errorf("consecutiveFailures should be 1") }

    acc.RecordSuccess()
    if acc.consecutiveFailures != 0 { t.Errorf("consecutiveFailures should reset after success") }
}

type mockAccount struct {
    key               string
    weight            int
    usageCount        int
    consecutiveFailures int
}

func (m *mockAccount) APIKey() string       { return m.key }
func (m *mockAccount) Weight() int          { return m.weight }
func (m *mockAccount) UsageCount() int     { return m.usageCount }
func (m *mockAccount) HealthScore() float64 {
    if m.consecutiveFailures == 0 { return 1.0 }
    return 1.0 - float64(m.consecutiveFailures)*0.2
}
func (m *mockAccount) IsHealthy() bool { return m.consecutiveFailures < 5 }
func (m *mockAccount) IncUsage()         { m.usageCount++ }
func (m *mockAccount) RecordFailure()     { m.consecutiveFailures++ }
func (m *mockAccount) RecordSuccess()     { m.consecutiveFailures = 0 }
