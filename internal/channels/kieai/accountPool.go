package kieai

import (
	"errors"
	"math/rand"
	"sync"

	"goloop/internal/core"
)

// Account aliases core.Account for the KIE.AI channel.
type Account = core.Account

// kieAccount implements Account for the KIE.AI channel.
type kieAccount struct {
	apiKey     string
	weight     int
	usageCount int
	failCount  int
	healthy    bool
	mu         sync.RWMutex
}

func (a *kieAccount) APIKey() string {
	return a.apiKey
}
func (a *kieAccount) Weight() int {
	return a.weight
}
func (a *kieAccount) UsageCount() int {
	return a.usageCount
}
func (a *kieAccount) HealthScore() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return 1.0 - float64(a.failCount)*0.2
}
func (a *kieAccount) IsHealthy() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.healthy && a.failCount < 5
}
func (a *kieAccount) IncUsage() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.usageCount++
}
func (a *kieAccount) RecordFailure() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failCount++
	if a.failCount >= 5 {
		a.healthy = false
	}
}
func (a *kieAccount) RecordSuccess() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failCount = max(0, a.failCount-1)
	a.healthy = true
}

// AccountPool manages KIE.AI accounts with weighted random selection.
type AccountPool struct {
	mu       sync.RWMutex
	accounts []*kieAccount
}

func NewAccountPool() *AccountPool {
	return &AccountPool{}
}

func (p *AccountPool) AddAccount(apiKey string, weight int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.accounts = append(p.accounts, &kieAccount{apiKey: apiKey, weight: weight, healthy: true})
}

func (p *AccountPool) Select() (Account, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	candidates := make([]Account, 0, len(p.accounts))
	weights := make([]int, 0, len(p.accounts))
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
		return nil, errors.New("kieai: no healthy accounts available")
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

func (p *AccountPool) Return(acc Account, success bool) {
	if success {
		acc.RecordSuccess()
	} else {
		acc.RecordFailure()
	}
}

func (p *AccountPool) List() []Account {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ret := make([]Account, len(p.accounts))
	for i, a := range p.accounts {
		ret[i] = a
	}
	return ret
}

func (p *AccountPool) Remove(apiKey string) bool {
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