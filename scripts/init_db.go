// scripts/init_db.go
// 数据库初始化脚本 - 独立运行工具
// 用法: go run scripts/init_db.go [--seed]
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"goloop/internal/database"
	"goloop/internal/config"
)

func main() {
	seed := flag.Bool("seed", false, "seed default data after initialization")
	clean := flag.Bool("clean", false, "clean old usage logs (30+ days)")
	check := flag.Bool("check", false, "only check database health")
	dbURL := flag.String("db", "", "database URL (or use DATABASE_URL env var)")
	flag.Parse()

	// 设置日志
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// 获取数据库 URL
	databaseURL := *dbURL
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		// 尝试加载配置
		cfg, err := config.Load()
		if err != nil {
			slog.Error("failed to get database URL", "err", err)
			slog.Error("please provide database URL via --db flag or DATABASE_URL env var")
			os.Exit(1)
		}
		databaseURL = cfg.DatabaseURL
	}

	// 连接数据库
	slog.Info("connecting to database", "url", maskPassword(databaseURL))
	db, err := database.NewDB(databaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	repo := database.NewRepository(db)

	if *check {
		// 仅执行健康检查
		slog.Info("performing database health check...")
		if err := repo.HealthCheck(); err != nil {
			slog.Error("health check failed", "err", err)
			os.Exit(1)
		}
		slog.Info("✓ database health check passed")
		
		// 显示统计信息
		if stats, err := repo.GetDBStats(); err == nil {
			slog.Info("database statistics",
				"channels", stats["channels"],
				"accounts", stats["accounts"],
				"model_mappings", stats["model_mappings"],
				"api_keys", stats["api_keys"],
				"usage_logs", stats["usage_logs"])
		}
		return
	}

	// 初始化数据库
	slog.Info("initializing database schema...")
	if err := repo.InitDB(); err != nil {
		slog.Error("failed to initialize database", "err", err)
		os.Exit(1)
	}
	slog.Info("✓ database schema initialized")

	// 健康检查
	if err := repo.HealthCheck(); err != nil {
		slog.Error("health check failed", "err", err)
		os.Exit(1)
	}
	slog.Info("✓ health check passed")

	// 创建默认数据
	if *seed {
		slog.Info("seeding default data...")
		if err := repo.SeedDefaultData(); err != nil {
			slog.Error("failed to seed default data", "err", err)
			os.Exit(1)
		}
		slog.Info("✓ default data seeded")
	}

	// 清理旧日志
	if *clean {
		slog.Info("cleaning old usage logs (30+ days)...")
		deleted, err := repo.CleanupOldLogs(30)
		if err != nil {
			slog.Error("failed to cleanup old logs", "err", err)
			os.Exit(1)
		}
		slog.Info(fmt.Sprintf("✓ cleaned %d old log entries", deleted))
	}

	// 显示统计信息
	if stats, err := repo.GetDBStats(); err == nil {
		slog.Info("database statistics",
			"channels", stats["channels"],
			"accounts", stats["accounts"],
			"model_mappings", stats["model_mappings"],
			"api_keys", stats["api_keys"],
			"usage_logs", stats["usage_logs"])
	}

	slog.Info("✓ database initialization completed successfully")
}

// maskPassword 隐藏数据库 URL 中的密码
func maskPassword(url string) string {
	// postgresql://user:password@host:port/db
	start := len("postgresql://")
	if len(url) <= start {
		return url
	}
	atIdx := -1
	for i := start; i < len(url); i++ {
		if url[i] == '@' {
			atIdx = i
			break
		}
	}
	if atIdx == -1 {
		return url
	}
	// 找到 user:password 部分
	userPass := url[start:atIdx]
	colonIdx := -1
	for i := 0; i < len(userPass); i++ {
		if userPass[i] == ':' {
			colonIdx = i
			break
		}
	}
	if colonIdx == -1 {
		return url
	}
	user := userPass[:colonIdx]
	return "postgresql://" + user + ":***@" + url[atIdx+1:]
}
