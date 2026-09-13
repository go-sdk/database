package migrate

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-sdk/core/errx"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

type migrationRecord struct {
	ID   int64 `gorm:"primaryKey"`
	Name string
}

func TestNewSortsAndValidatesMigrations(t *testing.T) {
	db := openSQLite(t, filepath.Join(t.TempDir(), "sort.db"))
	migrator, err := New(db, Migrations{
		{ID: "20260913_120000_02_second", Up: func(*gorm.DB) error { return nil }},
		{ID: "20260913_120000_01_first", Up: func(*gorm.DB) error { return nil }},
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{migrator.migrations[0].ID, migrator.migrations[1].ID}
	if !slices.Equal(ids, []string{"20260913_120000_01_first", "20260913_120000_02_second"}) {
		t.Fatalf("migrations were not sorted: %#v", ids)
	}
	if _, err := New(db, Migrations{{ID: "20261340_256100_01_invalid", Up: func(*gorm.DB) error { return nil }}}); !errx.Is(err, ErrInvalidMigrationID) {
		t.Fatalf("expected invalid ID error, got %v", err)
	}
	if _, err := New(db, nil, WithLockTimeout(-time.Second)); !errx.Is(err, ErrInvalidLockTimeout) {
		t.Fatalf("expected invalid lock timeout error, got %v", err)
	}
}

func TestUpDownAndReset(t *testing.T) {
	db := openSQLite(t, filepath.Join(t.TempDir(), "migration.db"))
	migrations := Migrations{
		{
			ID: "20260913_120000_01_create_records",
			Up: func(tx *gorm.DB) error { return tx.AutoMigrate(&migrationRecord{}) },
			Down: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable(&migrationRecord{})
			},
		},
		{
			ID: "20260913_120000_02_optional_rollback",
			Up: func(tx *gorm.DB) error {
				return tx.Exec("CREATE INDEX IF NOT EXISTS idx_migration_records_name ON migration_records(name)").Error
			},
		},
	}
	migrator, err := New(db, migrations)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := migrator.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable(&migrationRecord{}) {
		t.Fatal("Up did not create table")
	}
	if err := migrator.Down(ctx); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable(&migrationRecord{}) {
		t.Fatal("empty Down should only remove the migration record")
	}
	if err := migrator.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable(&migrationRecord{}) {
		t.Fatal("Reset did not execute all rollbacks")
	}
	var count int64
	if db.Migrator().HasTable(TableName) {
		if err := db.Table(TableName).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
	}
	if count != 0 {
		t.Fatalf("Reset retained %d migration records", count)
	}
}

func TestMigrationLogsFailureSummary(t *testing.T) {
	db := openSQLite(t, filepath.Join(t.TempDir(), "failure.db"))
	migrator, err := New(db, Migrations{{
		ID: "20260913_120000_01_failure",
		Up: func(*gorm.DB) error {
			return errx.New("expected failure")
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	ctx := zerolog.New(&output).WithContext(context.Background())
	if err := migrator.Up(ctx); err == nil {
		t.Fatal("failed migration should return an error")
	}
	text := output.String()
	for _, expected := range []string{
		"database migration step started",
		"database migration step failed",
		"database migration summary",
		`"status":"failed"`,
		`"executed":1`,
		`"from":"none"`,
		`"to":"none"`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("migration log does not contain %q: %s", expected, text)
		}
	}
}

func TestSQLiteLockTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock-timeout.db")
	first := openSQLite(t, path)
	second := openSQLite(t, path)
	err := withLock(context.Background(), first, time.Second, func(*gorm.DB) error {
		return withLock(context.Background(), second, 0, func(*gorm.DB) error {
			t.Fatal("contending migration unexpectedly acquired the SQLite lock")
			return nil
		})
	})
	if !errx.Is(err, ErrLockTimeout) {
		t.Fatalf("expected SQLite lock timeout, got %v", err)
	}
}

func TestSQLiteLockRollsBackAfterContextCancellation(t *testing.T) {
	db := openSQLite(t, filepath.Join(t.TempDir(), "lock-cancel.db"))
	ctx, cancel := context.WithCancel(context.Background())
	err := withLock(ctx, db, time.Second, func(*gorm.DB) error {
		cancel()
		return ctx.Err()
	})
	if !errx.Is(err, context.Canceled) {
		t.Fatalf("expected canceled migration, got %v", err)
	}
	if err := withLock(context.Background(), db, 0, func(*gorm.DB) error { return nil }); err != nil {
		t.Fatalf("SQLite connection retained the canceled transaction: %v", err)
	}
}

func TestMigrationTableLookupReturnsErrors(t *testing.T) {
	db := openSQLite(t, filepath.Join(t.TempDir(), "lookup-error.db"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceledDB := db.WithContext(ctx)
	if _, err := currentVersion(canceledDB); !errx.Is(err, context.Canceled) {
		t.Fatalf("currentVersion should return the metadata query error, got %v", err)
	}
	migrator, err := New(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.appliedCount(canceledDB); !errx.Is(err, context.Canceled) {
		t.Fatalf("appliedCount should return the metadata query error, got %v", err)
	}
}

func openSQLite(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dsn+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
