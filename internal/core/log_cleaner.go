package core

import (
	"log/slog"
	"time"
	
	"goloop/internal/database"
)

// LogCleaner 日志定时清理器
type LogCleaner struct {
	repo      *database.Repository
	interval  time.Duration
	retention time.Duration // 保留时长（默认 30 天）
	stopCh    chan struct{}
}

// NewLogCleaner 创建日志清理器
func NewLogCleaner(repo *database.Repository, interval, retention time.Duration) *LogCleaner {
	if interval == 0 {
		interval = 24 * time.Hour // 默认每天
	}
	if retention == 0 {
		retention = 30 * 24 * time.Hour // 默认保留 30 天
	}
	
	return &LogCleaner{
		repo:      repo,
		interval:  interval,
		retention: retention,
		stopCh:    make(chan struct{}),
	}
}

// Start 启动清理器
func (l *LogCleaner) Start() {
	// 计算下一次清理时间（凌晨 2 点）
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 2, 0, 0, 0, now.Location())
	if now.After(next) {
		next = next.Add(24 * time.Hour)
	}
	
	// 等待到下一次清理时间
	timer := time.NewTimer(time.Until(next))
	defer timer.Stop()
	
	for {
		select {
		case <-timer.C:
			l.clean()
			// 重置为下一天凌晨 2 点
			timer.Reset(24 * time.Hour)
			
		case <-l.stopCh:
			return
		}
	}
}

// Stop 停止清理器
func (l *LogCleaner) Stop() {
	close(l.stopCh)
}

func (l *LogCleaner) clean() {
	cutoffTime := time.Now().Add(-l.retention)
	
	deleted, err := l.repo.DeleteUsageLogsBefore(cutoffTime)
	if err != nil {
		slog.Error("failed to clean usage logs", "err", err)
		return
	}
	
	if deleted > 0 {
		slog.Info("cleaned usage logs", "deleted", deleted, "before", cutoffTime.Format("2006-01-02"))
	}
}
