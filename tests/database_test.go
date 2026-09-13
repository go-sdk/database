package tests

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/go-sdk/core/errx"
	"gorm.io/gorm"

	"github.com/go-sdk/database/dbx"
	"github.com/go-sdk/database/dbx/datatype"
	"github.com/go-sdk/database/dbx/migrate"
	_ "github.com/go-sdk/database/dbx/sqlite"
	"github.com/go-sdk/database/tests/models"
)

func TestSQLiteMigrationDatatypeAndSoftDelete(t *testing.T) {
	db, err := dbx.Open("sqlite", filepath.Join(t.TempDir(), "integration.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	migrator, err := migrate.New(db, migrate.Migrations{{
		ID: "20260913_120000_01_create_users",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.User{})
		},
		Down: func(tx *gorm.DB) error { return tx.Migrator().DropTable(&models.User{}) },
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := migrator.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("repeated Up should be idempotent: %v", err)
	}

	user := models.User{
		Id:    "user-1",
		Name:  "alice",
		Email: "alice@example.com",
		Extra: datatype.JSON(`{"profile":{"language":"zh-CN"}}`),
	}
	if err := gorm.G[models.User](db).Create(ctx, &user); err != nil {
		t.Fatal(err)
	}
	found, err := gorm.G[models.User](db).Where("id = ?", user.Id).First(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := found.Extra.Get("profile.language").String(); got != "zh-CN" {
		t.Fatalf("unexpected JSON path value: %q", got)
	}
	if _, err := gorm.G[models.User](db).Where("id = ?", user.Id).Delete(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := gorm.G[models.User](db).Where("id = ?", user.Id).First(ctx); !errx.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("soft-deleted row should be hidden: %v", err)
	}
	var deleted models.User
	if err := db.WithContext(ctx).Unscoped().First(&deleted, "id = ?", user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if deleted.DeletedAt == 0 {
		t.Fatal("soft delete did not store a millisecond timestamp")
	}
}
