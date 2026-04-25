package core

import (
	"log/slog"
	"time"
	
	"goloop/internal/database"
)

// UsageLogger 使用日志批量写入器
type UsageLogger struct {
	buffer        chan database.LogEntry
	repo          *database.Repository
	batchSize     int
	flushInterval time.Duration
	stopCh        chan struct{}
}

// NewUsageLogger 创建使用日志批量写入器
func NewUsageLogger(repo *database.Repository, batchSize int, flushInterval time.Duration) *UsageLogger {
	if batchSize == 0 {
		batchSize = 1000
	}
	if flushInterval == 0 {
		flushInterval = 10 * time.Second
	}
	
	return &UsageLogger{
		buffer:        make(chan database.LogEntry, batchSize*2), // 缓冲区是批量大小的 2 倍
		repo:          repo,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		stopCh:        make(chan struct{}),
	}
}

// Log 记录使用日志（非阻塞）
func (l *UsageLogger) Log(entry database.LogEntry) {
	select {
	case l.buffer <- entry:
	default:
		slog.Warn("usage log buffer full, dropping entry")
	}
}

// Start 启动批量写入器
func (l *UsageLogger) Start() {
	ticker := time.NewTicker(l.flushInterval)
	defer ticker.Stop()
	
	batch := make([]database.LogEntry, 0, l.batchSize)
	
	for {
		select {
		case entry := <-l.buffer:
			batch = append(batch, entry)
			if len(batch) >= l.batchSize {
				l.flush(batch)
				batch = batch[:0] // 清空但保留容量
			}
			
		case <-ticker.C:
			if len(batch) > 0 {
				l.flush(batch)
				batch = batch[:0]
			}
			
		case <-l.stopCh:
			// 停止前刷新剩余日志
			if len(batch) > 0 {
				l.flush(batch)
			}
			return
		}
	}
}

// Stop 停止批量写入器
func (l *UsageLogger) Stop() {
	close(l.stopCh)
}

func (l *UsageLogger) flush(batch []database.LogEntry) {
	if len(batch) == 0 {
		return
	}
	
	// 批量插入日志
	if err := l.repo.BatchInsertUsageLogs(batch); err != nil {
		slog.Error("failed to flush usage logs", "err", err, "count", len(batch))
		return
	}
	
	// 更新统计字段
	if err := l.repo.UpdateAPIKeyStats(batch); err != nil {
		slog.Error("failed to update api key stats", "err", err)
		return
	}
	
	slog.Debug("flushed usage logs", "count", len(batch))
}
