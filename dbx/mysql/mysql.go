// Package mysql 按需注册 MySQL GORM 驱动。
package mysql

import (
	"time"

	"github.com/go-sdk/core/osx"
	sqlmysql "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/go-sdk/database/dbx"
)

func init() {
	dbx.Register("mysql", dbx.Driver{
		Dialector: dialector,
		Pool: dbx.PoolConfig{
			MaxIdleConns:    osx.GetEnv[int](10, "DBX_MYSQL_MAX_IDLE_CONNS", "DBX_MAX_IDLE_CONNS"),
			MaxOpenConns:    osx.GetEnv[int](100, "DBX_MYSQL_MAX_OPEN_CONNS", "DBX_MAX_OPEN_CONNS"),
			ConnMaxLifetime: osx.GetEnv[time.Duration](3*time.Minute, "DBX_MYSQL_CONN_MAX_LIFETIME", "DBX_CONN_MAX_LIFETIME"),
			ConnMaxIdleTime: osx.GetEnv[time.Duration](time.Minute, "DBX_MYSQL_CONN_MAX_IDLE_TIME", "DBX_CONN_MAX_IDLE_TIME"),
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
