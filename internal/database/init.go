package database

import (
	"fmt"
	"log/slog"
	"time"
)

// InitDB 初始化数据库（创建表、索引等）
func (r *Repository) InitDB() error {
	slog.Info("initializing database schema...")
	
	// 自动迁移表结构
	if err := r.db.AutoMigrate(
		&Channel{},
		&Account{},
		&ModelMapping{},
		&APIKey{},
		&UsageLog{},
	); err != nil {
		return fmt.Errorf("failed to migrate tables: %w", err)
	}
	
	slog.Info("database schema initialized successfully")
	return nil
}

// CheckDBConnection 检查数据库连接
func (r *Repository) CheckDBConnection() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}
	
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}
	
	return nil
}

// GetDBStats 获取数据库统计信息
func (r *Repository) GetDBStats() (map[string]int64, error) {
	stats := make(map[string]int64)
	
	var count int64
	
	// 统计各表记录数
	if err := r.db.Model(&Channel{}).Count(&count).Error; err != nil {
		return nil, err
	}
	stats["channels"] = count
	
	if err := r.db.Model(&Account{}).Count(&count).Error; err != nil {
		return nil, err
	}
	stats["accounts"] = count
	
	if err := r.db.Model(&ModelMapping{}).Count(&count).Error; err != nil {
		return nil, err
	}
	stats["model_mappings"] = count
	
	if err := r.db.Model(&APIKey{}).Count(&count).Error; err != nil {
		return nil, err
	}
	stats["api_keys"] = count
	
	if err := r.db.Model(&UsageLog{}).Count(&count).Error; err != nil {
		return nil, err
	}
	stats["usage_logs"] = count
	
	return stats, nil
}

// SeedDefaultData 创建默认数据（可选）
func (r *Repository) SeedDefaultData() error {
	// 检查是否已有数据
	var channelCount int64
	if err := r.db.Model(&Channel{}).Count(&channelCount).Error; err != nil {
		return fmt.Errorf("failed to count channels: %w", err)
	}
	
	// 如果已经有渠道数据，跳过初始化
	if channelCount > 0 {
		slog.Info("database already has data, skipping seed")
		return nil
	}
	
	slog.Info("seeding default data...")
	
	// 这里可以添加默认数据，例如：
	// - 默认的示例渠道配置
	// - 默认的 API Key
	// 但通常建议通过 Admin UI 手动添加，这里暂时留空
	
	slog.Info("default data seeded successfully")
	return nil
}

// CleanupOldLogs 清理旧的使用日志（可以定期调用）
func (r *Repository) CleanupOldLogs(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 30 // 默认保留 30 天
	}
	
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)
	result := r.db.Where("created_at < ?", cutoffTime).Delete(&UsageLog{})
	
	if result.Error != nil {
		return 0, fmt.Errorf("failed to cleanup old logs: %w", result.Error)
	}
	
	return result.RowsAffected, nil
}

// HealthCheck 执行数据库健康检查
func (r *Repository) HealthCheck() error {
	// 1. 检查连接
	if err := r.CheckDBConnection(); err != nil {
		return fmt.Errorf("connection check failed: %w", err)
	}
	
	// 2. 检查表是否存在
	if !r.db.Migrator().HasTable(&Channel{}) {
		return fmt.Errorf("channel table does not exist")
	}
	if !r.db.Migrator().HasTable(&Account{}) {
		return fmt.Errorf("account table does not exist")
	}
	if !r.db.Migrator().HasTable(&ModelMapping{}) {
		return fmt.Errorf("model_mapping table does not exist")
	}
	if !r.db.Migrator().HasTable(&APIKey{}) {
		return fmt.Errorf("api_key table does not exist")
	}
	if !r.db.Migrator().HasTable(&UsageLog{}) {
		return fmt.Errorf("usage_log table does not exist")
	}
	
	// 3. 执行简单查询测试
	var count int64
	if err := r.db.Model(&Channel{}).Limit(1).Count(&count).Error; err != nil {
		return fmt.Errorf("query test failed: %w", err)
	}
	
	return nil
}
