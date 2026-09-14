package dbx

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DB 是 GORM 数据库会话的类型别名，保留全部原生方法和类型兼容性。
type DB = gorm.DB

// OnConflict 是 GORM 写入冲突策略的类型别名。
type OnConflict = clause.OnConflict
