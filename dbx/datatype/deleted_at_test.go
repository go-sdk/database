package datatype

import (
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	codecjson "github.com/go-sdk/core/codec/json"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type deletedModel struct {
	ID        string `gorm:"primaryKey"`
	DeletedAt DeletedAt
}

func TestDeletedAtValueAndJSON(t *testing.T) {
	var value DeletedAt
	if err := value.Scan([]byte("1723456789123")); err != nil {
		t.Fatal(err)
	}
	if value != 1723456789123 {
		t.Fatalf("unexpected scanned value: %d", value)
	}
	bytes, err := codecjson.Marshal[[]byte](value)
	if err != nil || string(bytes) != "1723456789123" {
		t.Fatalf("unexpected JSON value: %s, %v", bytes, err)
	}
	if err := codecjson.Unmarshal([]byte("null"), &value); err != nil || value != 0 {
		t.Fatalf("null should reset DeletedAt: %d, %v", value, err)
	}
	if err := value.Scan(uint32(42)); err != nil || value != 42 {
		t.Fatalf("cast should convert supported numeric values: %d, %v", value, err)
	}
}

func TestDeletedAtClauses(t *testing.T) {
	now := time.UnixMilli(1723456789123)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{NowFunc: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}

	query := db.Session(&gorm.Session{DryRun: true}).Where("id = ? OR id = ?", "1", "2").Find(&[]deletedModel{})
	if query.Error != nil || !containsAll(query.Statement.SQL.String(), "deleted_at", "= ?") {
		t.Fatalf("query does not contain soft-delete condition: %s, %v", query.Statement.SQL.String(), query.Error)
	}

	model := deletedModel{ID: "1"}
	deleted := db.Session(&gorm.Session{DryRun: true}).Delete(&model)
	if deleted.Error != nil || !containsAll(deleted.Statement.SQL.String(), "UPDATE", "deleted_at", "WHERE") {
		t.Fatalf("unexpected soft-delete SQL: %s, %v", deleted.Statement.SQL.String(), deleted.Error)
	}
	if model.DeletedAt != DeletedAt(now.UnixMilli()) {
		t.Fatalf("model was not assigned millisecond timestamp: %d", model.DeletedAt)
	}

	physical := db.Session(&gorm.Session{DryRun: true}).Unscoped().Delete(&model)
	if physical.Error != nil || !containsAll(physical.Statement.SQL.String(), "DELETE FROM") {
		t.Fatalf("unexpected physical-delete SQL: %s, %v", physical.Statement.SQL.String(), physical.Error)
	}
}

func TestDeletedAtDatabaseTypes(t *testing.T) {
	tests := []struct {
		name      string
		dialector gorm.Dialector
		want      string
	}{
		{name: "mysql", dialector: mysql.Open(""), want: "BIGINT"},
		{name: "postgres", dialector: postgres.Open(""), want: "BIGINT"},
		{name: "sqlite", dialector: sqlite.Open(":memory:"), want: "INTEGER"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := &gorm.DB{Config: &gorm.Config{Dialector: test.dialector}}
			if got := (DeletedAt)(0).GormDBDataType(db, nil); got != test.want {
				t.Fatalf("unexpected database type: %q", got)
			}
		})
	}
}

func containsAll(value string, values ...string) bool {
	for _, item := range values {
		if !strings.Contains(value, item) {
			return false
		}
	}
	return true
}
