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

// ChannelHealthReaper periodically probes unhealthy accounts and recovers
// hard-stopped channels after a quiet period.
type ChannelHealthReaper struct {
	reg              *PluginRegistry
	health           *HealthTracker
	probeInterval    time.Duration
	recoveryInterval time.Duration // how long after last failure before resetting a hard-stopped channel
	stopCh           chan struct{}
	stopOnce         sync.Once
	wg               sync.WaitGroup
}

// NewHealthReaper creates a reaper that probes unhealthy accounts on a schedule
// and recovers hard-stopped channels after recoveryInterval of inactivity.
func NewHealthReaper(reg *PluginRegistry, health *HealthTracker, interval time.Duration, recoveryInterval time.Duration) *ChannelHealthReaper {
	return &ChannelHealthReaper{
		reg:              reg,
		health:           health,
		probeInterval:    interval,
		recoveryInterval: recoveryInterval,
		stopCh:           make(chan struct{}),
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
			r.recoverStoppedChannels()
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

// recoverStoppedChannels resets hard-stopped channels (health < hardStopThreshold)
// to 50% health after recoveryInterval has elapsed since their last failure.
// This allows them to re-enter the routing pool with reduced traffic share
// so that real requests can validate their recovery without a separate probe cost.
func (r *ChannelHealthReaper) recoverStoppedChannels() {
	for _, ch := range r.reg.List() {
		score := r.health.HealthScore(ch.Name())
		if score >= hardStopThreshold {
			continue
		}
		lastFail := r.health.LastFailureTime(ch.Name())
		if lastFail.IsZero() || time.Since(lastFail) < r.recoveryInterval {
			continue
		}
		r.health.ResetHealthTo(ch.Name(), 0.5)
		slog.Info("healthReaper: channel recovered to 50%",
			"channel", ch.Name(), "prevScore", score)
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
