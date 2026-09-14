package dbx

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	coreconfig "github.com/go-sdk/core/config"
)

func TestConfiguredPoolConfig(t *testing.T) {
	previous := coreconfig.New()
	if err := previous.Load(); err != nil {
		t.Fatal(err)
	}
	defer coreconfig.SetDefault(previous)

	filename := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(filename, []byte(`
database:
  pool:
    max_idle_conns: 20
    max_open_conns: 200
    conn_max_lifetime: 15m
    conn_max_idle_time: 2m
  sqlite:
    pool:
      max_open_conns: 3
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := coreconfig.New(coreconfig.WithFile(filename))
	if err := cfg.Load(); err != nil {
		t.Fatal(err)
	}
	coreconfig.SetDefault(cfg)

	got := configuredPoolConfig("sqlite", PoolConfig{})
	want := PoolConfig{
		MaxIdleConns:    20,
		MaxOpenConns:    3,
		ConnMaxLifetime: 15 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}
	if got != want {
		t.Fatalf("unexpected configured pool: got %+v, want %+v", got, want)
	}
}
