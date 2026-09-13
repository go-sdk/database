package models

import (
	"time"

	"gorm.io/cli/gorm/field"
	"gorm.io/cli/gorm/genconfig"

	"github.com/go-sdk/database/dbx/datatype"
)

var _ = genconfig.Config{
	OutPath: "tests/modelg",
	FieldTypeMap: map[any]any{
		time.Time{}:           field.Time{},
		datatype.DeletedAt(0): field.Field[datatype.DeletedAt]{},
		datatype.JSON{}:       field.Field[datatype.JSON]{},
	},
}
