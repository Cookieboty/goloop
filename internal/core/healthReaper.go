package core

import (
	"log/slog"
	"sync"
	"time"
)

// ChannelWithPool is implemented by channels that expose an AccountPool.
type ChannelWithPool interface {
	Channel
	GetAccountPool() AccountPool
}

// ChannelHealthReaper periodically probes unhealthy accounts to recover them.
type ChannelHealthReaper struct {
	reg            *PluginRegistry
	health         *HealthTracker
	probeInterval  time.Duration
	recoveryThresh int
	stopCh         chan struct{}
	stopOnce       sync.Once
	wg             sync.WaitGroup
}

// NewHealthReaper creates a reaper that probes unhealthy accounts on a schedule.
func NewHealthReaper(reg *PluginRegistry, health *HealthTracker, interval time.Duration, recoveryThresh int) *ChannelHealthReaper {
	return &ChannelHealthReaper{
		reg:            reg,
		health:         health,
		probeInterval:  interval,
		recoveryThresh: recoveryThresh,
		stopCh:         make(chan struct{}),
	}
}

// Start begins the background probe loop.
func (r *ChannelHealthReaper) Start() {
	r.wg.Add(1)
	go r.run()
	slog.Info("healthReaper started", "interval", r.probeInterval)
}

func (r *ChannelHealthReaper) run() {
	defer r.wg.Done()
	ticker := time.NewTicker(r.probeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			slog.Info("healthReaper stopped")
			return
		case <-ticker.C:
			r.probeUnhealthyAccounts()
		}
	}
}

// Stop gracefully stops the reaper. Safe to call multiple times.
func (r *ChannelHealthReaper) Stop() {
	r.stopOnce.Do(func() {
		close(r.stopCh)
	})
	r.wg.Wait()
}

func (r *ChannelHealthReaper) probeUnhealthyAccounts() {
	for _, ch := range r.reg.List() {
		chwp, ok := ch.(ChannelWithPool)
		if !ok {
			continue
		}
		pool := chwp.GetAccountPool()
		if pool == nil {
			continue
		}

		for _, acc := range pool.List() {
			if acc.IsHealthy() {
				continue
			}

			ok := ch.Probe(acc)
			keyPreview := safeKeyPreview(acc.APIKey())
			if ok {
				acc.RecordSuccess()
				r.health.RecordSuccess(ch.Name())
				slog.Info("healthReaper: probe succeeded, recovered",
					"channel", ch.Name(), "apiKey", keyPreview)
			} else {
				slog.Debug("healthReaper: probe failed",
					"channel", ch.Name(), "apiKey", keyPreview)
			}
		}
	}
}

// safeKeyPreview returns a safe preview of an API key for logging,
// handling keys shorter than 8 characters without panicking.
func safeKeyPreview(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:8] + "..."
}
