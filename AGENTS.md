# 仓库协作规范

## 文档优先

每次修改代码前，必须先阅读以下文档：

1. `AGENTS.md`
2. `PROJECT_MAP.md`
3. `README.md`

如果文档描述与实际代码不一致，应在同一次修改中更新相关文档，确保文档反映最终实现。

## 修改原则

- 这是个人 Go 数据库基础类库，优先保持驱动隔离、公共行为一致和向后兼容。
- 修改前检查目录结构、调用关系和 Git 工作区状态，不覆盖或混入无关改动。
- 遵循现有包划分和代码风格，优先局部修改，不引入缺少明确收益的依赖或抽象。
- 代码注释、维护文档和面向使用者的说明统一使用简体中文。
- 注释只描述最终设计意图、业务含义和关键约束，不记录修改历史，不复述代码。
- 错误创建和包装优先使用 `core/errx`；错误文本和日志消息使用小写字母开头。

## 设计约束

- `dbx` 主包不得直接导入 MySQL、PostgreSQL 或 SQLite 驱动。应用通过空白导入 `dbx/mysql`、`dbx/postgres` 或 `dbx/sqlite` 按需注册驱动。
- `dbx.Open` 只负责打开连接、应用默认配置、注册 `core/logx` GORM Logger 和通过 `core/lifex` 管理关闭，不得隐式执行迁移。
- `dbx.Open` 始终启用 GORM `TranslateError`，调用方传入的配置不能关闭该行为。
- 连接池默认值从 `core/config` 的 `database.pool.*` 读取，数据库专用的 `database.<driver>.pool.*` 优先，显式 `WithPoolConfig` 的优先级最高。
- 数据库迁移统一由 `dbx/migrate` 管理，迁移表固定为 `_migrations`。
- 迁移 ID 使用 `YYYYMMDD_HHMMSS_NN_description`，构造 Migrator 时校验并排序。
- `Migrations.Add` 从直接调用方的 Go 文件名生成迁移 ID，一个迁移文件只注册一个迁移；已发布的迁移文件不得重命名。
- `Up`、`Down` 和 `Reset` 必须在数据库级互斥锁内执行；锁必须绑定同一个物理连接，不能依赖连接池碰巧复用连接。
- MySQL 和 MariaDB 执行迁移前必须已经选定目标数据库，DSN 未包含数据库名时返回错误。
- `Down` 回滚最后一个已应用版本，`Reset` 逆序回滚全部已应用版本；迁移未定义 `Down` 时仅移除迁移记录，不返回错误。
- MySQL 使用 `GET_LOCK`，PostgreSQL 使用 session-level advisory lock，SQLite 使用固定连接上的 `BEGIN IMMEDIATE`。
- 迁移需要记录各步骤开始和结果，并在操作结束时记录执行数量、起止版本和总耗时。
- `datatype.DeletedAt` 使用 `0` 表示有效记录，删除时写入 Unix 毫秒时间戳；普通查询和更新只处理 `deleted_at = 0` 的记录。
- `datatype.JSON` 在 MySQL、PostgreSQL、SQLite 中分别映射为 `JSON`、`JSONB`、`JSON`，并通过 gjson 提供进程内路径读取。
- 本仓库当前不提供租户字段或自动租户隔离。
- GORM Logger 必须使用传入的 context 获取 `core/logx` Logger，使 server 注入的 `trace-id` 和 `span-id` 自动进入数据库日志。
- SQL 日志按当前需求记录参数展开后的 SQL。调用方不得把密码、私钥、令牌或其他秘密作为 SQL 参数；涉及敏感业务字段时应降低日志级别或注入自定义 Logger。

## 数据库支持边界

- MySQL 5.7 及以上；MariaDB 10.5 及以上。
- PostgreSQL 14 及以上。
- SQLite 3.35 及以上。
- SQLite 多进程迁移锁要求各进程访问同一个数据库文件，并且底层文件系统正确支持 SQLite 文件锁。

## 验证边界

- 修改后使用项目已有的格式化工具，并检查 `git diff`、`git diff --check` 和工作区状态。
- 优先依次运行 `make lint` 和纯编译检查；不得把静态检查或编译成功描述为运行时验证。
- 未经用户明确授权，不运行单元、集成、端到端或冒烟测试，不启动示例程序，不访问真实 MySQL、PostgreSQL 或其他外部系统。
- 默认测试只能使用临时 SQLite 数据库或测试替身；真实 MySQL/PostgreSQL 测试必须显式注入 DSN 并单独授权。
- 不执行部署、发布、上传或 `git push`；本地提交需要用户明确要求。

## 提交规范

- 提交信息遵循 Conventional Commits。
- 标题使用简洁英文；需要补充背景、影响或风险时写入提交正文，正文每行不超过 80 个字符。
- 只提交当前任务明确包含并已复核的文件。
