// Package rdx 提供 Redis 单机和 Sentinel 客户端初始化，并统一接入应用生命周期。
package rdx

import (
	"context"
	"crypto/tls"
	"strings"
	"time"

	"github.com/go-sdk/core/errx"
	"github.com/go-sdk/core/lifex"
	"github.com/go-sdk/core/logx"
	"github.com/redis/go-redis/v9"
)

// Mode 表示 Redis 的连接模式。
type Mode string

const (
	// ModeStandalone 连接一个 Redis 单机节点。
	ModeStandalone Mode = "standalone"
	// ModeSentinel 通过 Sentinel 发现并连接主节点。
	ModeSentinel Mode = "sentinel"
)

var (
	// ErrAddressRequired 表示没有配置 Redis 地址。
	ErrAddressRequired = errx.New("redis address must not be empty")
	// ErrInvalidMode 表示 Redis 连接模式不受支持。
	ErrInvalidMode = errx.New("redis mode must be standalone or sentinel")
	// ErrStandaloneAddressCount 表示单机模式配置了不止一个地址。
	ErrStandaloneAddressCount = errx.New("redis standalone mode requires exactly one address")
	// ErrSentinelMasterRequired 表示 Sentinel 模式没有配置主节点名称。
	ErrSentinelMasterRequired = errx.New("redis sentinel master name must not be empty")
	// ErrInvalidDatabase 表示 Redis 数据库编号无效。
	ErrInvalidDatabase = errx.New("redis database must not be negative")
	// ErrInvalidPoolConfig 表示 Redis 连接池参数无效。
	ErrInvalidPoolConfig = errx.New("redis pool values must not be negative")
)

// TLSConfig 定义 Redis TLS 连接参数。
type TLSConfig struct {
	Enabled    bool   `json:"enabled"`
	ServerName string `json:"server_name"`
}

// Config 定义 Redis 单机或 Sentinel 客户端参数。
type Config struct {
	Mode             Mode          `json:"mode"`
	Addresses        []string      `json:"addresses"`
	MasterName       string        `json:"master_name"`
	Username         string        `json:"username"`
	Password         string        `json:"password"`
	SentinelUsername string        `json:"sentinel_username"`
	SentinelPassword string        `json:"sentinel_password"`
	Database         int           `json:"database"`
	ClientName       string        `json:"client_name"`
	DialTimeout      time.Duration `json:"dial_timeout"`
	ReadTimeout      time.Duration `json:"read_timeout"`
	WriteTimeout     time.Duration `json:"write_timeout"`
	PoolTimeout      time.Duration `json:"pool_timeout"`
	PoolSize         int           `json:"pool_size"`
	MinIdleConns     int           `json:"min_idle_conns"`
	MaxIdleConns     int           `json:"max_idle_conns"`
	MaxActiveConns   int           `json:"max_active_conns"`
	ConnMaxIdleTime  time.Duration `json:"conn_max_idle_time"`
	ConnMaxLifetime  time.Duration `json:"conn_max_lifetime"`
	TLS              TLSConfig     `json:"tls"`
}

// Client 是单机和 Sentinel Redis 客户端共同实现的接口。
type Client = redis.UniversalClient

// Open 校验配置、建立 Redis 连接、执行 Ping，并登记生命周期关闭函数。
func Open(ctx context.Context, config Config) (Client, error) {
	config.Mode = Mode(strings.ToLower(strings.TrimSpace(string(config.Mode))))
	if config.Mode == "" {
		config.Mode = ModeStandalone
	}
	config.Addresses = normalizeAddresses(config.Addresses)
	config.MasterName = strings.TrimSpace(config.MasterName)
	config.ClientName = strings.TrimSpace(config.ClientName)
	config.TLS.ServerName = strings.TrimSpace(config.TLS.ServerName)
	if err := validate(config); err != nil {
		return nil, err
	}

	var client Client
	switch config.Mode {
	case ModeStandalone:
		client = redis.NewClient(&redis.Options{
			Addr:                  config.Addresses[0],
			ClientName:            config.ClientName,
			Username:              config.Username,
			Password:              config.Password,
			DB:                    config.Database,
			DialTimeout:           config.DialTimeout,
			ReadTimeout:           config.ReadTimeout,
			WriteTimeout:          config.WriteTimeout,
			ContextTimeoutEnabled: true,
			PoolSize:              config.PoolSize,
			MinIdleConns:          config.MinIdleConns,
			MaxIdleConns:          config.MaxIdleConns,
			MaxActiveConns:        config.MaxActiveConns,
			PoolTimeout:           config.PoolTimeout,
			ConnMaxIdleTime:       config.ConnMaxIdleTime,
			ConnMaxLifetime:       config.ConnMaxLifetime,
			TLSConfig:             tlsConfig(config.TLS),
		})
	case ModeSentinel:
		client = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:            config.MasterName,
			SentinelAddrs:         config.Addresses,
			ClientName:            config.ClientName,
			SentinelUsername:      config.SentinelUsername,
			SentinelPassword:      config.SentinelPassword,
			Username:              config.Username,
			Password:              config.Password,
			DB:                    config.Database,
			DialTimeout:           config.DialTimeout,
			ReadTimeout:           config.ReadTimeout,
			WriteTimeout:          config.WriteTimeout,
			ContextTimeoutEnabled: true,
			PoolSize:              config.PoolSize,
			MinIdleConns:          config.MinIdleConns,
			MaxIdleConns:          config.MaxIdleConns,
			MaxActiveConns:        config.MaxActiveConns,
			PoolTimeout:           config.PoolTimeout,
			ConnMaxIdleTime:       config.ConnMaxIdleTime,
			ConnMaxLifetime:       config.ConnMaxLifetime,
			TLSConfig:             tlsConfig(config.TLS),
		})
	}
	if err := client.Ping(ctx).Err(); err != nil {
		pingErr := errx.Wrap(err, "ping redis")
		if closeErr := client.Close(); closeErr != nil {
			return nil, errx.Join(pingErr, errx.Wrap(closeErr, "close redis after failed ping"))
		}
		return nil, pingErr
	}
	lifex.OnDeinit(client.Close)
	logx.Ctx(ctx).Info().Str("mode", string(config.Mode)).Int("database", config.Database).Msg("redis initialized")
	return client, nil
}

func normalizeAddresses(addresses []string) []string {
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if value := strings.TrimSpace(address); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func validate(config Config) error {
	if len(config.Addresses) == 0 {
		return ErrAddressRequired
	}
	if config.Database < 0 {
		return ErrInvalidDatabase
	}
	if config.PoolSize < 0 || config.MinIdleConns < 0 || config.MaxIdleConns < 0 || config.MaxActiveConns < 0 {
		return ErrInvalidPoolConfig
	}
	switch config.Mode {
	case ModeStandalone:
		if len(config.Addresses) != 1 {
			return ErrStandaloneAddressCount
		}
	case ModeSentinel:
		if config.MasterName == "" {
			return ErrSentinelMasterRequired
		}
	default:
		return ErrInvalidMode
	}
	return nil
}

func tlsConfig(config TLSConfig) *tls.Config {
	if !config.Enabled {
		return nil
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: config.ServerName,
	}
}
