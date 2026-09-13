package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-sdk/core/errx"
	"github.com/go-sdk/core/seq"
	sqlmysql "github.com/go-sql-driver/mysql"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/go-sdk/database/dbx"
	"github.com/go-sdk/database/dbx/datatype"
	"github.com/go-sdk/database/dbx/migrate"
	_ "github.com/go-sdk/database/dbx/mysql"
	_ "github.com/go-sdk/database/dbx/postgres"
	_ "github.com/go-sdk/database/dbx/sqlite"
	"github.com/go-sdk/database/tests/modelg"
	"github.com/go-sdk/database/tests/models"
)

// crudFlow 在目标数据库上执行建表迁移，并对 users 和 bills 表完成增删改查断言。
func crudFlow(t *testing.T, driverName, dsn string) {
	t.Helper()

	ctx := context.Background()
	db, err := dbx.Open(driverName, dsn)
	if err != nil {
		t.Fatalf("open %s database: %v", driverName, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	migrator, err := migrate.New(db, migrate.Migrations{{
		ID: "20260913_120000_01_create_example_tables",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.User{}, &models.Bill{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&models.Bill{}, &models.User{})
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Up(ctx); err != nil {
		t.Fatal(err)
	}

	// Metadata 主键没有数据库默认值，每次执行生成全局唯一 id
	userId := seq.NextID()
	billId := seq.NextID()

	// 增
	user := models.User{
		Id:    userId,
		Name:  "alice",
		Email: "alice@example.com",
		Extra: datatype.JSON(`{"profile":{"language":"zh-CN"}}`),
	}
	if err := gorm.G[models.User](db).Create(ctx, &user); err != nil {
		t.Fatal(err)
	}
	bill := models.Bill{
		Id:     billId,
		Amount: decimal.NewFromFloat(12.5),
	}
	if err := gorm.G[models.Bill](db).Create(ctx, &bill); err != nil {
		t.Fatal(err)
	}

	// 查
	found, err := gorm.G[models.User](db).Where(modelg.User.Id.Eq(userId)).First(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := found.Extra.Get("profile.language").String(); got != "zh-CN" {
		t.Fatalf("unexpected JSON path value: %q", got)
	}
	foundBill, err := gorm.G[models.Bill](db).Where(modelg.Bill.Id.Eq(billId)).First(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !foundBill.Amount.Equal(decimal.NewFromFloat(12.5)) {
		t.Fatalf("unexpected bill amount: %s", foundBill.Amount.String())
	}

	// 改
	rows, err := gorm.G[models.User](db).Where(modelg.User.Id.Eq(userId)).Update(ctx, "email", "alice@example.org")
	if err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("update user affected %d rows", rows)
	}
	rows, err = gorm.G[models.Bill](db).Where(modelg.Bill.Id.Eq(billId)).Set(modelg.Bill.Amount.Set(decimal.NewFromFloat(99.9))).Update(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("update bill affected %d rows", rows)
	}
	updatedUser, err := gorm.G[models.User](db).Where(modelg.User.Id.Eq(userId)).First(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if updatedUser.Email != "alice@example.org" {
		t.Fatalf("unexpected user email: %s", updatedUser.Email)
	}
	updatedBill, err := gorm.G[models.Bill](db).Where(modelg.Bill.Id.Eq(billId)).First(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !updatedBill.Amount.Equal(decimal.NewFromFloat(99.9)) {
		t.Fatalf("unexpected bill amount: %s", updatedBill.Amount.String())
	}

	// 删
	rows, err = gorm.G[models.User](db).Where(modelg.User.Id.Eq(userId)).Delete(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("delete user affected %d rows", rows)
	}
	rows, err = gorm.G[models.Bill](db).Where(modelg.Bill.Id.Eq(billId)).Delete(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("delete bill affected %d rows", rows)
	}
	if _, err := gorm.G[models.User](db).Where(modelg.User.Id.Eq(userId)).First(ctx); !errx.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("soft-deleted user should be hidden, got err: %v", err)
	}
	var deletedBill models.Bill
	if err := db.WithContext(ctx).Unscoped().First(&deletedBill, "id = ?", billId).Error; err != nil {
		t.Fatal(err)
	}
	if deletedBill.DeletedAt == 0 {
		t.Fatal("soft delete did not store a millisecond timestamp")
	}
}

func TestMySQL(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not set")
	}
	crudFlow(t, "mysql", dsn)
}

func TestMySQLMigrationRequiresSelectedDatabase(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not set")
	}
	config, err := sqlmysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.DBName = ""
	db, err := dbx.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrator, err := migrate.New(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Down(context.Background()); !errx.Is(err, migrate.ErrDatabaseNotSelected) {
		t.Fatalf("expected database selection error, got %v", err)
	}
}

// TestMariaDB MariaDB 兼容 MySQL 协议，复用 mysql 驱动。
func TestMariaDB(t *testing.T) {
	dsn := os.Getenv("TEST_MARIADB_DSN")
	if dsn == "" {
		t.Skip("TEST_MARIADB_DSN is not set")
	}
	crudFlow(t, "mysql", dsn)
}

func TestPostgreSQL(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	crudFlow(t, "postgres", dsn)
}

func TestSQLite(t *testing.T) {
	crudFlow(t, "sqlite", filepath.Join(t.TempDir(), "test.db"))
}
