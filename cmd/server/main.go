// cmd/server/main.go
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

	"goloop/internal/config"
	"goloop/internal/handler"
	"goloop/internal/kieai"
	"goloop/internal/storage"
	"goloop/internal/transformer"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to config file")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	store, err := storage.NewStore(cfg.Storage.LocalPath, cfg.Storage.BaseURL)
	if err != nil {
		slog.Error("failed to init storage", "err", err)
		os.Exit(1)
	}

	kieaiClient := kieai.NewClient(cfg.KieAI.BaseURL, cfg.KieAI.Timeout)
	poller := kieai.NewPoller(kieaiClient, kieai.PollerConfig{
		InitialInterval: cfg.Poller.InitialInterval,
		MaxInterval:     cfg.Poller.MaxInterval,
		MaxWaitTime:     cfg.Poller.MaxWaitTime,
		RetryAttempts:   cfg.Poller.RetryAttempts,
	})

	reqTransformer := transformer.NewRequestTransformer(store, cfg.ModelMapping)
	respTransformer := transformer.NewResponseTransformer(store)

	geminiHandler := handler.NewGeminiHandler(reqTransformer, respTransformer, kieaiClient, poller)
	mux := http.NewServeMux()
	geminiHandler.RegisterRoutes(mux)

	// Static file server for saved images
	mux.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir(cfg.Storage.LocalPath))))

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

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}
