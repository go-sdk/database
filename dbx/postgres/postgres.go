// Package postgres 按需注册 PostgreSQL GORM 驱动。
package postgres

import (
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/go-sdk/database/dbx"
)

func init() {
	dbx.Register("postgres", dbx.Driver{
		Dialector: func(dsn string) (gorm.Dialector, error) { return postgres.Open(dsn), nil },
		Pool: dbx.PoolConfig{
			MaxIdleConns:    10,
			MaxOpenConns:    100,
			ConnMaxLifetime: 30 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
	})
}
