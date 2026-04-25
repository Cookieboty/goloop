package database

import (
	"fmt"
	"log/slog"
	
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewDB 创建数据库连接并自动迁移表结构
func NewDB(databaseURL string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // 生产环境使用 Silent
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	
	// 自动迁移表结构
	if err := db.AutoMigrate(
		&Channel{},
		&Account{},
		&ModelMapping{},
		&APIKey{},
		&UsageLog{},
	); err != nil {
		return nil, fmt.Errorf("failed to auto migrate: %w", err)
	}
	
	slog.Info("database connected and migrated successfully")
	return db, nil
}
