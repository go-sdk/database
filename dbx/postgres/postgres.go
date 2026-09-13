// Package postgres 按需注册 PostgreSQL GORM 驱动。
package postgres

import (
	"time"

	"github.com/go-sdk/core/osx"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/go-sdk/database/dbx"
)

func init() {
	dbx.Register("postgres", dbx.Driver{
		Dialector: func(dsn string) (gorm.Dialector, error) { return postgres.Open(dsn), nil },
		Pool: dbx.PoolConfig{
			MaxIdleConns:    osx.GetEnv[int](10, "DBX_POSTGRES_MAX_IDLE_CONNS", "DBX_MAX_IDLE_CONNS"),
			MaxOpenConns:    osx.GetEnv[int](100, "DBX_POSTGRES_MAX_OPEN_CONNS", "DBX_MAX_OPEN_CONNS"),
			ConnMaxLifetime: osx.GetEnv[time.Duration](30*time.Minute, "DBX_POSTGRES_CONN_MAX_LIFETIME", "DBX_CONN_MAX_LIFETIME"),
			ConnMaxIdleTime: osx.GetEnv[time.Duration](5*time.Minute, "DBX_POSTGRES_CONN_MAX_IDLE_TIME", "DBX_CONN_MAX_IDLE_TIME"),
		},
	})
}
