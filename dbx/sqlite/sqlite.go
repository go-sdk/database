// Package sqlite 按需注册无 CGO 的 SQLite GORM 驱动。
package sqlite

import (
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/go-sdk/database/dbx"
)

const defaultPragmas = "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"

func init() {
	dbx.Register("sqlite", dbx.Driver{
		Dialector: func(dsn string) (gorm.Dialector, error) {
			return sqlite.Open(withDefaults(dsn)), nil
		},
		Pool: dbx.PoolConfig{
			MaxIdleConns:    1,
			MaxOpenConns:    1,
			ConnMaxLifetime: 0,
			ConnMaxIdleTime: 0,
		},
	})
}

func withDefaults(dsn string) string {
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + defaultPragmas
}
