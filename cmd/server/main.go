package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"goloop/internal/channels/kieai"
	"goloop/internal/config"
	"goloop/internal/core"
	"goloop/internal/handler"
	kieaipkg "goloop/internal/kieai"
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

	// Core infrastructure
	registry := core.NewPluginRegistry()
	health := core.NewHealthTracker()
	router := core.NewRouter(registry, health)
	issuer := core.NewJWTIssuer(cfg.JWT.Secret, cfg.JWT.Expiry)

	// Storage
	store, err := storage.NewStore(cfg.Storage.LocalPath, cfg.Storage.BaseURL)
	if err != nil {
		slog.Error("failed to init storage", "err", err)
		os.Exit(1)
	}

	// Cleanup worker for old images
	cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
	defer cancelCleanup()
	go store.StartCleanupWorker(cleanupCtx, 1*time.Hour, 24*time.Hour)

	// Bootstrap channels
	var kieBaseURL string
	var kieTimeout time.Duration
	for name, chCfg := range cfg.Channels {
		switch chCfg.Type {
		case "kieai":
			pool := kieai.NewAccountPool()
			for _, acc := range chCfg.Accounts {
				pool.AddAccount(acc.APIKey, acc.Weight)
			}
			timeout := chCfg.Timeout
			if timeout == 0 {
				timeout = 120 * time.Second
			}
			kieCh := kieai.NewChannel(chCfg.BaseURL, pool, kieai.Config{
				BaseURL:         chCfg.BaseURL,
				Timeout:         timeout,
				InitialInterval: chCfg.InitialInterval,
				MaxInterval:     chCfg.MaxInterval,
				MaxWaitTime:     chCfg.MaxWaitTime,
				RetryAttempts:   chCfg.RetryAttempts,
			})
			registry.Register(kieCh)
			slog.Info("channel registered", "name", name, "accounts", len(chCfg.Accounts))
			// Capture first kieai channel's baseURL for task manager
			if kieBaseURL == "" {
				kieBaseURL = chCfg.BaseURL
				kieTimeout = timeout
			}
		default:
			slog.Warn("unknown channel type", "name", name, "type", chCfg.Type)
		}
	}

	if len(registry.List()) == 0 {
		slog.Error("no channels registered")
		os.Exit(1)
	}

	// Task manager for streaming (uses first kieai channel's baseURL)
	var taskManager *kieaipkg.TaskManager
	if kieBaseURL != "" {
		kClient := kieaipkg.NewClient(kieBaseURL, kieTimeout)
		pollerCfg := kieaipkg.PollerConfig{
			InitialInterval: cfg.Channels["kieai"].InitialInterval,
			MaxInterval:     cfg.Channels["kieai"].MaxInterval,
			MaxWaitTime:     cfg.Channels["kieai"].MaxWaitTime,
			RetryAttempts:   cfg.Channels["kieai"].RetryAttempts,
		}
		taskManager = kieaipkg.NewTaskManager(kClient, pollerCfg, 20)
		defer taskManager.Stop()
	}

	// Transformers
	reqTransformer := transformer.NewRequestTransformer(store, cfg.ModelMapping)
	respTransformer := transformer.NewResponseTransformer(store)

	// Handlers
	geminiHandler := handler.NewGeminiHandler(router, registry, issuer, store, taskManager, reqTransformer, respTransformer)

	mux := http.NewServeMux()
	geminiHandler.RegisterRoutes(mux)

	// Image file server
	mux.HandleFunc("/images/", func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/images/", http.FileServer(http.Dir(cfg.Storage.LocalPath))).ServeHTTP(w, r)
	})

	// Root redirects to gemini endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/v1beta/models", http.StatusFound)
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      mux,
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
