// Package sqlite 按需注册无 CGO 的 SQLite GORM 驱动。
package sqlite

import (
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-sdk/core/osx"
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
			MaxIdleConns:    osx.GetEnv[int](1, "DBX_SQLITE_MAX_IDLE_CONNS", "DBX_MAX_IDLE_CONNS"),
			MaxOpenConns:    osx.GetEnv[int](1, "DBX_SQLITE_MAX_OPEN_CONNS", "DBX_MAX_OPEN_CONNS"),
			ConnMaxLifetime: osx.GetEnv[time.Duration](0, "DBX_SQLITE_CONN_MAX_LIFETIME", "DBX_CONN_MAX_LIFETIME"),
			ConnMaxIdleTime: osx.GetEnv[time.Duration](0, "DBX_SQLITE_CONN_MAX_IDLE_TIME", "DBX_CONN_MAX_IDLE_TIME"),
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
