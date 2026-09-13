package datatype

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	codecjson "github.com/go-sdk/core/codec/json"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestJSON(t *testing.T) {
	value := JSON(`{"profile":{"name":"alice"},"enabled":true}`)
	if !value.Valid() {
		t.Fatal("JSON should be valid")
	}
	if got := value.Get("profile.name").String(); got != "alice" {
		t.Fatalf("unexpected path value: %q", got)
	}
	if !value.Parse().Get("enabled").Bool() {
		t.Fatal("parsed JSON should expose nested values")
	}
	databaseValue, err := value.Value()
	if err != nil || databaseValue != value.String() {
		t.Fatalf("unexpected database value: %#v, %v", databaseValue, err)
	}
	bytes, err := codecjson.Marshal[[]byte](value)
	if err != nil || string(bytes) != value.String() {
		t.Fatalf("unexpected JSON output: %s, %v", bytes, err)
	}
}

func TestJSONScanCopiesBytes(t *testing.T) {
	source := []byte(`{"name":"alice"}`)
	var value JSON
	if err := value.Scan(source); err != nil {
		t.Fatal(err)
	}
	source[9] = 'b'
	if got := value.Get("name").String(); got != "alice" {
		t.Fatalf("scanner retained driver buffer: %q", got)
	}
}

func TestJSONRejectsInvalidDocument(t *testing.T) {
	var value JSON
	if err := value.Scan([]byte(`{"name":`)); err == nil {
		t.Fatal("Scan should reject invalid JSON")
	}
	if _, err := JSON(`{"name":`).Value(); err == nil {
		t.Fatal("Value should reject invalid JSON")
	}
}

func TestJSONNull(t *testing.T) {
	var value JSON
	if err := value.Scan(nil); err != nil || value.String() != "null" {
		t.Fatalf("unexpected NULL scan: %q, %v", value, err)
	}
	var empty JSON
	databaseValue, err := empty.Value()
	if err != nil || databaseValue != nil {
		t.Fatalf("empty JSON should become SQL NULL: %#v, %v", databaseValue, err)
	}
}

func TestJSONDatabaseTypes(t *testing.T) {
	tests := []struct {
		name      string
		dialector gorm.Dialector
		want      string
	}{
		{name: "mysql", dialector: mysql.Open(""), want: "JSON"},
		{name: "postgres", dialector: postgres.Open(""), want: "JSONB"},
		{name: "sqlite", dialector: sqlite.Open(":memory:"), want: "JSON"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := &gorm.DB{Config: &gorm.Config{Dialector: test.dialector}}
			if got := (JSON{}).GormDBDataType(db, nil); got != test.want {
				t.Fatalf("unexpected database type: %q", got)
			}
		})
	}
}

func TestJSONGormValueForMySQLVariants(t *testing.T) {
	tests := []struct {
		name          string
		serverVersion string
		wantSQL       string
	}{
		{name: "mysql", serverVersion: "8.4.0", wantSQL: "CAST(? AS JSON)"},
		{name: "mariadb", serverVersion: "10.11.15-MariaDB", wantSQL: "?"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dialector := mysql.New(mysql.Config{ServerVersion: test.serverVersion})
			db := &gorm.DB{Config: &gorm.Config{Dialector: dialector}}
			expression := JSON(`{"name":"alice"}`).GormValue(context.Background(), db)
			if expression.SQL != test.wantSQL {
				t.Fatalf("unexpected SQL expression: %q", expression.SQL)
			}
			if len(expression.Vars) != 1 || expression.Vars[0] != `{"name":"alice"}` {
				t.Fatalf("unexpected SQL variables: %#v", expression.Vars)
			}
		})
	}
}
