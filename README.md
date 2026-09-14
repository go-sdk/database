# database

`database` 是个人使用的 Go 数据库基础类库，模块路径为 `github.com/go-sdk/database`。项目基于 GORM 统一 MySQL、PostgreSQL 和 SQLite 的初始化、日志、版本迁移、跨副本迁移锁、JSON 字段和毫秒时间戳软删除行为。

## 环境要求

- Go 1.27 或更高版本
- MySQL 5.7 或更高版本，或 MariaDB 10.5 或更高版本
- PostgreSQL 14 或更高版本
- SQLite 3.35 或更高版本

最低版本同时考虑当前数据库驱动的维护范围和迁移锁实现。SQLite 由纯 Go 驱动提供，不要求 CGO。

## 安装

```bash
go get github.com/go-sdk/database
```

## 打开数据库

只导入实际使用的数据库驱动：

```go
import (
	"github.com/go-sdk/database/dbx"
	_ "github.com/go-sdk/database/dbx/mysql"
)

db, err := dbx.Open("mysql", "user:password@tcp(localhost:3306)/app")
```

PostgreSQL 和 SQLite 分别使用：

```go
import _ "github.com/go-sdk/database/dbx/postgres"
db, err := dbx.Open("postgres", "postgres://user:password@localhost/app?sslmode=verify-full")

import _ "github.com/go-sdk/database/dbx/sqlite"
db, err := dbx.Open("sqlite", "app.db")
```

`dbx` 主包不导入具体数据库驱动。没有空白导入对应包时，`Open` 返回 `dbx.ErrDriverNotRegistered`。

`dbx.Open` 始终启用 GORM `TranslateError`，即使通过 `WithGORMConfig` 传入 `false` 也不会关闭，确保三个数据库使用统一的 GORM 错误语义。`WithGORMConfig` 的其他字段仍覆盖默认配置。

MySQL 默认启用时间解析；SQLite 默认开启 foreign key 和五秒 busy timeout。默认连接池为：

| 数据库     | MaxIdleConns | MaxOpenConns | ConnMaxLifetime | ConnMaxIdleTime |
|------------|-------------:|-------------:|----------------:|----------------:|
| MySQL      |           10 |          100 |          3 分钟 |          1 分钟 |
| PostgreSQL |           10 |          100 |         30 分钟 |          5 分钟 |
| SQLite     |            1 |            1 |          不限制 |          不限制 |

可以覆盖连接池配置：

```go
db, err := dbx.Open("postgres", dsn, dbx.WithPoolConfig(dbx.PoolConfig{
	MaxIdleConns:    20,
	MaxOpenConns:    200,
	ConnMaxLifetime: 30 * time.Minute,
	ConnMaxIdleTime: 5 * time.Minute,
}))
```

连接池默认值也可以通过 `core/config` 覆盖。数据库专用配置优先于通用配置，`WithPoolConfig` 又优先于配置文件和 `APP__` 环境变量：

| 参数            | 通用配置                              | PostgreSQL 专用配置示例                          |
|-----------------|---------------------------------------|--------------------------------------------------|
| MaxIdleConns    | `database.pool.max_idle_conns`        | `database.postgres.pool.max_idle_conns`          |
| MaxOpenConns    | `database.pool.max_open_conns`        | `database.postgres.pool.max_open_conns`          |
| ConnMaxLifetime | `database.pool.conn_max_lifetime`     | `database.postgres.pool.conn_max_lifetime`       |
| ConnMaxIdleTime | `database.pool.conn_max_idle_time`    | `database.postgres.pool.conn_max_idle_time`      |

数据库专用路径中的驱动名为 `mysql`、`postgres` 或 `sqlite`。连接数使用整数，时间使用 Go duration，例如 `30m`、`5m` 或 `0`。环境变量遵循 `core/config` 的路径规则，例如 `APP__DATABASE__POOL__MAX_OPEN_CONNS=200` 和 `APP__DATABASE__POSTGRES__POOL__MAX_OPEN_CONNS=300`。

```yaml
database:
  pool:
    max_idle_conns: 20
    max_open_conns: 200
    conn_max_lifetime: 30m
    conn_max_idle_time: 5m
  postgres:
    pool:
      max_open_conns: 300
```

连接池配置在每次 `Open` 时读取，因此应用可以在打开数据库前通过 `core/config.SetDefault` 安装自己的配置实例。

`Open` 通过 `core/lifex` 注册底层 `sql.DB` 的关闭函数，但不会执行数据库迁移。

## 数据库迁移

