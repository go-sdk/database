// Package database 是个人使用的 Go 数据库基础类库，基于 GORM 统一 MySQL、
// PostgreSQL 和 SQLite 的初始化、日志、版本迁移、跨副本迁移锁、Redis
// 客户端与租约锁、JSON 字段和毫秒时间戳软删除行为。
//
// 根包本身不提供 API，主要能力位于以下子包：
//
//	dbx           打开数据库、连接池配置、GORM 日志、错误翻译和软删除元数据
//	dbx/migrate   数据库版本迁移、迁移记录表和多副本迁移锁
//	dbx/mysql     MySQL 与 MariaDB 驱动
//	dbx/postgres  PostgreSQL 驱动
//	dbx/sqlite    SQLite 驱动
//	dbx/datatype  JSON 字段等扩展数据类型
//	rdx           Redis 单机与 Sentinel 客户端
//	rdx/lock      Redis 持有者令牌租约锁
//
// 安装后按需空白导入驱动，并使用 dbx.Open 打开数据库：
//
//	import (
//		"github.com/go-sdk/database/dbx"
//		_ "github.com/go-sdk/database/dbx/mysql"
//	)
//
//	db, err := dbx.Open("mysql", "user:password@tcp(localhost:3306)/app")
//
// 安装：
//
//	go get github.com/go-sdk/database
//
// 迁移、连接池配置、软删除和真实数据库测试等详细说明见项目 README。
package database
