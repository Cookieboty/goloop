package config

import (
	"testing"
	"time"
)

const testAdminPassword = "test-admin-password-ok"

func TestLoad(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-for-config-test")
	t.Setenv("ADMIN_PASSWORD", testAdminPassword)
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("REDIS_ENABLED", "false")

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
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("ADMIN_PASSWORD", testAdminPassword)
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("REDIS_ENABLED", "false")
	t.Setenv("SERVER_PORT", "9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port: got %d", cfg.Server.Port)
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
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("SERVER_PORT", "0")

	_, err := Load()
	if err == nil {
		t.Error("expected error for port=0")
	}
}
