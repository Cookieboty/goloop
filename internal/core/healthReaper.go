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
    reg             *PluginRegistry
    health          *HealthTracker
    probeInterval   time.Duration
    recoveryThresh int
    stopCh          chan struct{}
    wg              sync.WaitGroup
}

// NewHealthReaper creates a reaper that starts probing unhealthy accounts.
func NewHealthReaper(reg *PluginRegistry, health *HealthTracker, interval time.Duration, recoveryThresh int) *ChannelHealthReaper {
    return &ChannelHealthReaper{
        reg:             reg,
        health:          health,
        probeInterval:   interval,
        recoveryThresh: recoveryThresh,
        stopCh:          make(chan struct{}),
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

// Stop gracefully stops the reaper.
func (r *ChannelHealthReaper) Stop() {
    close(r.stopCh)
    r.wg.Wait()
}

func (r *ChannelHealthReaper) probeUnhealthyAccounts() {
    for _, ch := range r.reg.List() {
        // Type assert to get the AccountPool
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
                continue // skip healthy accounts
            }

            // Probe the account
            ok := ch.Probe(acc)
            if ok {
                // Probe succeeded: record as success (not counted as failure)
                acc.RecordSuccess()
                r.health.RecordSuccess(ch.Name())
                slog.Info("healthReaper: probe succeeded, recovered",
                    "channel", ch.Name(), "apiKey", acc.APIKey()[:8]+"...")
            } else {
                // Probe failed: just log, do not penalize
                slog.Debug("healthReaper: probe failed",
                    "channel", ch.Name(), "apiKey", acc.APIKey()[:8]+"...")
            }
        }
    }
}