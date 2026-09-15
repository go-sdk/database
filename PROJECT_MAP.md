# 项目地图

## 项目定位

`github.com/go-sdk/database` 是基于 GORM 的数据库基础类库，通过按需注册的驱动统一 MySQL、PostgreSQL 和 SQLite 初始化，提供 `core/logx` 日志适配、gormigrate 版本迁移、跨副本迁移锁以及 JSON 和毫秒时间戳软删除类型。

## 目录结构

```text
database/
├── .github/workflows/golang.yml     CI 静态检查、四数据库测试和发布
├── dbx/
│   ├── datatype/
│   │   ├── deleted_at.go            Unix 毫秒时间戳软删除类型及 GORM Clauses
│   │   ├── field.go                 Scanner、Valuer 和 JSON 接口约束
│   │   └── json.go                  跨数据库 JSON 类型及 gjson 路径读取
│   ├── migrate/
│   │   ├── lock.go                  MySQL、PostgreSQL 和 SQLite 迁移锁
│   │   └── migrate.go               迁移校验、排序及 Up、Down、Reset
│   ├── mysql/mysql.go               MySQL 驱动注册和连接池默认值
│   ├── postgres/postgres.go         PostgreSQL 驱动注册和连接池默认值
│   ├── sqlite/sqlite.go             SQLite 驱动注册、PRAGMA 和连接池默认值
│   ├── db.go                        驱动注册表、Open、Options 和连接生命周期
│   ├── error.go                     数据库公共错误和错误链判断
│   ├── logger.go                    core/logx GORM Logger
│   ├── metadata.go                  公共模型元数据
│   └── types.go                     面向应用的 GORM 类型别名
├── tests/
│   ├── db/main.go                   SQLite 示例程序
│   ├── db/main_test.go              MySQL、MariaDB、PostgreSQL、SQLite 增删改查集成测试
│   ├── modelg/                      GORM CLI 生成的类型安全字段
│   ├── models/                      示例模型和生成配置
│   └── database_test.go             SQLite 集成测试
├── docker-compose.yml               本地 MySQL、MariaDB、PostgreSQL 测试服务
├── AGENTS.md                        仓库协作与修改规范
├── PROJECT_MAP.md                   项目结构和关键调用链
├── README.md                        安装、公共 API 和行为说明
├── Makefile                         生成、检查、测试、本地 docker 数据库测试和示例构建命令
└── go.mod                           Go 模块和依赖定义
```

## 数据库打开链路

```text
应用空白导入 dbx/<driver>
    -> driver.init
    -> dbx.Register(name, Driver)

dbx.Open(name, dsn, options...)
    -> 查询驱动注册表
    -> 创建 GORM Dialector
    -> 注册 core/logx Logger 并始终启用错误翻译
    -> gorm.Open
    -> 应用驱动默认值、core/config 连接池配置或调用方覆盖值
    -> lifex.OnDeinit(sql.DB.Close)
    -> 返回 *dbx.DB
```

`dbx.Open` 不执行迁移。未导入对应驱动包时返回 `ErrDriverNotRegistered`，不会隐式引入其他数据库驱动。

## 日志链路

```text
server request context
    -> 附加带 trace-id、span-id、depth 的 zerolog Logger
    -> gorm.G[T](db).Find(ctx) 或 db.WithContext(ctx)
    -> GORM logger.Interface
    -> logx.Ctx(ctx)
    -> 结构化 SQL 日志
```

正常模式只记录错误和超过 200ms 的慢查询；debug 模式记录普通 SQL。日志包含展开参数后的 SQL、行数和耗时，因此调用方必须控制敏感字段的日志风险。SQL 日志的 `source` 字段跳过 GORM 生态（gorm.io、gormigrate、glebarez）和 dbx 内部帧，指向业务调用方。

## 迁移链路

```text
migrate.New(db, migrations)
    -> Migrations.Add 从直接调用方文件名生成迁移 ID
    -> 校验 YYYYMMDD_HHMMSS_NN_description
    -> 拒绝重复 ID 和空 Up
    -> 按 ID 升序排序

Up / Down / Reset
    -> 固定 database/sql.Conn
    -> 获取数据库对应的迁移锁
    -> 记录各迁移开始及成功或失败日志
    -> gormigrate 操作 _migrations
    -> 释放锁或提交 SQLite 事务
    -> 记录执行数量、起止版本和总耗时
```

