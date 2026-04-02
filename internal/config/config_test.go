// internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
server:
  port: 8080
  read_timeout: 130s
  write_timeout: 130s
kieai:
  base_url: https://api.kie.ai
  timeout: 120s
poller:
  initial_interval: 2s
  max_interval: 10s
  max_wait_time: 120s
  retry_attempts: 3
storage:
  type: local
  local_path: /tmp/images
  base_url: http://localhost:8080/images
model_mapping:
  gemini-3.1-flash-image-preview:
    kieai_model: nano-banana-2
    aspect_ratio: "1:1"
    resolution: "1K"
    output_format: png
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
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

func TestLoadConfig_EnvExpansion(t *testing.T) {
	t.Setenv("TEST_BASE_URL", "https://custom.kie.ai")
	dir := t.TempDir()
	yaml := `
server:
  port: 9090
kieai:
  base_url: ${TEST_BASE_URL}
  timeout: 30s
model_mapping:
  test-model:
    kieai_model: test-kieai
`
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.KieAI.BaseURL != "https://custom.kie.ai" {
		t.Errorf("env expansion failed: got %q", cfg.KieAI.BaseURL)
	}
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	dir := t.TempDir()
	yaml := `
server:
  port: 0
kieai:
  base_url: https://api.kie.ai
model_mapping:
  x:
    kieai_model: y
`
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(yaml), 0644)
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for port=0")
	}
}
