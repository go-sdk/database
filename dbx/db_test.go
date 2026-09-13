package dbx_test

import (
	"path/filepath"
	"testing"

	"github.com/go-sdk/core/errx"
	"gorm.io/gorm"

	"github.com/go-sdk/database/dbx"
	_ "github.com/go-sdk/database/dbx/sqlite"
)

func TestOpenValidation(t *testing.T) {
	if _, err := dbx.Open("", "dsn"); !errx.Is(err, dbx.ErrDriverRequired) {
		t.Fatalf("expected driver error, got %v", err)
	}
	if _, err := dbx.Open("sqlite", ""); !errx.Is(err, dbx.ErrDSNRequired) {
		t.Fatalf("expected DSN error, got %v", err)
	}
	if _, err := dbx.Open("unknown", "dsn"); !errx.Is(err, dbx.ErrDriverNotRegistered) {
		t.Fatalf("expected unregistered driver error, got %v", err)
	}
}

func TestOpenAppliesSQLitePoolDefaults(t *testing.T) {
	db, err := dbx.Open("sqlite", filepath.Join(t.TempDir(), "default.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if got := sqlDB.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("unexpected SQLite max open connections: %d", got)
	}
}

func TestOpenOverridesPoolDefaults(t *testing.T) {
	db, err := dbx.Open("sqlite", filepath.Join(t.TempDir(), "override.db"), dbx.WithPoolConfig(dbx.PoolConfig{
		MaxIdleConns: 2,
		MaxOpenConns: 2,
	}))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if got := sqlDB.Stats().MaxOpenConnections; got != 2 {
		t.Fatalf("pool override was not applied: %d", got)
	}
}

func TestOpenAlwaysEnablesTranslateError(t *testing.T) {
	db, err := dbx.Open("sqlite", filepath.Join(t.TempDir(), "translate-error.db"), dbx.WithGORMConfig(&gorm.Config{
		TranslateError: false,
	}))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if !db.TranslateError {
		t.Fatal("TranslateError should always be enabled")
	}
}
