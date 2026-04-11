package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server       ServerConfig
	JWT          JWTConfig
	Storage      StorageConfig
	Health       HealthConfig
	Channels     map[string]ChannelConfig
	ModelMapping map[string]ModelDefaults
}

type ServerConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type JWTConfig struct {
	Secret string
	Expiry time.Duration
}

type StorageConfig struct {
	Type      string
	LocalPath string
	BaseURL   string
}

type HealthConfig struct {
	ProbeInterval     time.Duration
	ProbeTimeout     time.Duration
	RecoveryThreshold int
}

type ChannelConfig struct {
	Type            string
	BaseURL         string
	Timeout         time.Duration
	InitialInterval time.Duration
	MaxInterval     time.Duration
	MaxWaitTime     time.Duration
	RetryAttempts   int
	Accounts        []AccountConfig
}

type AccountConfig struct {
	APIKey string
	Weight int
}

type ModelDefaults struct {
	Channel     string
	KieAIModel  string
	AspectRatio string
	Resolution  string
	OutputFormat string
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" { return v }
	return fallback
}

func getEnvDuration(key, fallback string) time.Duration {
	d, err := time.ParseDuration(getEnv(key, fallback))
	if err != nil { d, _ = time.ParseDuration(fallback) }
	return d
}

func getEnvInt(key string, fallback int) int {
	s := os.Getenv(key)
	if s == "" { return fallback }
	n, err := strconv.Atoi(s)
	if err != nil { return fallback }
	return n
}

// Load reads from environment variables.
func Load() (*Config, error) {
	jwtSecret := getEnv("JWT_SECRET", "")
	if jwtSecret == "" {
		return nil, fmt.Errorf("config: JWT_SECRET is required")
	}

	cfg := &Config{
		Server: ServerConfig{
			Port:         getEnvInt("SERVER_PORT", 8080),
			ReadTimeout:  getEnvDuration("SERVER_READ_TIMEOUT", "130s"),
			WriteTimeout: getEnvDuration("SERVER_WRITE_TIMEOUT", "130s"),
		},
		JWT: JWTConfig{
			Secret: jwtSecret,
			Expiry: getEnvDuration("JWT_EXPIRY", "24h"),
		},
		Storage: StorageConfig{
			Type:      getEnv("STORAGE_TYPE", "local"),
			LocalPath: getEnv("STORAGE_LOCAL_PATH", "/tmp/images"),
			BaseURL:   getEnv("STORAGE_BASE_URL", ""),
		},
		Health: HealthConfig{
			ProbeInterval:     getEnvDuration("HEALTH_PROBE_INTERVAL", "30s"),
			ProbeTimeout:     getEnvDuration("HEALTH_PROBE_TIMEOUT", "5s"),
			RecoveryThreshold: getEnvInt("HEALTH_RECOVERY_THRESHOLD", 2),
		},
		ModelMapping: map[string]ModelDefaults{
			"gemini-3.1-flash-image-preview": {
				Channel:     "kieai",
				KieAIModel: getEnv("MODEL_NANO_BANANA_2", "nano-banana-2"),
				AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png",
			},
			"gemini-3-pro-image-preview": {
				Channel:     "kieai",
				KieAIModel: getEnv("MODEL_NANO_BANANA_PRO", "nano-banana-pro"),
				AspectRatio: "1:1", Resolution: "2K", OutputFormat: "png",
			},
			"gemini-2.5-flash-image": {
				Channel:     "kieai",
				KieAIModel: getEnv("MODEL_GOOGLE_NANO_BANANA", "google/nano-banana"),
				AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png",
			},
		},
	}

	// Build channel configs from env
	kieBaseURL := getEnv("KIEAI_BASE_URL", "")
	if kieBaseURL != "" {
		kieCh := ChannelConfig{
			Type:            "kieai",
			BaseURL:         kieBaseURL,
			Timeout:         getEnvDuration("KIEAI_TIMEOUT", "120s"),
			InitialInterval: getEnvDuration("POLLER_INITIAL_INTERVAL", "2s"),
			MaxInterval:     getEnvDuration("POLLER_MAX_INTERVAL", "10s"),
			MaxWaitTime:     getEnvDuration("POLLER_MAX_WAIT_TIME", "120s"),
			RetryAttempts:   getEnvInt("POLLER_RETRY_ATTEMPTS", 3),
		}
		// Read account keys from env: KIEAI_KEY_1, KIEAI_KEY_2, ...
		for i := 1; i <= 10; i++ {
			key := os.Getenv(fmt.Sprintf("KIEAI_KEY_%d", i))
			if key == "" { continue }
			weight := getEnvInt(fmt.Sprintf("KIEAI_WEIGHT_%d", i), 100)
			kieCh.Accounts = append(kieCh.Accounts,
				AccountConfig{APIKey: key, Weight: weight})
		}
		cfg.Channels = map[string]ChannelConfig{"kieai": kieCh}
	}

	if len(cfg.Channels) == 0 {
		return nil, fmt.Errorf("config: at least one channel must be configured (set KIEAI_BASE_URL)")
	}

	return cfg, nil
}

func validate(cfg *Config) error {
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid SERVER_PORT %d", cfg.Server.Port)
	}
	if cfg.JWT.Secret == "" {
		return fmt.Errorf("config: JWT_SECRET is required")
	}
	return nil
}