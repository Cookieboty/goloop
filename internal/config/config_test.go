// internal/config/config_test.go
package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Setenv("KIEAI_BASE_URL", "https://api.kie.ai")
	t.Setenv("STORAGE_BASE_URL", "http://localhost:8080/images")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("port: got %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 130*time.Second {
		t.Errorf("read_timeout: got %v, want 130s", cfg.Server.ReadTimeout)
	}
	if cfg.KieAI.BaseURL != "https://api.kie.ai" {
		t.Errorf("base_url: got %q", cfg.KieAI.BaseURL)
	}
	if cfg.Poller.InitialInterval != 2*time.Second {
		t.Errorf("initial_interval: got %v", cfg.Poller.InitialInterval)
	}
	if cfg.Poller.RetryAttempts != 3 {
		t.Errorf("retry_attempts: got %d", cfg.Poller.RetryAttempts)
	}
	m, ok := cfg.ModelMapping["gemini-3.1-flash-image-preview"]
	if !ok {
		t.Fatal("model mapping missing gemini-3.1-flash-image-preview")
	}
	if m.KieAIModel != "nano-banana-2" {
		t.Errorf("kieai_model: got %q", m.KieAIModel)
	}
	if m.AspectRatio != "1:1" {
		t.Errorf("aspect_ratio: got %q", m.AspectRatio)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("KIEAI_BASE_URL", "https://custom.kie.ai")
	t.Setenv("STORAGE_BASE_URL", "http://custom:8080/images")
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("POLLER_RETRY_ATTEMPTS", "5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.KieAI.BaseURL != "https://custom.kie.ai" {
		t.Errorf("got %q", cfg.KieAI.BaseURL)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port: got %d", cfg.Server.Port)
	}
	if cfg.Poller.RetryAttempts != 5 {
		t.Errorf("retry_attempts: got %d", cfg.Poller.RetryAttempts)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	t.Setenv("KIEAI_BASE_URL", "")
	t.Setenv("STORAGE_BASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Error("expected error when KIEAI_BASE_URL is missing")
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	t.Setenv("KIEAI_BASE_URL", "https://api.kie.ai")
	t.Setenv("STORAGE_BASE_URL", "http://localhost:8080/images")
	t.Setenv("SERVER_PORT", "0")

	_, err := Load()
	if err == nil {
		t.Error("expected error for port=0")
	}
}
