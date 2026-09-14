// Package mysql 按需注册 MySQL GORM 驱动。
package mysql

import (
	"time"

	sqlmysql "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/go-sdk/database/dbx"
)

func init() {
	dbx.Register("mysql", dbx.Driver{
		Dialector: dialector,
		Pool: dbx.PoolConfig{
			MaxIdleConns:    10,
			MaxOpenConns:    100,
			ConnMaxLifetime: 3 * time.Minute,
			ConnMaxIdleTime: time.Minute,
		},
	})
}

func dialector(dsn string) (gorm.Dialector, error) {
	config, err := sqlmysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	// Metadata 包含 time.Time 字段，统一要求驱动直接解析时间值。
	config.ParseTime = true
	return mysql.New(mysql.Config{DSNConfig: config}), nil
}
