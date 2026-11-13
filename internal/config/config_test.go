package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaultsWhenMissing(t *testing.T) {
	t.Setenv("REBANG_CONFIG_DIR", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.API() != DefaultAPIBase || cfg.Theme != "auto" {
		t.Fatalf("unexpected %+v", cfg)
	}
	d, _ := cfg.Refresh()
	if d != DefaultRefreshInterval {
		t.Fatalf("refresh %v", d)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REBANG_CONFIG_DIR", dir)
	cfg := Default()
	cfg.Output = "md"
	cfg.AutoUpdate = true
	cfg.RefreshInterval = "2m"
	cfg.APIBase = "http://127.0.0.1:9898/"
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Output != "md" || !got.AutoUpdate || got.API() != "http://127.0.0.1:9898" {
		t.Fatalf("got %+v", got)
	}
	if d, _ := got.Refresh(); d != 2*time.Minute {
		t.Fatalf("refresh %v", d)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Fatal(err)
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("REBANG_CONFIG_DIR", t.TempDir())
	t.Setenv("REBANG_API_BASE", "http://localhost:1")
	t.Setenv("REBANG_MANIFEST_URL", "http://localhost:2/m.json")
	cfg := Default()
	cfg.APIBase = "http://ignored"
	if cfg.API() != "http://localhost:1" || cfg.Manifest() != "http://localhost:2/m.json" {
		t.Fatal("env should win")
	}
}

func TestValidate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REBANG_CONFIG_DIR", dir)
	os.WriteFile(filepath.Join(dir, FileName), []byte(`output = "xml"`), 0o600)
	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
	c := Default()
	c.RefreshInterval = "0"
	if d, err := c.Refresh(); err != nil || d != 0 {
		t.Fatal("0 disables refresh")
	}
	c.RefreshInterval = "1s"
	if d, _ := c.Refresh(); d != 30*time.Second {
		t.Fatal("min clamp")
	}
}
