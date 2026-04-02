// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig             `yaml:"server"`
	KieAI        KieAIConfig              `yaml:"kieai"`
	Poller       PollerConfig             `yaml:"poller"`
	Storage      StorageConfig            `yaml:"storage"`
	ModelMapping map[string]ModelDefaults `yaml:"model_mapping"`
}

type ServerConfig struct {
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

type KieAIConfig struct {
	BaseURL string        `yaml:"base_url"`
	Timeout time.Duration `yaml:"timeout"`
}

type PollerConfig struct {
	InitialInterval time.Duration `yaml:"initial_interval"`
	MaxInterval     time.Duration `yaml:"max_interval"`
	MaxWaitTime     time.Duration `yaml:"max_wait_time"`
	RetryAttempts   int           `yaml:"retry_attempts"`
}

type StorageConfig struct {
	Type      string `yaml:"type"`
	LocalPath string `yaml:"local_path"`
	BaseURL   string `yaml:"base_url"`
}

type ModelDefaults struct {
	KieAIModel   string `yaml:"kieai_model"`
	AspectRatio  string `yaml:"aspect_ratio"`
	Resolution   string `yaml:"resolution"`
	OutputFormat string `yaml:"output_format"`
}

// Load reads and parses the YAML config file at path.
// Environment variable ${VAR} references in values are expanded.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read file %q: %w", path, err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("config: yaml unmarshal: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func validate(cfg *Config) error {
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("config: invalid server.port %d", cfg.Server.Port)
	}
	if cfg.KieAI.BaseURL == "" {
		return fmt.Errorf("config: kieai.base_url is required")
	}
	if len(cfg.ModelMapping) == 0 {
		return fmt.Errorf("config: model_mapping must not be empty")
	}
	return nil
}
