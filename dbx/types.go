package dbx

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DB 是 GORM 数据库会话的类型别名，保留全部原生方法和类型兼容性。
type DB = gorm.DB

// Expr 是 GORM 原生 SQL 表达式的类型别名，用于在查询和更新中嵌入 SQL 片段。
type Expr = clause.Expr

// OnConflict 是 GORM 写入冲突策略的类型别名。
type OnConflict = clause.OnConflict

// Locking 是 GORM 行级锁子句的类型别名，用于生成 FOR UPDATE、FOR SHARE 等锁定语句。
type Locking = clause.Locking