- `Up` 执行所有未应用迁移。
- `Down` 回滚最后一个已应用迁移。
- `Reset` 按迁移 ID 逆序回滚所有已应用迁移。
- 一个迁移 Go 文件通过 `Migrations.Add` 注册一个迁移，文件 basename 即迁移 ID；已发布文件不得重命名。
- `Down` 函数可以为空；为空时不执行 schema 回滚，仅删除对应 `_migrations` 记录。
- 未知迁移默认被拒绝，避免旧版本程序错误修改由新版本创建的 schema。

## 迁移锁

| 数据库     | 最低版本 | 锁实现                                    | 生命周期       |
|------------|---------:|-------------------------------------------|----------------|
| MySQL      |      5.7 | `GET_LOCK` / `RELEASE_LOCK`               | 固定数据库会话 |
| MariaDB    |     10.5 | `GET_LOCK` / `RELEASE_LOCK`               | 固定数据库会话 |
| PostgreSQL |       14 | `pg_advisory_lock` / `pg_advisory_unlock` | 固定数据库会话 |
| SQLite     |     3.35 | `BEGIN IMMEDIATE`                         | 固定连接事务   |

锁名由当前数据库和 `_migrations` 派生。MySQL 锁名使用 SHA-256 限制为 64 个字符，PostgreSQL 使用稳定的 64 位哈希键；SQLite 依靠目标数据库文件的写事务锁，并通过事务感知连接让迁移内部的嵌套事务使用 SAVEPOINT。

MySQL 和 MariaDB 获取迁移锁前会验证当前数据库，DSN 未选定数据库时返回 `ErrDatabaseNotSelected`。

## 数据类型

### `datatype.JSON`

- 写入时区分空 Go 值对应的 SQL NULL 与 JSON `null`；读取 SQL NULL 后统一表示为 JSON `null`。
- 扫描数据库字节时复制缓冲区并验证 JSON。
- 使用 cast 转换数据库驱动返回的 `any`，转换失败时返回错误。
- `Get`、`Parse` 和 `Valid` 直接使用 gjson。
- `Unmarshal` 和 `MustUnmarshal` 使用 `core/codec/json` 反序列化为目标类型，`MustUnmarshal` 失败时 panic 并由 `core/codec/json` 统一打印堆栈。
- MySQL 使用 `JSON`，PostgreSQL 使用 `JSONB`，SQLite 使用 `JSON`；MariaDB 不支持 `CAST(... AS JSON)`，写入时退回直接字符串参数。
- MariaDB 判断复用 GORM MySQL Dialector 初始化时保存的 `ServerVersion`，SQL 构建阶段不会额外访问数据库。

### `datatype.DeletedAt`

- Go 底层类型为 `int64`。
- MySQL 和 PostgreSQL 使用 `BIGINT`，SQLite 使用 `INTEGER`。
- 使用 cast 转换数据库驱动返回的 `any`，并继续校验溢出和负数。
- 普通 Query 和 Update 自动增加 `deleted_at = 0`。
- Delete 转换为 UPDATE 并写入 `NowFunc().UnixMilli()`。
- `Unscoped` 查询包含已删除记录，`Unscoped().Delete` 执行物理删除。

## 测试结构

- `dbx` 测试参数校验、SQLite 默认连接池和日志 context 继承。
- `dbx/datatype` 测试 JSON 扫描、gjson 查询、DeletedAt 编解码和 GORM Clauses。
- `dbx/migrate` 使用临时 SQLite 文件测试排序、Up、Down、Reset 和空回滚函数。
- `tests` 使用临时 SQLite 文件覆盖打开、重复迁移、JSON 持久化和软删除。
- `tests/db` 通过 `TEST_MYSQL_DSN`、`TEST_MARIADB_DSN` 和 `TEST_POSTGRES_DSN` 显式注入 DSN 后执行增删改查集成测试，未设置时跳过；MariaDB 复用 mysql 驱动，SQLite 始终使用临时文件。
- CI 使用 docker-compose 启动 MySQL、MariaDB 和 PostgreSQL 后运行全部测试；服务均配置 healthcheck，`docker compose up --wait` 等待就绪。
- `tests/models/gen.go` 显式映射 JSON 和 DeletedAt，避免外部嵌入字段被生成器误判为关联结构体。
- 编译或 SQLite 测试不能证明 MySQL/PostgreSQL 真实连接、权限、锁等待或 DDL 行为。