迁移与数据库初始化相互独立。推荐在应用的迁移包中声明集合，并让每个 Go 文件注册一个迁移：

```go
// migration.go
var migrations migrate.Migrations

func New(db *gorm.DB) (*migrate.Migrator, error) {
	return migrate.New(db, migrations)
}
```

```go
// 20260914_120000_01_create_users.go
func init() {
	migrations.Add(
		func(tx *gorm.DB) error {
			return tx.AutoMigrate(&User{})
		},
		func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&User{})
		},
	)
}
```

`Migrations.Add` 从直接调用方文件名去掉 `.go` 后得到迁移 ID。每个迁移文件只能注册一个迁移，且必须直接调用 `Add`，不能通过公共 helper 间接调用。迁移文件一旦发布不得重命名，否则数据库会将新文件名识别为新的迁移。

仍然支持显式构造迁移列表：

```go
import "github.com/go-sdk/database/dbx/migrate"

migrator, err := migrate.New(db, migrate.Migrations{
	{
		ID: "20260913_153000_01_create_users",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&User{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&User{})
		},
	},
})
if err != nil {
	return err
}

if err := migrator.Up(ctx); err != nil {
	return err
}
```

迁移 ID 必须使用：

```text
YYYYMMDD_HHMMSS_NN_description
```

日期和时间必须有效，`NN` 是两位排序号，说明只使用小写英文、数字和下划线。无论通过 `Add` 还是显式列表注册，Migrator 都会复制列表、校验重复 ID 和空 `Up`，再按 ID 升序排序。

公开操作：

```go
err = migrator.Up(ctx)    // 执行全部尚未应用的迁移
err = migrator.Down(ctx)  // 回滚最后一个已应用迁移
err = migrator.Reset(ctx) // 逆序回滚全部已应用迁移
```

`Down` 函数允许为空。`Down` 或 `Reset` 遇到这种迁移时，不执行 schema 回滚，但会从 `_migrations` 删除对应记录。

迁移记录表固定为 `_migrations`。未知迁移默认被拒绝，也可以使用 `WithValidateUnknownMigrations(false)` 放宽；锁等待默认 30 秒，可通过 `WithLockTimeout` 调整，传入 `0` 表示只尝试一次而不等待，负值会返回参数错误。

MySQL 和 MariaDB 的 DSN 必须包含目标数据库名；未选定数据库时，迁移操作返回 `migrate.ErrDatabaseNotSelected`，不会在服务器级空命名空间中获取迁移锁或把回滚误报为成功。

### 多副本迁移锁

| 数据库     | 最低支持版本 | 实现                                     |
|------------|-------------:|------------------------------------------|
| MySQL      |          5.7 | 固定会话上的 `GET_LOCK` / `RELEASE_LOCK` |
| MariaDB    |         10.5 | 固定会话上的 `GET_LOCK` / `RELEASE_LOCK` |
| PostgreSQL |           14 | 固定会话上的 session-level advisory lock |
| SQLite     |         3.35 | 固定连接上的 `BEGIN IMMEDIATE` 写事务    |

MySQL 命名锁不会因 DDL 隐式提交而释放；PostgreSQL 使用会话锁而不是事务锁。SQLite 要求所有副本访问同一个数据库文件，并依赖底层文件系统正确实现 SQLite 文件锁。

每个实际执行的迁移都会记录开始及成功或失败日志。每次 `Up`、`Down`、`Reset` 结束后还会记录汇总日志，包括操作类型、状态、执行数量、起始版本、结束版本和总耗时。日志通过调用时的 context 获取 `core/logx` Logger。

SQLite 在固定连接上通过 `BEGIN IMMEDIATE` 获取写锁，并让 GORM 的嵌套事务使用 SAVEPOINT，避免 `_migrations` 写入或迁移函数内部事务重复执行 `BEGIN`。

## GORM 日志

`dbx.Open` 默认注册 `core/logx` GORM Logger。使用 GORM 泛型 API 传入 context，或使用传统 API 的 `WithContext`：

```go
users, err := gorm.G[User](db).Find(ctx)
err = db.WithContext(ctx).First(&user).Error
```

`server` 已将带有 `trace-id`、`span-id` 和 depth 的 zerolog Logger 附加到请求 context。database 只调用 `logx.Ctx(ctx)`，不会反向依赖 server，SQL 日志会自动继承这些字段。

默认行为：

