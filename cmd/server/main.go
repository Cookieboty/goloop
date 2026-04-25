package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"goloop/internal/admin"
	"goloop/internal/cache"
	"goloop/internal/config"
	"goloop/internal/core"
	"goloop/internal/database"
	"goloop/internal/handler"
	kieaipkg "goloop/internal/kieai"
	"goloop/internal/middleware"
	"goloop/internal/storage"
	"goloop/internal/transformer"
)

func main() {
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	// Database
	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	repo := database.NewRepository(db)
	
	// Initialize database schema (create tables if not exist)
	slog.Info("checking database schema...")
	if err := repo.InitDB(); err != nil {
		slog.Error("failed to initialize database schema", "err", err)
		os.Exit(1)
	}
	
	// Health check
	if err := repo.HealthCheck(); err != nil {
		slog.Error("database health check failed", "err", err)
		os.Exit(1)
	}
	
	// Show database stats
	if stats, err := repo.GetDBStats(); err == nil {
		slog.Info("database statistics",
			"channels", stats["channels"],
			"accounts", stats["accounts"],
			"model_mappings", stats["model_mappings"],
			"api_keys", stats["api_keys"],
			"usage_logs", stats["usage_logs"])
	}

	// Redis
	slog.Debug("redis config", "enabled", cfg.Redis.Enabled, "url", cfg.Redis.URL)
	var redisClient *cache.APIKeyCache
	if cfg.Redis.Enabled {
		slog.Info("attempting to connect to redis", "url", cfg.Redis.URL)
		client, err := cache.NewRedisClient(cfg.Redis.URL)
		if err != nil {
			slog.Error("failed to connect to redis", "err", err)
			os.Exit(1)
		}
		redisClient = cache.NewAPIKeyCache(client, cfg.Redis.APIKeyCacheTTL)
	} else {
		slog.Warn("Redis is disabled - API Key validation will be slow and less reliable")
	}

	// Config Manager (load channels from database)
	configMgr := core.NewConfigManager(repo)
	if err := configMgr.Load(); err != nil {
		slog.Warn("failed to load channels from database, using empty config", "err", err)
	}

	// Core infrastructure
	registry := core.NewPluginRegistry()
	health := core.NewHealthTracker()
	router := core.NewRouter(registry, health)
	issuer := core.NewJWTIssuer(cfg.JWT.Secret, cfg.JWT.Expiry)

	// Storage
	store, err := storage.NewStore(cfg.Storage.LocalPath, cfg.Storage.BaseURL, cfg.Storage.DownloadTimeout, cfg.Storage.MaxImageBytes)
	if err != nil {
		slog.Error("failed to init storage", "err", err)
		os.Exit(1)
	}

	// Cleanup worker for old images
	cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
	defer cancelCleanup()
	go store.StartCleanupWorker(cleanupCtx, 1*time.Hour, 24*time.Hour)

	// Bootstrap all configured channels from database
	bootstrapper := NewChannelBootstrapper(registry, configMgr, store)
	if err := bootstrapper.Bootstrap(); err != nil {
		slog.Error("failed to bootstrap channels", "err", err)
		os.Exit(1)
	}
	
	// Find first callback channel for task manager
	var firstCallbackBaseURL string
	var firstCallbackTimeout time.Duration
	for _, chCfg := range configMgr.GetAllChannels() {
		if chCfg.Type == "gemini_callback" || chCfg.Type == "openai_callback" {
			firstCallbackBaseURL = chCfg.BaseURL
			firstCallbackTimeout = chCfg.Timeout
			if firstCallbackTimeout == 0 {
				firstCallbackTimeout = 120 * time.Second
			}
			break
		}
	}

	// Task manager for streaming (uses the first callback channel's connection).
	var taskManager *kieaipkg.TaskManager
	if firstCallbackBaseURL != "" {
		kClient := kieaipkg.NewClient(firstCallbackBaseURL, firstCallbackTimeout)
		
		// Use default poller config
		pollerCfg := kieaipkg.PollerConfig{
			InitialInterval: 2 * time.Second,
			MaxInterval:     10 * time.Second,
			MaxWaitTime:     120 * time.Second,
			RetryAttempts:   3,
		}
		
		// Try to use the first gemini_callback channel's config if available
		for _, chCfg := range configMgr.GetAllChannels() {
			if chCfg.Type == "gemini_callback" {
				pollerCfg.InitialInterval = chCfg.InitialInterval
				pollerCfg.MaxInterval = chCfg.MaxInterval
				pollerCfg.MaxWaitTime = chCfg.MaxWaitTime
				pollerCfg.RetryAttempts = chCfg.RetryAttempts
				break
			}
		}
		
		taskManager = kieaipkg.NewTaskManager(kClient, pollerCfg, 20)
		defer taskManager.Stop()
	}

	// Start health reaper for automatic account recovery.
	reaper := core.NewHealthReaper(registry, health, cfg.Health.ProbeInterval, cfg.Health.RecoveryInterval)
	reaper.Start()
	defer reaper.Stop()

	// Usage logger (batch writes)
	usageLogger := core.NewUsageLogger(repo, 1000, 10*time.Second)
	go usageLogger.Start()
	defer usageLogger.Stop()

	// Log cleaner (delete logs older than 30 days)
	logCleaner := core.NewLogCleaner(repo, 24*time.Hour, 30*24*time.Hour)
	go logCleaner.Start()
	defer logCleaner.Stop()

	// Transformers (model mapping now comes from configMgr)
	reqTransformer := transformer.NewRequestTransformer(store, configMgr, cfg.Storage.MaxImageBytes)
	respTransformer := transformer.NewResponseTransformer(store)

	// HTTP handlers
	geminiHandler := handler.NewGeminiHandler(router, registry, issuer, store, taskManager, reqTransformer, respTransformer, cfg.Server.MaxRequestBodyBytes, usageLogger)
	openaiHandler := handler.NewOpenAIHandler(router, registry, issuer, configMgr, cfg.Server.MaxRequestBodyBytes, usageLogger)
	adminHandler := handler.NewAdminHandler(issuer, registry, health, cfg.AdminPassword, repo, redisClient, configMgr, bootstrapper)

	// Business API routes (Gemini, OpenAI) - require API Key authentication
	businessMux := http.NewServeMux()
	geminiHandler.RegisterRoutes(businessMux)
	openaiHandler.RegisterRoutes(businessMux)
	
	// Wrap business routes with API Key middleware
	apiKeyMiddleware := middleware.NewAPIKeyMiddleware(redisClient, repo, configMgr, businessMux)
	
	// Main mux combines business routes (with API Key auth) and admin routes
	mux := http.NewServeMux()
	mux.Handle("/v1/", apiKeyMiddleware)
	mux.Handle("/v1beta/", apiKeyMiddleware)
	adminHandler.RegisterRoutes(mux)

	// Apply rate limiting if configured
	var handler http.Handler = mux
	if cfg.RateLimit.RPS > 0 {
		rateLimitCtx, cancelRateLimit := context.WithCancel(context.Background())
		defer cancelRateLimit()
		rateLimiter := middleware.NewRateLimiter(rateLimitCtx, cfg.RateLimit.RPS, cfg.RateLimit.Burst)
		handler = rateLimiter.Middleware(mux)
		slog.Info("rate limiting enabled", "rps", cfg.RateLimit.RPS, "burst", cfg.RateLimit.Burst)
	}

	// Admin UI static files (embedded Next.js static export)
	uiFS, uiErr := fs.Sub(admin.UIAssets, "out")
	if uiErr != nil {
		slog.Error("failed to create UI sub-FS", "err", uiErr)
		os.Exit(1)
	}
	mux.Handle("/admin/ui/", http.StripPrefix("/admin/ui/", http.FileServerFS(uiFS)))

	// Image file server (public access, no auth required)
	mux.HandleFunc("/images/", func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/images/", http.FileServer(http.Dir(cfg.Storage.LocalPath))).ServeHTTP(w, r)
	})

	// Root redirects to admin UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/ui/", http.StatusFound)
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		slog.Info("server starting", "port", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	slog.Info("server stopped")
}
