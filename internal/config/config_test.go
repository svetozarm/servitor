package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidFullConfig(t *testing.T) {
	content := `
server:
  port: 9090
  host: 0.0.0.0
frontend:
  path: ./frontend
backend:
  path: ./backend/bin
  api_prefix: /api
  timeout: 5s
sync:
  enabled: true
  inactivity_delay: 10s
  max_interval: 2m
`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Frontend.Path != "./frontend" {
		t.Errorf("expected frontend path ./frontend, got %s", cfg.Frontend.Path)
	}
	if cfg.Addr() != "0.0.0.0:9090" {
		t.Errorf("expected addr 0.0.0.0:9090, got %s", cfg.Addr())
	}
}

func TestLoad_MinimalConfig_DefaultsApplied(t *testing.T) {
	content := `
frontend:
  path: ./public
backend:
  path: ./bin/app
  api_prefix: /api
sync:
  enabled: false
`
	path := writeTempConfig(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected default host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoad_MissingRequiredField(t *testing.T) {
	content := `
backend:
  path: ./bin
  api_prefix: /api
`
	path := writeTempConfig(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing frontend.path")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTempConfig(t, "{{invalid yaml")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	for _, port := range []int{-1, 70000} {
		content := `
server:
  port: ` + itoa(port) + `
frontend:
  path: ./f
backend:
  path: ./b
  api_prefix: /api
sync:
  enabled: false
`
		path := writeTempConfig(t, content)
		_, err := Load(path)
		if err == nil {
			t.Errorf("expected error for port %d", port)
		}
	}
}

func TestLoad_SyncValidation_MaxIntervalLessThanDelay(t *testing.T) {
	content := `
frontend:
  path: ./f
backend:
  path: ./b
  api_prefix: /api
sync:
  enabled: true
  inactivity_delay: 1m
  max_interval: 30s
`
	path := writeTempConfig(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error when max_interval < inactivity_delay")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/.servitor.conf")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".servitor.conf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
