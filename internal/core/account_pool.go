package core

import (
	"errors"
	"math/rand"
	"sync"
)

// DefaultAccount implements the Account interface with weighted health tracking.
// It is the standard account implementation shared across all channel types.
type DefaultAccount struct {
	apiKey     string
	weight     int
	usageCount int
	failCount  int
	healthy    bool
	mu         sync.RWMutex
}

func (a *DefaultAccount) APIKey() string {
	return a.apiKey
}

func (a *DefaultAccount) Weight() int {
	return a.weight
}

func (a *DefaultAccount) UsageCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.usageCount
}

func (a *DefaultAccount) HealthScore() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	score := 1.0 - float64(a.failCount)*0.2
	if score < 0 {
		return 0
	}
	return score
}

func (a *DefaultAccount) IsHealthy() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.healthy && a.failCount < 5
}

func (a *DefaultAccount) IncUsage() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.usageCount++
}

func (a *DefaultAccount) RecordFailure() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failCount++
	if a.failCount >= 5 {
		a.healthy = false
	}
}

func (a *DefaultAccount) RecordSuccess() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.failCount > 0 {
		a.failCount--
	}
	a.healthy = true
}

// ConsecutiveFailures returns the current failure count (for admin display).
func (a *DefaultAccount) ConsecutiveFailures() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.failCount
}

// SetWeight updates the account's routing weight.
func (a *DefaultAccount) SetWeight(weight int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.weight = weight
}

// DefaultAccountPool manages a set of DefaultAccounts with weighted random
// selection and health-aware filtering. It implements the AccountPool interface.
type DefaultAccountPool struct {
	mu       sync.RWMutex
	accounts []*DefaultAccount
}

// NewDefaultAccountPool creates an empty account pool.
func NewDefaultAccountPool() *DefaultAccountPool {
	return &DefaultAccountPool{}
}

// AddAccount registers a new account with the given API key and routing weight.
func (p *DefaultAccountPool) AddAccount(apiKey string, weight int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.accounts = append(p.accounts, &DefaultAccount{
		apiKey:  apiKey,
		weight:  weight,
		healthy: true,
	})
}

// Select picks a healthy account using weighted random selection.
func (p *DefaultAccountPool) Select() (Account, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var candidates []Account
	var weights []int
	totalWeight := 0

	for _, acc := range p.accounts {
		if acc.IsHealthy() {
			effectiveWeight := acc.Weight() * int(acc.HealthScore()*100) / 100
			if effectiveWeight <= 0 {
				effectiveWeight = 1
			}
			candidates = append(candidates, acc)
			weights = append(weights, effectiveWeight)
			totalWeight += effectiveWeight
		}
	}

	if len(candidates) == 0 {
		return nil, errors.New("account pool: no healthy accounts available")
	}

	n := rand.Intn(totalWeight)
	var cumulative int
	for i, acc := range candidates {
		cumulative += weights[i]
		if n < cumulative {
			return acc, nil
		}
	}
	return candidates[len(candidates)-1], nil
}

// Return records the outcome of using an account and updates its health state.
func (p *DefaultAccountPool) Return(acc Account, success bool) {
	if success {
		acc.RecordSuccess()
	} else {
		acc.RecordFailure()
	}
}

// List returns all accounts (including unhealthy ones).
func (p *DefaultAccountPool) List() []Account {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ret := make([]Account, len(p.accounts))
	for i, a := range p.accounts {
		ret[i] = a
	}
	return ret
}

// ListRaw returns direct pointers to internal accounts for admin operations.
func (p *DefaultAccountPool) ListRaw() []*DefaultAccount {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ret := make([]*DefaultAccount, len(p.accounts))
	copy(ret, p.accounts)
	return ret
}

// Remove removes an account by API key. Returns true if found and removed.
func (p *DefaultAccountPool) Remove(apiKey string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, acc := range p.accounts {
		if acc.APIKey() == apiKey {
			p.accounts = append(p.accounts[:i], p.accounts[i+1:]...)
			return true
		}
	}
	return false
}

// SetWeight updates the weight of an account by API key.
func (p *DefaultAccountPool) SetWeight(apiKey string, weight int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, acc := range p.accounts {
		if acc.APIKey() == apiKey {
			acc.SetWeight(weight)
			return true
		}
	}
	return false
}
