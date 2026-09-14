package dbx

import (
	"github.com/go-sdk/core/errx"
	"gorm.io/gorm"
)

var (
	ErrDriverRequired      = errx.New("database driver is required")
	ErrDSNRequired         = errx.New("database dsn is required")
	ErrDriverNotRegistered = errx.New("database driver is not registered")

	// ErrRecordNotFound 表示查询没有找到记录。
	ErrRecordNotFound = gorm.ErrRecordNotFound
	// ErrDuplicatedKey 表示唯一约束或主键冲突。
	ErrDuplicatedKey = gorm.ErrDuplicatedKey
	// ErrForeignKeyViolated 表示外键约束冲突。
	ErrForeignKeyViolated = gorm.ErrForeignKeyViolated
)

// IsRecordNotFound 判断错误链是否包含记录不存在错误。
func IsRecordNotFound(err error) bool {
	return errx.Is(err, ErrRecordNotFound)
}

// IsDuplicatedKey 判断错误链是否包含唯一键冲突错误。
func IsDuplicatedKey(err error) bool {
	return errx.Is(err, ErrDuplicatedKey)
}

// IsForeignKeyViolated 判断错误链是否包含外键约束错误。
func IsForeignKeyViolated(err error) bool {
	return errx.Is(err, ErrForeignKeyViolated)
}
