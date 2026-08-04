package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const validConfig = `
server:
  host: "127.0.0.1"
  port: 9090
auth:
  username: "admin"
  password: "somehash"
root_dir: "."
services:
  - name: "SvcA"
    display_name: "服务A"
log:
  path: "logs"
  max_size_mb: 5
  max_backups: 3
  max_age_days: 10
`

func TestLoadValid(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, validConfig)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port mismatch: %d", cfg.Server.Port)
	}
	if cfg.Auth.Username != "admin" {
		t.Errorf("username mismatch")
	}
	if len(cfg.Services) != 1 || cfg.Services[0].Name != "SvcA" {
		t.Errorf("services mismatch: %+v", cfg.Services)
	}
}

func TestLoadMissingAuth(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, `
server:
  host: "127.0.0.1"
  port: 9090
auth:
  username: ""
  password: ""
root_dir: "."
services:
  - name: "SvcA"
`)
	_, err := Load(path)
	if err == nil {
		t.Errorf("expected error for missing auth")
	}
}

func TestLoadBadPort(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, `
server:
  host: "127.0.0.1"
  port: 99999
auth:
  username: "a"
  password: "b"
root_dir: "."
services:
  - name: "SvcA"
`)
	_, err := Load(path)
	if err == nil {
		t.Errorf("expected error for bad port")
	}
}

func TestLoadNoServices(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, `
server:
  host: "127.0.0.1"
  port: 9090
auth:
  username: "a"
  password: "b"
root_dir: "."
services: []
`)
	_, err := Load(path)
	if err == nil {
		t.Errorf("expected error for no services")
	}
}

func TestFindService(t *testing.T) {
	cfg := &Config{
		Services: []ServiceEntry{
			{Name: "A", DisplayName: "Alpha"},
			{Name: "B", DisplayName: "Beta"},
		},
	}
	s, ok := cfg.FindService("B")
	if !ok || s.DisplayName != "Beta" {
		t.Errorf("FindService B failed: %+v ok=%v", s, ok)
	}
	if _, ok := cfg.FindService("C"); ok {
		t.Errorf("FindService C should not exist")
	}
}

func TestServiceNames(t *testing.T) {
	cfg := &Config{
		Services: []ServiceEntry{{Name: "A"}, {Name: "B"}},
	}
	names := cfg.ServiceNames()
	if len(names) != 2 || names[0] != "A" || names[1] != "B" {
		t.Errorf("ServiceNames: %v", names)
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, `
server:
  host: "127.0.0.1"
  port: 9090
auth:
  username: "a"
  password: "b"
root_dir: "."
services:
  - name: "SvcA"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Log.MaxSizeMB != 10 {
		t.Errorf("expected default max_size_mb=10, got %d", cfg.Log.MaxSizeMB)
	}
}

func TestLoadWithAPIToken(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, `
server:
  host: "127.0.0.1"
  port: 9090
auth:
  username: "a"
  password: "b"
  api_token: "my-secret-token"
root_dir: "."
services:
  - name: "SvcA"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.APIToken != "my-secret-token" {
		t.Errorf("api_token mismatch: %s", cfg.Auth.APIToken)
	}
}

func TestLoadWithRateLimit(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, `
server:
  host: "127.0.0.1"
  port: 9090
auth:
  username: "a"
  password: "b"
root_dir: "."
services:
  - name: "SvcA"
rate_limit:
  enabled: true
  max_requests: 30
  per_ip: false
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.RateLimit.Enabled {
		t.Errorf("rate_limit.enabled should be true")
	}
	if cfg.RateLimit.MaxRequests != 30 {
		t.Errorf("max_requests mismatch: %d", cfg.RateLimit.MaxRequests)
	}
	if cfg.RateLimit.PerIP {
		t.Errorf("per_ip should be false")
	}
}

func TestLoadRateLimitDefaults(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, validConfig)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RateLimit.Enabled {
		t.Errorf("rate_limit should be disabled by default")
	}
}
