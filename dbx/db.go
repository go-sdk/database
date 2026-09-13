package dbx

import (
	"database/sql"
	"strings"
	"sync"
	"time"

	"github.com/go-sdk/core/errx"
	"github.com/go-sdk/core/lifex"
	"gorm.io/gorm"
)

// DialectorFunc 根据原生 DSN 创建 GORM Dialector。
type DialectorFunc func(dsn string) (gorm.Dialector, error)

// PoolConfig 定义 database/sql 连接池参数。
type PoolConfig struct {
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// Driver 描述按需注册的数据库驱动及其默认连接池参数。
type Driver struct {
	Dialector DialectorFunc
	Pool      PoolConfig
}

type config struct {
	gormConfig  *gorm.Config
	gormOptions []gorm.Option
	pool        *PoolConfig
}

// Option 调整 Open 的公共配置。
type Option func(*config)

var drivers = struct {
	sync.RWMutex
	items map[string]Driver
}{items: map[string]Driver{}}

// Register 注册数据库驱动。重复注册属于程序配置错误，会直接 panic。
func Register(name string, driver Driver) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" || driver.Dialector == nil {
		panic("dbx: invalid driver registration")
	}
	drivers.Lock()
	defer drivers.Unlock()
	if _, ok := drivers.items[name]; ok {
		panic("dbx: driver already registered: " + name)
	}
	drivers.items[name] = driver
}

// WithGORMConfig 使用指定的 GORM 配置覆盖默认配置；TranslateError 始终保持开启。
func WithGORMConfig(value *gorm.Config) Option {
	return func(c *config) {
		if value != nil {
			copyValue := *value
			c.gormConfig = &copyValue
		}
	}
}

// WithGORMOptions 追加传递给 gorm.Open 的选项。
func WithGORMOptions(options ...gorm.Option) Option {
	return func(c *config) { c.gormOptions = append(c.gormOptions, options...) }
}

// WithPoolConfig 覆盖驱动提供的默认连接池配置。
func WithPoolConfig(value PoolConfig) Option {
	return func(c *config) { c.pool = &value }
}

// Open 使用已注册驱动和原生 DSN 打开数据库，不执行数据库迁移。
func Open(driverName, dsn string, options ...Option) (*gorm.DB, error) {
	driverName = strings.TrimSpace(strings.ToLower(driverName))
	if driverName == "" {
		return nil, ErrDriverRequired
	}
	if strings.TrimSpace(dsn) == "" {
		return nil, ErrDSNRequired
	}

	drivers.RLock()
	driver, ok := drivers.items[driverName]
	drivers.RUnlock()
	if !ok {
		return nil, errx.Wrapf(ErrDriverNotRegistered, "driver %q", driverName)
	}

	cfg := &config{gormConfig: defaultGORMConfig()}
	for _, option := range options {
		if option != nil {
			option(cfg)
		}
	}
	if cfg.gormConfig.Logger == nil {
		cfg.gormConfig.Logger = NewLogger(DefaultLoggerConfig())
	}
	if !cfg.gormConfig.TranslateError {
		cfg.gormConfig.TranslateError = true
	}

	dialector, err := driver.Dialector(dsn)
	if err != nil {
		return nil, errx.Wrap(err, "create database dialector")
	}
	gormOptions := make([]gorm.Option, 0, len(cfg.gormOptions)+1)
	gormOptions = append(gormOptions, cfg.gormConfig)
	gormOptions = append(gormOptions, cfg.gormOptions...)
	db, err := gorm.Open(dialector, gormOptions...)
	if err != nil {
		return nil, errx.Wrap(err, "open database")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, errx.Wrap(err, "get sql database")
	}
	pool := driver.Pool
	if cfg.pool != nil {
		pool = *cfg.pool
	}
	applyPoolConfig(sqlDB, pool)
	lifex.OnDeinit(func() error { return sqlDB.Close() })
	return db, nil
}

func defaultGORMConfig() *gorm.Config {
	return &gorm.Config{
		Logger:         NewLogger(DefaultLoggerConfig()),
		TranslateError: true,
	}
}

func applyPoolConfig(db *sql.DB, config PoolConfig) {
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)
}
