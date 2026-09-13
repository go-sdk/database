package datatype

import (
	"database/sql/driver"
	"reflect"

	codecjson "github.com/go-sdk/core/codec/json"
	"github.com/go-sdk/core/errx"
	"github.com/spf13/cast"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// DeletedAt 使用 0 表示有效记录，使用 Unix 毫秒时间戳表示已删除记录。
type DeletedAt int64

var _ Field = (*DeletedAt)(nil)

//goland:noinspection GoMixedReceiverTypes
func (x *DeletedAt) Scan(value any) error {
	if value == nil {
		*x = 0
		return nil
	}
	source := reflect.ValueOf(value)
	switch source.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if source.Uint() > ^uint64(0)>>1 {
			return errx.New("scan DeletedAt: value overflows int64")
		}
	case reflect.Float32, reflect.Float64, reflect.Bool:
		return errx.Newf("scan DeletedAt from %T: unsupported type", value)
	}
	if bytes, ok := value.([]byte); ok {
		value = string(bytes)
	}
	number, err := cast.ToInt64E(value)
	if err != nil {
		return errx.Wrapf(err, "scan DeletedAt from %T", value)
	}
	if number < 0 {
		return errx.New("scan DeletedAt: negative timestamp")
	}
	*x = DeletedAt(number)
	return nil
}

func (x DeletedAt) Value() (driver.Value, error) {
	if x < 0 {
		return nil, errx.New("value DeletedAt: negative timestamp")
	}
	return int64(x), nil
}

func (x DeletedAt) MarshalJSON() ([]byte, error) { return codecjson.Marshal[[]byte](int64(x)) }

//goland:noinspection GoMixedReceiverTypes
func (x *DeletedAt) UnmarshalJSON(bytes []byte) error {
	if string(bytes) == "null" {
		*x = 0
		return nil
	}
	var value int64
	if err := codecjson.Unmarshal(bytes, &value); err != nil {
		return err
	}
	if value < 0 {
		return errx.New("unmarshal DeletedAt: negative timestamp")
	}
	*x = DeletedAt(value)
	return nil
}

func (DeletedAt) GormDataType() string { return "deleted_at" }

func (DeletedAt) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Name() {
	case "mysql", "postgres":
		return "BIGINT"
	case "sqlite":
		return "INTEGER"
	default:
		return ""
	}
}

func (DeletedAt) QueryClauses(field *schema.Field) []clause.Interface {
	return []clause.Interface{softDeleteQueryClause{Field: field}}
}

func (DeletedAt) UpdateClauses(field *schema.Field) []clause.Interface {
	return []clause.Interface{softDeleteUpdateClause{Field: field}}
}

func (DeletedAt) DeleteClauses(field *schema.Field) []clause.Interface {
	return []clause.Interface{softDeleteDeleteClause{Field: field}}
}

type softDeleteQueryClause struct{ Field *schema.Field }

func (softDeleteQueryClause) Name() string               { return "" }
func (softDeleteQueryClause) Build(clause.Builder)       {}
func (softDeleteQueryClause) MergeClause(*clause.Clause) {}

func (softDeleteQueryClauseValue softDeleteQueryClause) ModifyStatement(statement *gorm.Statement) {
	if _, ok := statement.Clauses["soft_delete_enabled"]; ok || statement.Unscoped {
		return
	}
	if whereClause, ok := statement.Clauses["WHERE"]; ok {
		if where, ok := whereClause.Expression.(clause.Where); ok {
			for _, expression := range where.Exprs {
				if orConditions, ok := expression.(clause.OrConditions); ok && len(orConditions.Exprs) == 1 {
					where.Exprs = []clause.Expression{clause.And(where.Exprs...)}
					whereClause.Expression = where
					statement.Clauses["WHERE"] = whereClause
					break
				}
			}
		}
	}
	statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Eq{
		Column: clause.Column{Table: clause.CurrentTable, Name: softDeleteQueryClauseValue.Field.DBName},
		Value:  DeletedAt(0),
	}}})
	statement.Clauses["soft_delete_enabled"] = clause.Clause{}
}

type softDeleteUpdateClause struct{ Field *schema.Field }

func (softDeleteUpdateClause) Name() string               { return "" }
func (softDeleteUpdateClause) Build(clause.Builder)       {}
func (softDeleteUpdateClause) MergeClause(*clause.Clause) {}

func (value softDeleteUpdateClause) ModifyStatement(statement *gorm.Statement) {
	if statement.SQL.Len() == 0 && !statement.Unscoped {
		softDeleteQueryClause(value).ModifyStatement(statement)
	}
}

type softDeleteDeleteClause struct{ Field *schema.Field }

func (softDeleteDeleteClause) Name() string               { return "" }
func (softDeleteDeleteClause) Build(clause.Builder)       {}
func (softDeleteDeleteClause) MergeClause(*clause.Clause) {}

func (value softDeleteDeleteClause) ModifyStatement(statement *gorm.Statement) {
	if statement.SQL.Len() != 0 || statement.Unscoped {
		return
	}
	deletedAt := DeletedAt(statement.DB.NowFunc().UnixMilli())
	statement.AddClause(clause.Set{{Column: clause.Column{Name: value.Field.DBName}, Value: deletedAt}})
	statement.SetColumn(value.Field.DBName, deletedAt, true)
	if statement.Schema != nil {
		_, queryValues := schema.GetIdentityFieldValuesMap(statement.Context, statement.ReflectValue, statement.Schema.PrimaryFields)
		column, values := schema.ToQueryValues(statement.Table, statement.Schema.PrimaryFieldDBNames, queryValues)
		if len(values) > 0 {
			statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.IN{Column: column, Values: values}}})
		}
		if statement.ReflectValue.CanAddr() && statement.Dest != statement.Model && statement.Model != nil {
			_, queryValues = schema.GetIdentityFieldValuesMap(statement.Context, reflect.ValueOf(statement.Model), statement.Schema.PrimaryFields)
			column, values = schema.ToQueryValues(statement.Table, statement.Schema.PrimaryFieldDBNames, queryValues)
			if len(values) > 0 {
				statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.IN{Column: column, Values: values}}})
			}
		}
	}
	softDeleteQueryClause(value).ModifyStatement(statement)
	statement.AddClauseIfNotExists(clause.Update{})
	statement.Build(statement.DB.Callback().Update().Clauses...)
}
