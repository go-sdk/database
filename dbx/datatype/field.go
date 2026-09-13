package datatype

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
)

type Field interface {
	sql.Scanner
	driver.Valuer

	json.Marshaler
	json.Unmarshaler
}
