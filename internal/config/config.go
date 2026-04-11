package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server        ServerConfig
	JWT           JWTConfig
	Storage       StorageConfig
	Health        HealthConfig
	AdminPassword string
	Channels      map[string]ChannelConfig
	ModelMapping  map[string]ModelDefaults
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
	ProbeTimeout      time.Duration
	RecoveryThreshold int
}

type ChannelConfig struct {
	Type            string
	BaseURL         string
	Weight          int
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
	Channel      string
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

// channelEnvPrefix converts a channel name to its environment variable prefix.
// e.g. "kieai" -> "CHANNEL_KIEAI_", "gemini-direct" -> "CHANNEL_GEMINI_DIRECT_"
func channelEnvPrefix(name string) string {
	upper := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	return "CHANNEL_" + upper + "_"
}

// parseAccounts parses the ACCOUNTS env value: "key1:50,key2:30,key3"
// Each entry is apikey:weight; weight defaults to 100 if omitted.
func parseAccounts(raw string) []AccountConfig {
	var accounts []AccountConfig
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 2)
		apiKey := strings.TrimSpace(parts[0])
		if apiKey == "" {
			continue
		}
		weight := 100
		if len(parts) == 2 {
			if w, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && w > 0 {
				weight = w
			}
		}
		accounts = append(accounts, AccountConfig{APIKey: apiKey, Weight: weight})
	}
	return accounts
}

// loadChannelFromEnv loads a single channel config using the CHANNEL_<NAME>_* prefix.
func loadChannelFromEnv(name string) (ChannelConfig, bool) {
	pfx := channelEnvPrefix(name)
	baseURL := os.Getenv(pfx + "BASE_URL")
	if baseURL == "" {
		return ChannelConfig{}, false
	}
	chType := getEnv(pfx+"TYPE", name)
	accountsRaw := os.Getenv(pfx + "ACCOUNTS")
	accounts := parseAccounts(accountsRaw)

	return ChannelConfig{
		Type:            chType,
		BaseURL:         baseURL,
		Weight:          getEnvInt(pfx+"WEIGHT", 100),
		Timeout:         getEnvDuration(pfx+"TIMEOUT", "120s"),
		InitialInterval: getEnvDuration(pfx+"INITIAL_INTERVAL", "2s"),
		MaxInterval:     getEnvDuration(pfx+"MAX_INTERVAL", "10s"),
		MaxWaitTime:     getEnvDuration(pfx+"MAX_WAIT_TIME", "120s"),
		RetryAttempts:   getEnvInt(pfx+"RETRY_ATTEMPTS", 3),
		Accounts:        accounts,
	}, true
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
		AdminPassword: getEnv("ADMIN_PASSWORD", ""),
		Server: ServerConfig{
			Port:         getEnvInt("SERVER_PORT", 8080),
			ReadTimeout:  getEnvDuration("SERVER_READ_TIMEOUT", "130s"),
			WriteTimeout: getEnvDuration("SERVER_WRITE_TIMEOUT", "130s"),
		},
		JWT: JWTConfig{
			Secret: getEnv("JWT_SECRET", "dev-secret-change-in-production"),
			Expiry: getEnvDuration("JWT_EXPIRY", "24h"),
		},
		Storage: StorageConfig{
			Type:      getEnv("STORAGE_TYPE", "local"),
			LocalPath: getEnv("STORAGE_LOCAL_PATH", "/tmp/images"),
			BaseURL:   getEnv("STORAGE_BASE_URL", ""),
		},
		Health: HealthConfig{
			ProbeInterval:     getEnvDuration("HEALTH_PROBE_INTERVAL", "30s"),
			ProbeTimeout:      getEnvDuration("HEALTH_PROBE_TIMEOUT", "5s"),
			RecoveryThreshold: getEnvInt("HEALTH_RECOVERY_THRESHOLD", 2),
		},
		ModelMapping: map[string]ModelDefaults{
			"gemini-3.1-flash-image-preview": {
				Channel:      "kieai",
				KieAIModel:   getEnv("MODEL_NANO_BANANA_2", "nano-banana-2"),
				AspectRatio:  "1:1",
				Resolution:   "1K",
				OutputFormat: "png",
			},
			"gemini-3-pro-image-preview": {
				Channel:      "kieai",
				KieAIModel:   getEnv("MODEL_NANO_BANANA_PRO", "nano-banana-pro"),
				AspectRatio:  "1:1",
				Resolution:   "2K",
				OutputFormat: "png",
			},
			"gemini-2.5-flash-image": {
				Channel:      "kieai",
				KieAIModel:   getEnv("MODEL_GOOGLE_NANO_BANANA", "google/nano-banana"),
				AspectRatio:  "1:1",
				Resolution:   "1K",
				OutputFormat: "png",
			},
		},
	}

	cfg.Channels = make(map[string]ChannelConfig)

	// New multi-channel format: CHANNELS=kieai,gemini-direct
	if channelsEnv := os.Getenv("CHANNELS"); channelsEnv != "" {
		for _, name := range strings.Split(channelsEnv, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			chCfg, ok := loadChannelFromEnv(name)
			if !ok {
				return nil, fmt.Errorf("config: channel %q listed in CHANNELS but CHANNEL_%s_BASE_URL is not set",
					name, strings.ToUpper(strings.ReplaceAll(name, "-", "_")))
			}
			cfg.Channels[name] = chCfg
		}
	} else {
		// Legacy format: KIEAI_BASE_URL + KIEAI_KEY_N (up to 50 accounts)
		kieBaseURL := getEnv("KIEAI_BASE_URL", "")
		if kieBaseURL != "" {
			kieCh := ChannelConfig{
				Type:            "kieai",
				BaseURL:         kieBaseURL,
				Weight:          getEnvInt("KIEAI_WEIGHT", 100),
				Timeout:         getEnvDuration("KIEAI_TIMEOUT", "120s"),
				InitialInterval: getEnvDuration("POLLER_INITIAL_INTERVAL", "2s"),
				MaxInterval:     getEnvDuration("POLLER_MAX_INTERVAL", "10s"),
				MaxWaitTime:     getEnvDuration("POLLER_MAX_WAIT_TIME", "120s"),
				RetryAttempts:   getEnvInt("POLLER_RETRY_ATTEMPTS", 3),
			}
			for i := 1; i <= 50; i++ {
				key := os.Getenv(fmt.Sprintf("KIEAI_KEY_%d", i))
				if key == "" {
					continue
				}
				weight := getEnvInt(fmt.Sprintf("KIEAI_WEIGHT_%d", i), 100)
				kieCh.Accounts = append(kieCh.Accounts, AccountConfig{APIKey: key, Weight: weight})
			}
			cfg.Channels["kieai"] = kieCh
		}
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
	if cfg.AdminPassword == "" {
		return fmt.Errorf("config: ADMIN_PASSWORD is required")
	}
	if len(cfg.AdminPassword) < 16 {
		return fmt.Errorf("config: ADMIN_PASSWORD must be at least 16 characters")
	}
	return nil
}
