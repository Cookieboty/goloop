package config

import (
	"testing"
	"time"
)

const testAdminPassword = "test-admin-password-ok"

func TestLoad(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-for-config-test")
	t.Setenv("ADMIN_PASSWORD", testAdminPassword)
	t.Setenv("KIEAI_BASE_URL", "https://api.kie.ai")
	t.Setenv("STORAGE_BASE_URL", "http://localhost:8080/images")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("port: got %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 1300*time.Second {
		t.Errorf("read_timeout: got %v, want 1300s", cfg.Server.ReadTimeout)
	}
	if cfg.AdminPassword != testAdminPassword {
		t.Errorf("admin_password: got %q", cfg.AdminPassword)
	}
	if cfg.Channels["kieai"].BaseURL != "https://api.kie.ai" {
		t.Errorf("base_url: got %q", cfg.Channels["kieai"].BaseURL)
	}
	if cfg.Channels["kieai"].InitialInterval != 2*time.Second {
		t.Errorf("initial_interval: got %v", cfg.Channels["kieai"].InitialInterval)
	}
	if cfg.Channels["kieai"].RetryAttempts != 3 {
		t.Errorf("retry_attempts: got %d", cfg.Channels["kieai"].RetryAttempts)
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
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("ADMIN_PASSWORD", testAdminPassword)
	t.Setenv("KIEAI_BASE_URL", "https://custom.kie.ai")
	t.Setenv("STORAGE_BASE_URL", "http://custom:8080/images")
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("POLLER_RETRY_ATTEMPTS", "5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Channels["kieai"].BaseURL != "https://custom.kie.ai" {
		t.Errorf("got %q", cfg.Channels["kieai"].BaseURL)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port: got %d", cfg.Server.Port)
	}
	if cfg.Channels["kieai"].RetryAttempts != 5 {
		t.Errorf("retry_attempts: got %d", cfg.Channels["kieai"].RetryAttempts)
	}
}

func TestLoad_NewChannelFormat(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("ADMIN_PASSWORD", testAdminPassword)
	t.Setenv("CHANNELS", "kieai")
	t.Setenv("CHANNEL_KIEAI_TYPE", "kieai")
	t.Setenv("CHANNEL_KIEAI_BASE_URL", "https://api.kie.ai")
	t.Setenv("CHANNEL_KIEAI_ACCOUNTS", "key1:50,key2:30,key3")
	t.Cleanup(func() {
		t.Setenv("CHANNELS", "")
		t.Setenv("CHANNEL_KIEAI_BASE_URL", "")
		t.Setenv("CHANNEL_KIEAI_ACCOUNTS", "")
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	kieCh, ok := cfg.Channels["kieai"]
	if !ok {
		t.Fatal("kieai channel not loaded")
	}
	if kieCh.BaseURL != "https://api.kie.ai" {
		t.Errorf("base_url: got %q", kieCh.BaseURL)
	}
	if len(kieCh.Accounts) != 3 {
		t.Fatalf("expected 3 accounts, got %d", len(kieCh.Accounts))
	}
	if kieCh.Accounts[0].Weight != 50 {
		t.Errorf("account[0] weight: got %d, want 50", kieCh.Accounts[0].Weight)
	}
	if kieCh.Accounts[2].Weight != 100 {
		t.Errorf("account[2] weight (default): got %d, want 100", kieCh.Accounts[2].Weight)
	}
}

func TestLoad_MissingAdminPassword(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("ADMIN_PASSWORD", "")

	_, err := Load()
	if err == nil {
		t.Error("expected error when ADMIN_PASSWORD is missing")
	}
}

func TestLoad_ShortAdminPassword(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("ADMIN_PASSWORD", "short")

	_, err := Load()
	if err == nil {
		t.Error("expected error when ADMIN_PASSWORD is too short")
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("ADMIN_PASSWORD", testAdminPassword)
	t.Setenv("KIEAI_BASE_URL", "https://api.kie.ai")
	t.Setenv("SERVER_PORT", "0")

	_, err := Load()
	if err == nil {
		t.Error("expected error for port=0")
	}
}
