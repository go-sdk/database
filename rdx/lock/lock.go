// Package lock 提供基于 Redis 的持有者令牌租约锁。
package lock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/go-sdk/core/errx"
	"github.com/redis/go-redis/v9"

	"github.com/go-sdk/database/rdx"
)

const tokenBytes = 32

var (
	// ErrClientRequired 表示没有提供 Redis 客户端。
	ErrClientRequired = errx.New("redis lock client must not be nil")
	// ErrNamespaceRequired 表示锁命名空间为空。
	ErrNamespaceRequired = errx.New("redis lock namespace must not be empty")
	// ErrKeyRequired 表示锁业务键为空。
	ErrKeyRequired = errx.New("redis lock key must not be empty")
	// ErrInvalidTTL 表示租约时长小于 Redis 支持的毫秒精度。
	ErrInvalidTTL = errx.New("redis lock ttl must be at least one millisecond")
	// ErrNotAcquired 表示锁已被其他持有者占用。
	ErrNotAcquired = errx.New("redis lock is already held")
	// ErrLeaseLost 表示当前持有者已失去租约。
	ErrLeaseLost = errx.New("redis lock lease is no longer owned")
)

var (
	renewScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)
	unlockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)
)

// Manager 使用统一命名空间创建 Redis 租约锁。
type Manager struct {
	client    rdx.Client
	namespace string
}

// Lease 表示一次成功获取的 Redis 租约。
type Lease struct {
	client rdx.Client
	key    string
	token  string
}

// New 创建 Redis 租约锁管理器。
func New(client rdx.Client, namespace string) (*Manager, error) {
	if client == nil {
		return nil, ErrClientRequired
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil, ErrNamespaceRequired
	}
	return &Manager{client: client, namespace: namespace}, nil
}

// Acquire 尝试获取指定业务键的租约；锁被占用时返回 ErrNotAcquired。
func (m *Manager) Acquire(ctx context.Context, key string, ttl time.Duration) (*Lease, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrKeyRequired
	}
	if ttl < time.Millisecond {
		return nil, ErrInvalidTTL
	}
	token, err := newToken()
	if err != nil {
		return nil, errx.Wrap(err, "generate redis lock owner token")
	}
	lockKey := m.namespace + ":" + key
	acquired, err := m.client.SetNX(ctx, lockKey, token, ttl).Result()
	if err != nil {
		return nil, errx.Wrap(err, "acquire redis lock")
	}
	if !acquired {
		return nil, ErrNotAcquired
	}
	return &Lease{client: m.client, key: lockKey, token: token}, nil
}

// Renew 仅在当前持有者仍拥有锁时原子延长租约。
func (l *Lease) Renew(ctx context.Context, ttl time.Duration) error {
	if ttl < time.Millisecond {
		return ErrInvalidTTL
	}
	updated, err := renewScript.Run(ctx, l.client, []string{l.key}, l.token, ttl.Milliseconds()).Int64()
	if err != nil {
		return errx.Wrap(err, "renew redis lock")
	}
	if updated == 0 {
		return ErrLeaseLost
	}
	return nil
}

// Unlock 仅在当前持有者仍拥有锁时原子释放租约。
func (l *Lease) Unlock(ctx context.Context) error {
	deleted, err := unlockScript.Run(ctx, l.client, []string{l.key}, l.token).Int64()
	if err != nil {
		return errx.Wrap(err, "release redis lock")
	}
	if deleted == 0 {
		return ErrLeaseLost
	}
	return nil
}

func newToken() (string, error) {
	value := make([]byte, tokenBytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