- 正常模式记录错误和超过 200ms 的慢查询。
- debug 模式记录普通 SQL。
- 记录展开参数后的 SQL、影响行数和耗时。
- `source` 字段指向触发 SQL 的业务代码位置，自动跳过 GORM 生态和 dbx 内部帧。
- 默认忽略 `record not found` 日志，但错误仍返回调用方。

SQL 参数可能包含邮箱、证书信息或其他敏感业务数据。不得把密码、私钥、令牌和访问密钥作为可记录的 SQL 参数；需要隐藏参数时应通过 `WithGORMConfig` 注入自定义 Logger。

## JSON

```go
type User struct {
	Extra datatype.JSON `gorm:"comment:额外信息"`
}

language := user.Extra.Get("profile.language").String()
enabled := user.Extra.Get("enabled").Bool()
valid := user.Extra.Valid()
root := user.Extra.Parse()
```

数据库字段类型为 MySQL `JSON`、PostgreSQL `JSONB` 和 SQLite `JSON`。写入时，空 Go 值写为 SQL NULL，JSON `null` 仍作为 JSON 文档保存；从数据库读取后，SQL NULL 统一表示为 JSON `null`，两者不再区分。MySQL 写入使用 `CAST(? AS JSON)`；MariaDB 不支持该语法，因此根据 GORM 初始化时保存的服务端版本自动退回直接字符串参数，不会在 SQL 构建阶段额外查询数据库。

JSON 和 DeletedAt 的数据库 `Scan(any)` 使用 cast 做基础类型转换，并在转换后继续校验 JSON 合法性、整数溢出和负时间戳，避免无错误的截断或非法持久化。

## 软删除

`dbx.Metadata` 已包含 `datatype.DeletedAt`：

```go
type User struct {
	dbx.Metadata
	Name string
}
```

主键 `id` 为无数据库默认值的字符串主键，新增记录时必须显式填充，推荐使用 `core/seq.NextID()`：

```go
user := User{Metadata: dbx.Metadata{Id: seq.NextID()}, Name: "alice"}
```

普通查询自动增加：

```sql
WHERE deleted_at = 0
```

删除转换为更新并写入 Unix 毫秒时间戳：

```sql
UPDATE users SET deleted_at = 1723456789123 WHERE id = ? AND deleted_at = 0
```

MySQL 和 PostgreSQL 使用 `BIGINT`，SQLite 使用 `INTEGER`。通过 `Unscoped` 查询已删除数据或执行物理删除：

```go
db.Unscoped().Find(&users)
db.Unscoped().Delete(&user)
```

整数零值可以安全参与联合唯一索引，例如业务字段与 `deleted_at` 共同组成唯一索引，使软删除后可以重新创建相同业务键。

## 真实数据库测试

`tests/db` 提供针对 MySQL、MariaDB、PostgreSQL 和 SQLite 的增删改查集成测试。SQLite 始终使用临时数据库运行；其余测试通过环境变量显式注入 DSN，未设置时自动跳过：

| 数据库     | 环境变量            | 示例 DSN                                                           |
|------------|---------------------|--------------------------------------------------------------------|
| MySQL      | `TEST_MYSQL_DSN`    | `root:12345678@tcp(127.0.0.1:3001)/test`                           |
| MariaDB    | `TEST_MARIADB_DSN`  | `root:12345678@tcp(127.0.0.1:3002)/test`                           |
| PostgreSQL | `TEST_POSTGRES_DSN` | `postgres://postgres:12345678@127.0.0.1:3003/test?sslmode=disable` |

MariaDB 兼容 MySQL 协议，复用 `dbx/mysql` 驱动。本地可以执行 `make local-test`，它使用 `docker-compose.yml` 启动三个数据库并等待就绪后注入 DSN，只运行 `tests/db` 数据库集成测试，结束后无论成功与否都会停止并移除容器：

```bash
make local-test
```

CI 在 `.github/workflows/golang.yml` 中执行同样的流程。

## 开发约定

修改代码前先阅读 `AGENTS.md`、`PROJECT_MAP.md` 和本文件。发现文档与实现不一致时，应在同一次修改中更新。

```bash
make generate    # 生成 tests/modelg
make lint        # 整理依赖并执行 golangci-lint
make test        # 使用竞态检测运行测试
make local-test  # 启动本地 docker 数据库并运行全部测试
make build       # 编译 tests/db 示例
```

未注入 DSN 时，默认测试只使用临时 SQLite 数据库，不访问真实 MySQL、MariaDB 或 PostgreSQL。编译和本地 SQLite 测试不能证明生产数据库权限、网络、锁等待和具体 DDL 的兼容性。
