package datatype

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"reflect"
	"strings"

	codecjson "github.com/go-sdk/core/codec/json"
	"github.com/go-sdk/core/errx"
	"github.com/spf13/cast"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// JSON 是可直接持久化并通过 gjson 路径读取的原始 JSON 文档。
type JSON json.RawMessage

var _ Field = (*JSON)(nil)

// Scan 从数据库读取 JSON，并复制驱动可能复用的字节缓冲区。
//
//goland:noinspection GoMixedReceiverTypes
func (x *JSON) Scan(value any) error {
	if value == nil {
		*x = JSON("null")
		return nil
	}
	text, err := cast.ToStringE(value)
	if err != nil {
		return errx.Wrapf(err, "scan JSON from %T", value)
	}
	bytes := []byte(text)
	if !json.Valid(bytes) {
		return errx.New("scan JSON: invalid JSON document")
	}
	*x = bytes
	return nil
}

func (x JSON) Value() (driver.Value, error) {
	if len(x) == 0 {
		return nil, nil
	}
	if !json.Valid(x) {
		return nil, errx.New("value JSON: invalid JSON document")
	}
	return string(x), nil
}

func (x JSON) MarshalJSON() ([]byte, error) {
	return json.RawMessage(x).MarshalJSON()
}

//goland:noinspection GoMixedReceiverTypes
func (x *JSON) UnmarshalJSON(bytes []byte) error {
	value := json.RawMessage{}
	if err := value.UnmarshalJSON(bytes); err != nil {
		return err
	}
	*x = JSON(value)
	return nil
}

func (x JSON) String() string { return string(x) }

// Valid 判断当前值是否为完整且合法的 JSON 文档。
func (x JSON) Valid() bool { return gjson.ValidBytes(x) }

// Parse 将完整文档解析为可继续查询的 gjson Result。
func (x JSON) Parse() gjson.Result { return gjson.ParseBytes(x) }

// Get 使用 gjson 路径语法读取文档中的值。
func (x JSON) Get(path string) gjson.Result { return gjson.GetBytes(x, path) }

// Unmarshal 使用 core/codec/json 将文档反序列化为类型 T，文档不完整或与 T 结构不匹配时返回错误，
// 失败时保持 T 的零值。
func (x JSON) Unmarshal[T any]() (t T, err error) {
	err = codecjson.Unmarshal([]byte(x), &t)
	return
}

// MustUnmarshal 行为与 Unmarshal 一致，但失败时 panic 并由 core/codec/json 统一打印调用堆栈，
// 仅适用于文档格式已在初始化阶段保证正确的场景。
func (x JSON) MustUnmarshal[T any]() T {
	return codecjson.MustUnmarshal[T]([]byte(x))
}

func (JSON) GormDataType() string { return "json" }

func (JSON) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Name() {
	case "postgres":
		return "JSONB"
	case "mysql", "sqlite":
		return "JSON"
	default:
		return ""
	}
}

func (x JSON) GormValue(_ context.Context, db *gorm.DB) clause.Expr {
	if len(x) == 0 {
		return gorm.Expr("NULL")
	}
	if db.Name() == "mysql" && !isMariaDB(db.Dialector) {
		return gorm.Expr("CAST(? AS JSON)", string(x))
	}
	return gorm.Expr("?", string(x))
}

// isMariaDB 读取 MySQL Dialector 初始化时保存的服务端版本，避免在 SQL 构建阶段访问数据库。
func isMariaDB(dialector gorm.Dialector) bool {
	if dialector == nil || dialector.Name() != "mysql" {
		return false
	}
	value := reflect.ValueOf(dialector)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return false
	}
	serverVersion := value.FieldByName("ServerVersion")
	return serverVersion.IsValid() && serverVersion.Kind() == reflect.String && strings.Contains(serverVersion.String(), "MariaDB")
}
