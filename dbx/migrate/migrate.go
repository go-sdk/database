// Package migrate 提供带跨副本锁的有序 GORM 数据库迁移。
package migrate

import (
	"context"
	"regexp"
	"slices"
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/go-sdk/core/errx"
	"github.com/go-sdk/core/logx"
	"gorm.io/gorm"
)

const (
	TableName              = "_migrations"
	MinimumMySQLVersion    = "5.7"
	MinimumMariaDBVersion  = "10.5"
	MinimumPostgresVersion = "14"
	MinimumSQLiteVersion   = "3.35"
)

var migrationIDPattern = regexp.MustCompile(`^[0-9]{8}_[0-9]{6}_[0-9]{2}_[a-z0-9]+(?:_[a-z0-9]+)*$`)

var (
	ErrDatabaseRequired    = errx.New("migrate: database is required")
	ErrDatabaseNotSelected = errx.New("migrate: database is not selected")
	ErrInvalidMigrationID  = errx.New("migrate: invalid migration ID")
	ErrDuplicatedID        = errx.New("migrate: duplicated migration ID")
	ErrUpRequired          = errx.New("migrate: migration Up is required")
	ErrUnknownMigration    = errx.New("migrate: database contains an unknown migration")
	ErrUnsupportedDriver   = errx.New("migrate: unsupported database driver")
	ErrInvalidLockTimeout  = errx.New("migrate: lock timeout must not be negative")
	ErrLockTimeout         = errx.New("migrate: lock timeout")
	ErrLockFailed          = errx.New("migrate: lock operation failed")
)

// Migration 是一个可向前执行并可选回滚的数据库版本。
type Migration struct {
	ID   string
	Up   func(*gorm.DB) error
	Down func(*gorm.DB) error
}

// Migrations 是按 ID 排序后执行的迁移集合。
type Migrations []*Migration

type config struct {
	lockTimeout               time.Duration
	useTransaction            bool
	validateUnknownMigrations bool
}

// Option 调整迁移和锁行为。
type Option func(*config)

// WithLockTimeout 设置等待迁移锁的最长时间。
func WithLockTimeout(timeout time.Duration) Option {
	return func(c *config) { c.lockTimeout = timeout }
}

// WithTransaction 配置 gormigrate 是否为迁移批次开启事务。
// SQLite 始终由 BEGIN IMMEDIATE 包裹，以同时提供锁和事务边界。
func WithTransaction(enabled bool) Option {
	return func(c *config) { c.useTransaction = enabled }
}

// WithValidateUnknownMigrations 配置是否拒绝数据库中未在当前代码声明的迁移。
func WithValidateUnknownMigrations(enabled bool) Option {
	return func(c *config) { c.validateUnknownMigrations = enabled }
}

// Migrator 管理一组固定且已排序的迁移。
type Migrator struct {
	db         *gorm.DB
	migrations Migrations
	config     config
}

// New 校验并复制迁移列表，调用方后续修改原切片不会影响执行顺序。
func New(db *gorm.DB, migrations Migrations, options ...Option) (*Migrator, error) {
	if db == nil {
		return nil, ErrDatabaseRequired
	}
	cfg := config{lockTimeout: 30 * time.Second, validateUnknownMigrations: true}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	if cfg.lockTimeout < 0 {
		return nil, ErrInvalidLockTimeout
	}
	items := slices.Clone(migrations)
	seen := make(map[string]struct{}, len(items))
	for index, migration := range items {
		if migration == nil {
			return nil, errx.Wrapf(ErrInvalidMigrationID, "migration %d", index)
		}
		if !migrationIDPattern.MatchString(migration.ID) {
			return nil, errx.Wrapf(ErrInvalidMigrationID, "migration %q", migration.ID)
		}
		if _, err := time.Parse("20060102_150405", migration.ID[:15]); err != nil {
			return nil, errx.Wrapf(ErrInvalidMigrationID, "migration %q", migration.ID)
		}
		if migration.Up == nil {
			return nil, errx.Wrapf(ErrUpRequired, "migration %q", migration.ID)
		}
		if _, ok := seen[migration.ID]; ok {
			return nil, errx.Wrapf(ErrDuplicatedID, "migration %q", migration.ID)
		}
		seen[migration.ID] = struct{}{}
	}
	slices.SortFunc(items, func(left, right *Migration) int { return cmpString(left.ID, right.ID) })
	return &Migrator{db: db, migrations: items, config: cfg}, nil
}

// Up 按 ID 顺序执行全部尚未应用的迁移。
func (m *Migrator) Up(ctx context.Context) error {
	return m.execute(ctx, "up", func(db *gorm.DB, stats *executionStats) error {
		if len(m.migrations) == 0 {
			return nil
		}
		return m.gormigrate(db, stats).Migrate()
	})
}

// Down 回滚最后一个已应用迁移。未定义 Down 时仅移除该迁移记录。
func (m *Migrator) Down(ctx context.Context) error {
	return m.execute(ctx, "down", func(db *gorm.DB, stats *executionStats) error {
		count, err := m.appliedCount(db)
		if err != nil || count == 0 {
			return err
		}
		return m.gormigrate(db, stats).RollbackLast()
	})
}

