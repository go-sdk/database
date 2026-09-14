package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"hash/fnv"
	"math"
	"time"

	"github.com/go-sdk/core/errx"
	"gorm.io/gorm"
)

func withLock(ctx context.Context, db *gorm.DB, timeout time.Duration, run func(*gorm.DB) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return db.WithContext(ctx).Connection(func(connection *gorm.DB) error {
		// Connection 返回的会话 clone 为 0，后续链式调用会原地修改共享 Statement；
		// 锁操作的 Raw().Scan() 会把表名和模型解析结果写入 Statement，
		// 迁移函数内的 AutoMigrate 会因此把模型绑定到错误的表。
		// 统一换成全新 Statement 的会话，仅保留固定连接和 context。
		session := connection.Session(&gorm.Session{NewDB: true})
		switch connection.Name() {
		case "mysql":
			return withMySQLLock(ctx, session, timeout, run)
		case "postgres":
			return withPostgresLock(ctx, session, timeout, run)
		case "sqlite":
			return withSQLiteLock(ctx, session, timeout, run)
		default:
			return errx.Wrapf(ErrUnsupportedDriver, "driver %q", connection.Name())
		}
	})
}

func withMySQLLock(ctx context.Context, db *gorm.DB, timeout time.Duration, run func(*gorm.DB) error) (resultErr error) {
	var databaseName sql.NullString
	if err := db.Raw("SELECT DATABASE()").Scan(&databaseName).Error; err != nil {
		return wrapLockError(ErrLockFailed, err)
	}
	if !databaseName.Valid || databaseName.String == "" {
		return ErrDatabaseNotSelected
	}
	lockName := fmt.Sprintf("%x", sha256.Sum256([]byte("dbx:migrate:"+databaseName.String+":"+TableName)))
	var acquired sql.NullInt64
	lockCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		lockCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	if err := db.WithContext(lockCtx).Raw("SELECT GET_LOCK(?, ?)", lockName, max(int64(math.Ceil(timeout.Seconds())), 0)).Scan(&acquired).Error; err != nil {
		if errx.Is(lockCtx.Err(), context.DeadlineExceeded) {
			return wrapLockError(ErrLockTimeout, err)
		}
		return wrapLockError(ErrLockFailed, err)
	}
	if !acquired.Valid {
		return ErrLockFailed
	}
	if acquired.Int64 != 1 {
		return ErrLockTimeout
	}
	defer func() { resultErr = errx.Join(resultErr, releaseMySQLLock(db, lockName)) }()
	return run(db)
}

func releaseMySQLLock(db *gorm.DB, lockName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var released sql.NullInt64
	if err := db.WithContext(ctx).Raw("SELECT RELEASE_LOCK(?)", lockName).Scan(&released).Error; err != nil {
		return wrapLockError(ErrLockFailed, err)
	}
	if !released.Valid || released.Int64 != 1 {
		return ErrLockFailed
	}
	return nil
}

func withPostgresLock(ctx context.Context, db *gorm.DB, timeout time.Duration, run func(*gorm.DB) error) (resultErr error) {
	var databaseName string
	if err := db.Raw("SELECT current_database()").Scan(&databaseName).Error; err != nil {
		return wrapLockError(ErrLockFailed, err)
	}
	lockKey := advisoryLockKey(databaseName + ":" + TableName)
	if timeout == 0 {
		var acquired bool
		if err := db.Raw("SELECT pg_try_advisory_lock(?)", lockKey).Scan(&acquired).Error; err != nil {
			return wrapLockError(ErrLockFailed, err)
		}
		if !acquired {
			return ErrLockTimeout
		}
		defer func() { resultErr = errx.Join(resultErr, releasePostgresLock(db, lockKey)) }()
		return run(db.WithContext(ctx))
	}
	lockCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := db.WithContext(lockCtx).Exec("SELECT pg_advisory_lock(?)", lockKey).Error; err != nil {
		if errx.Is(lockCtx.Err(), context.DeadlineExceeded) {
			return wrapLockError(ErrLockTimeout, err)
		}
		return wrapLockError(ErrLockFailed, err)
	}
	defer func() { resultErr = errx.Join(resultErr, releasePostgresLock(db, lockKey)) }()
	return run(db.WithContext(ctx))
}

func releasePostgresLock(db *gorm.DB, lockKey int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var released bool
	if err := db.WithContext(ctx).Raw("SELECT pg_advisory_unlock(?)", lockKey).Scan(&released).Error; err != nil {
		return wrapLockError(ErrLockFailed, err)
	}
	if !released {
		return ErrLockFailed
	}
	return nil
}

func withSQLiteLock(ctx context.Context, db *gorm.DB, timeout time.Duration, run func(*gorm.DB) error) (resultErr error) {
	if err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", max(timeout.Milliseconds(), 0))).Error; err != nil {
		return wrapLockError(ErrLockFailed, err)
	}
	if err := db.Exec("BEGIN IMMEDIATE").Error; err != nil {
		if errx.Is(ctx.Err(), context.DeadlineExceeded) || isSQLiteLockContention(err) {
			return wrapLockError(ErrLockTimeout, err)
		}
		return wrapLockError(ErrLockFailed, err)
	}
	lockedDB := db.Session(&gorm.Session{})
	lockedDB.Statement.ConnPool = &sqliteImmediateConnPool{ConnPool: db.Statement.ConnPool}
	committed := false
	defer func() {
		if !committed {
			resultErr = errx.Join(resultErr, execSQLiteControl(db, "ROLLBACK"))
		}
	}()
	if err := run(lockedDB.WithContext(ctx)); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := db.Exec("COMMIT").Error; err != nil {
		return err
	}
	committed = true
	return nil
}

// sqliteImmediateConnPool 让 GORM 将外层 BEGIN IMMEDIATE 识别为事务，嵌套事务因此使用 SAVEPOINT。
type sqliteImmediateConnPool struct {
	gorm.ConnPool
}

func (*sqliteImmediateConnPool) Commit() error   { return gorm.ErrInvalidTransaction }
func (*sqliteImmediateConnPool) Rollback() error { return gorm.ErrInvalidTransaction }

type sqliteErrorCoder interface{ Code() int }

func isSQLiteLockContention(err error) bool {
	var sqliteErr sqliteErrorCoder
	if !errx.As(err, &sqliteErr) {
		return false
	}
	const (
		sqliteBusy   = 5
		sqliteLocked = 6
	)
	code := sqliteErr.Code() & 0xff
	return code == sqliteBusy || code == sqliteLocked
}

func execSQLiteControl(db *gorm.DB, statement string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return db.WithContext(ctx).Exec(statement).Error
}

func advisoryLockKey(value string) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(value))
	return int64(hash.Sum64())
}

func wrapLockError(marker, cause error) error {
	if cause == nil {
		return marker
	}
	return errx.Wrap(marker, cause.Error())
}
