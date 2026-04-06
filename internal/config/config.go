// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server       ServerConfig
	KieAI        KieAIConfig
	Poller       PollerConfig
	Storage      StorageConfig
	ModelMapping map[string]ModelDefaults
}

type ServerConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type KieAIConfig struct {
	BaseURL string
	Timeout time.Duration
}

type PollerConfig struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	MaxWaitTime     time.Duration
	RetryAttempts   int
}

type StorageConfig struct {
	Type      string
	LocalPath string
	BaseURL   string
}

type ModelDefaults struct {
	KieAIModel   string
	AspectRatio  string
	Resolution   string
	OutputFormat string
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key, fallback string) time.Duration {
	s := getEnv(key, fallback)
	d, err := time.ParseDuration(s)
	if err != nil {
		d, _ = time.ParseDuration(fallback)
	}
	return d
}

func getEnvInt(key string, fallback int) int {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:         getEnvInt("SERVER_PORT", 8080),
			ReadTimeout:  getEnvDuration("SERVER_READ_TIMEOUT", "130s"),
			WriteTimeout: getEnvDuration("SERVER_WRITE_TIMEOUT", "130s"),
		},
		KieAI: KieAIConfig{
			BaseURL: getEnv("KIEAI_BASE_URL", ""),
			Timeout: getEnvDuration("KIEAI_TIMEOUT", "120s"),
		},
		Poller: PollerConfig{
			InitialInterval: getEnvDuration("POLLER_INITIAL_INTERVAL", "2s"),
			MaxInterval:     getEnvDuration("POLLER_MAX_INTERVAL", "10s"),
			MaxWaitTime:     getEnvDuration("POLLER_MAX_WAIT_TIME", "120s"),
			RetryAttempts:   getEnvInt("POLLER_RETRY_ATTEMPTS", 3),
		},
		Storage: StorageConfig{
			Type:      getEnv("STORAGE_TYPE", "local"),
			LocalPath: getEnv("STORAGE_LOCAL_PATH", "/tmp/images"),
			BaseURL:   getEnv("STORAGE_BASE_URL", ""),
		},
		ModelMapping: defaultModelMapping(),
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func defaultModelMapping() map[string]ModelDefaults {
	return map[string]ModelDefaults{
		"gemini-3.1-flash-image-preview": {
			KieAIModel:   getEnv("MODEL_NANO_BANANA_2", "nano-banana-2"),
			AspectRatio:  "1:1",
			Resolution:   "1K",
			OutputFormat: "png",
		},
		"gemini-3-pro-image-preview": {
			KieAIModel:   getEnv("MODEL_NANO_BANANA_PRO", "nano-banana-pro"),
			AspectRatio:  "1:1",
			Resolution:   "1K",
			OutputFormat: "png",
		},
		"gemini-2.5-flash-image": {
			KieAIModel:   getEnv("MODEL_GOOGLE_NANO_BANANA", "google/nano-banana"),
			AspectRatio:  "1:1",
			Resolution:   "1K",
			OutputFormat: "png",
		},
	}
}

func validate(cfg *Config) error {
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("config: invalid SERVER_PORT %d", cfg.Server.Port)
	}
	if cfg.KieAI.BaseURL == "" {
		return fmt.Errorf("config: KIEAI_BASE_URL is required")
	}
	if cfg.Storage.BaseURL == "" {
		return fmt.Errorf("config: STORAGE_BASE_URL is required")
	}
	return nil
}