// Reset 按逆序回滚全部已应用迁移。未定义 Down 的迁移直接跳过回滚逻辑并移除记录。
func (m *Migrator) Reset(ctx context.Context) error {
	return m.execute(ctx, "reset", func(db *gorm.DB, stats *executionStats) error {
		count, err := m.appliedCount(db)
		if err != nil {
			return err
		}
		migration := m.gormigrate(db, stats)
		for range count {
			if err := migration.RollbackLast(); err != nil {
				return err
			}
		}
		return nil
	})
}

func (m *Migrator) gormigrate(db *gorm.DB, stats *executionStats) *gormigrate.Gormigrate {
	options := &gormigrate.Options{
		TableName:                 TableName,
		IDColumnName:              "id",
		IDColumnSize:              255,
		UseTransaction:            m.config.useTransaction && db.Name() != "sqlite",
		ValidateUnknownMigrations: m.config.validateUnknownMigrations,
	}
	migrations := make([]*gormigrate.Migration, 0, len(m.migrations))
	for _, item := range m.migrations {
		down := item.Down
		if down == nil {
			down = func(*gorm.DB) error { return nil }
		}
		migrations = append(migrations, &gormigrate.Migration{
			ID:       item.ID,
			Migrate:  stats.wrap(item.ID, "up", item.Up),
			Rollback: stats.wrap(item.ID, "down", down),
		})
	}
	return gormigrate.New(db, options, migrations)
}

func (m *Migrator) execute(ctx context.Context, operation string, run func(*gorm.DB, *executionStats) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	stats := &executionStats{ctx: ctx, operation: operation, startedAt: time.Now(), from: "none", to: "none"}
	err := withLock(ctx, m.db, m.config.lockTimeout, func(db *gorm.DB) error {
		version, err := currentVersion(db)
		if err != nil {
			return err
		}
		stats.from, stats.to = version, version
		logx.Ctx(ctx).Info().Str("operation", operation).Str("from", version).Msg("database migration started")
		runErr := run(db, stats)
		if runErr != nil && db.Name() == "sqlite" {
			return runErr
		}
		version, versionErr := currentVersion(db)
		if versionErr == nil {
			stats.to = version
		}
		return combineErrors(runErr, versionErr)
	})
	stats.logSummary(err)
	return err
}

type executionStats struct {
	ctx       context.Context
	operation string
	startedAt time.Time
	executed  int
	from      string
	to        string
}

func (s *executionStats) wrap(id, direction string, run func(*gorm.DB) error) func(*gorm.DB) error {
	return func(db *gorm.DB) error {
		startedAt := time.Now()
		s.executed++
		logx.Ctx(s.ctx).Info().Str("operation", s.operation).Str("direction", direction).
			Str("migration_id", id).Msg("database migration step started")
		err := run(db)
		if err != nil {
			logx.Ctx(s.ctx).Error().Err(err).Str("operation", s.operation).Str("direction", direction).
				Str("migration_id", id).Dur("elapsed", time.Since(startedAt).Truncate(time.Millisecond)).Msg("database migration step failed")
			return err
		}
		logx.Ctx(s.ctx).Info().Str("operation", s.operation).Str("direction", direction).
			Str("migration_id", id).Dur("elapsed", time.Since(startedAt).Truncate(time.Millisecond)).Msg("database migration step succeeded")
		return nil
	}
}

func (s *executionStats) logSummary(err error) {
	event := logx.Ctx(s.ctx).Info()
	status := "succeeded"
	if err != nil {
		event = logx.Ctx(s.ctx).Error().Err(err)
		status = "failed"
	}
	event.Str("operation", s.operation).Str("status", status).Int("executed", s.executed).
		Str("from", s.from).Str("to", s.to).Dur("elapsed", time.Since(s.startedAt).Truncate(time.Millisecond)).Msg("database migration summary")
}

func currentVersion(db *gorm.DB) (string, error) {
	exists, err := migrationTableExists(db)
	if err != nil {
		return "none", err
	}
	if !exists {
		return "none", nil
	}
	var applied []string
	if err := db.Table(TableName).Pluck("id", &applied).Error; err != nil {
		return "none", err
	}
	if len(applied) == 0 {
		return "none", nil
	}
	slices.Sort(applied)
	return applied[len(applied)-1], nil
}

func (m *Migrator) appliedCount(db *gorm.DB) (int, error) {
	exists, err := migrationTableExists(db)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	var applied []string
	if err := db.Table(TableName).Pluck("id", &applied).Error; err != nil {
		return 0, err
	}
	known := make(map[string]struct{}, len(m.migrations))
	for _, migration := range m.migrations {
		known[migration.ID] = struct{}{}
	}
	count := 0
	for _, id := range applied {
		if _, ok := known[id]; !ok {
			if m.config.validateUnknownMigrations {
				return 0, errx.Wrapf(ErrUnknownMigration, "migration %q", id)
			}
			continue
		}
		count++
	}
	return count, nil
}

// migrationTableExists 使用可返回错误的表枚举，避免把元数据查询失败误判为迁移表不存在。
func migrationTableExists(db *gorm.DB) (bool, error) {
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return false, err
	}
	return slices.Contains(tables, TableName), nil
}

func cmpString(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
