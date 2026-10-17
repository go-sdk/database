package dbx

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// DB 是 GORM 数据库会话的类型别名，保留全部原生方法和类型兼容性。
type DB = gorm.DB

// Config 是 GORM 打开连接时使用的配置类型别名，供 WithGORMConfig 使用。
type Config = gorm.Config

// Session 是 GORM 会话配置的类型别名，用于创建独立会话或调整查询行为。
type Session = gorm.Session

// GORMOption 是 GORM 连接选项的类型别名，与 dbx 自身的 Option 区分。
type GORMOption = gorm.Option

// Dialector 是 GORM 数据库方言接口的类型别名，供自定义驱动注册使用。
type Dialector = gorm.Dialector

// Logger 是 GORM 日志接口的类型别名，供注入自定义日志实现使用。
type Logger = logger.Interface

// LogLevel 是 GORM SQL 日志级别的类型别名。
type LogLevel = logger.LogLevel

// SQL 日志级别与 GORM 保持一致；LogSilent 禁用全部 SQL 日志。
const (
	LogSilent = logger.Silent
	LogError  = logger.Error
	LogWarn   = logger.Warn
	LogInfo   = logger.Info
)

// Expr 是 GORM 原生 SQL 表达式的类型别名，用于在查询和更新中嵌入 SQL 片段。
type Expr = clause.Expr

// Table 是 GORM 表名引用的类型别名，用于在 Joins 等子句中引用带别名的表。
type Table = clause.Table

// Column 是 GORM 列引用的类型别名，用于在 Order、Group 等子句中引用带表名前缀的列。
type Column = clause.Column

// OnConflict 是 GORM 写入冲突策略的类型别名。
type OnConflict = clause.OnConflict

// Returning 是 GORM RETURNING 子句的类型别名，用于返回写入操作影响的记录或指定列。
type Returning = clause.Returning

// Locking 是 GORM 行级锁子句的类型别名，用于生成 FOR UPDATE、FOR SHARE 等锁定语句。
type Locking = clause.Locking

// Assignments 根据指定的列和值构造 UPDATE 赋值子句。
var Assignments = clause.Assignments

// AssignmentColumns 根据模型当前字段值构造指定列的 UPDATE 赋值子句。
var AssignmentColumns = clause.AssignmentColumns
