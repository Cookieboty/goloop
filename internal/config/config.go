package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server        ServerConfig
	JWT           JWTConfig
	Storage       StorageConfig
	Health        HealthConfig
	RateLimit     RateLimitConfig
	Redis         RedisConfig
	AdminPassword string
	DatabaseURL   string
}

type ServerConfig struct {
	Port                int
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	MaxRequestBodyBytes int64
}

type JWTConfig struct {
	Secret string
	Expiry time.Duration
}

type StorageConfig struct {
	Type            string
	LocalPath       string
	BaseURL         string
	DownloadTimeout time.Duration
	MaxImageBytes   int64
}

type HealthConfig struct {
	ProbeInterval     time.Duration
	ProbeTimeout      time.Duration
	RecoveryThreshold int
	RecoveryInterval  time.Duration // how long after last failure before resetting a hard-stopped channel
}

type RateLimitConfig struct {
	RPS   float64
	Burst int
}

type RedisConfig struct {
	URL           string
	Enabled       bool
	APIKeyCacheTTL time.Duration
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key, fallback string) time.Duration {
	d, err := time.ParseDuration(getEnv(key, fallback))
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

func getEnvBool(key string, fallback bool) bool {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		return fallback
	}
	return v
}

// Load reads configuration from environment variables.
//
// Channel configuration supports two formats:
//
// New format (recommended, supports multiple channels):
//
//	CHANNELS=kieai,gemini-direct
//	CHANNEL_KIEAI_TYPE=kieai
//	CHANNEL_KIEAI_BASE_URL=https://api.kie.ai
//	CHANNEL_KIEAI_TIMEOUT=120s
//	CHANNEL_KIEAI_ACCOUNTS=key1:50,key2:30,key3:20
//
// Legacy format (backward compatible, single kieai channel):
//
//	KIEAI_BASE_URL=https://api.kie.ai
//	KIEAI_KEY_1=xxx   KIEAI_WEIGHT_1=50
//	KIEAI_KEY_2=yyy   KIEAI_WEIGHT_2=30
//	... up to KIEAI_KEY_50
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:   getEnv("DATABASE_URL", ""),
		AdminPassword: getEnv("ADMIN_PASSWORD", ""),
		Server: ServerConfig{
			Port:                getEnvInt("SERVER_PORT", 8080),
			ReadTimeout:         getEnvDuration("SERVER_READ_TIMEOUT", "1300s"),
			WriteTimeout:        getEnvDuration("SERVER_WRITE_TIMEOUT", "1300s"),
			MaxRequestBodyBytes: int64(getEnvInt("SERVER_MAX_REQUEST_BODY_MB", 50)) * 1024 * 1024,
		},
		JWT: JWTConfig{
			Secret: getEnv("JWT_SECRET", "dev-secret-change-in-production"),
			Expiry: getEnvDuration("JWT_EXPIRY", "24h"),
		},
		Redis: RedisConfig{
			Enabled:        getEnvBool("REDIS_ENABLED", true),
			URL:            getEnv("REDIS_URL", ""),
			APIKeyCacheTTL: getEnvDuration("REDIS_APIKEY_CACHE_TTL", "5m"),
		},
		Storage: StorageConfig{
			Type:            getEnv("STORAGE_TYPE", "local"),
			LocalPath:       getEnv("STORAGE_LOCAL_PATH", "/tmp/images"),
			BaseURL:         getEnv("STORAGE_BASE_URL", ""),
			DownloadTimeout: getEnvDuration("STORAGE_DOWNLOAD_TIMEOUT", "120s"),
			MaxImageBytes:   int64(getEnvInt("STORAGE_MAX_IMAGE_MB", 30)) * 1024 * 1024,
		},
		Health: HealthConfig{
			ProbeInterval:     getEnvDuration("HEALTH_PROBE_INTERVAL", "30s"),
			ProbeTimeout:      getEnvDuration("HEALTH_PROBE_TIMEOUT", "5s"),
			RecoveryThreshold: getEnvInt("HEALTH_RECOVERY_THRESHOLD", 2),
			RecoveryInterval:  getEnvDuration("HEALTH_RECOVERY_INTERVAL", "30m"),
		},
		RateLimit: RateLimitConfig{
			RPS:   float64(getEnvInt("RATELIMIT_RPS", 0)),
			Burst: getEnvInt("RATELIMIT_BURST", 10),
		},
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validate(cfg *Config) error {
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("config: invalid SERVER_PORT %d", cfg.Server.Port)
	}
	if cfg.JWT.Secret == "" {
		return fmt.Errorf("config: JWT_SECRET is required")
	}
	if cfg.JWT.Secret == "dev-secret-change-in-production" {
		return fmt.Errorf("config: JWT_SECRET must be changed from default value")
	}
	if cfg.AdminPassword == "" {
		return fmt.Errorf("config: ADMIN_PASSWORD is required")
	}
	if len(cfg.AdminPassword) < 16 {
		return fmt.Errorf("config: ADMIN_PASSWORD must be at least 16 characters")
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("config: DATABASE_URL is required")
	}
	if cfg.Redis.Enabled && cfg.Redis.URL == "" {
		return fmt.Errorf("config: REDIS_URL is required when Redis is enabled")
	}
	return nil
}
