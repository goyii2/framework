# 07: DB 组件 + yiimigrate 并入

**Status:** resolved

**What to build:** 数据库接入（ADR-0004）：GORM 连接作为 db 组件懒初始化（DSN 来自应用配置，兼容 DB_DSN 环境变量）；yiimigrate 源码并入 migrate 包，保持 database/sql 直连、不引 ORM 的原架构；`goyii migrate` 暴露八命令（up/down/redo/history/new/mark/to/create），历史表、方言锁、dry-run 按原语义。

**Blocked by:** 02, 06

- [x] 首次访问建立 GORM 连接；DSN 从应用配置注入，DB_DSN 环境变量兼容
- [x] yiimigrate 源码并入后编译通过、原测试随之迁移并可运行（以其测试为准绳）
- [x] 对 SQLite 完成 create -> up -> down -> redo 全流程验收
- [x] dry-run 与迁移锁行为保持原语义

## Comments

**落地形态**（main `ea622e2`，分支 feat/db-and-migrate 已 ff 合入并删除）：

- **DB 组件（db/）**：`db.Conn` 包装 `*gorm.DB`（DML 层）；`db.Component` 是懒初始化持有器——首次 `Get()` 才连接、失败不缓存（下次重试）、`Set(nil)` 复位回懒初始化，并发安全。`base.Application` 持有它：`app.DB() (*db.Conn, error)` / `app.SetDB(conn)`。DSN 解析：`DB_DSN` 环境变量优先于 `config.DB.DSN`；driver 空（含别名 sqlite3/postgresql/mssql 归一）默认 mysql，与迁移命令同一默认。SQLite 走纯 Go `github.com/glebarez/sqlite`（无 CGO），mysql/postgres/sqlserver 走 gorm 官方 dialector。
- **yiimigrate 并入（migrate/）**：原仓库 `migrate/` 27 个文件逐字并入（方言四件套、migrator、schema plan、column builder、query helper、一致性矩阵等全部测试）；`internal/generator` 并入 `migrate/internal/generator`（生成模板的 migrate 导入路径改为 `github.com/goyii2/framework/migrate`）。原文件无版权头，包 doc.go 记录来源与作者署名（github.com/phgors/yiimigrate，维护者本人授权并入）。migrate 包零 GORM 依赖（`grep gorm migrate/` 为空），保持 database/sql 直连四方言。
- **八命令（migrate/commands/）**：原 `cmd/migrate/main.go`（flag 包 CLI）适配为 cobra 子命令树 `goyii migrate <up|down|redo|history|new|mark|to|create>`，经 `console.AddCommand(commands.New(app, migrations.All()))` 挂入——console 本身不依赖任何 DB 驱动，未挂迁移命令的应用不链接驱动。连接从 `config.DB` 解析；`DB_DSN`/`DB_DIALECT`/`MIGRATE_TABLE`/`MIGRATE_DRY_RUN` 环境变量保留原独立 CLI 语义（环境优先）。create 生成目录 `migrations/`、默认方言随应用配置（DB_DIALECT 仍可覆盖）。
- **调整点（相对原仓库）**：① sqlite 的 database/sql 驱动由 `modernc.org/sqlite` 换为 `github.com/glebarez/go-sqlite`——与 GORM 的 glebarez 驱动共用同一 "sqlite" 注册，两层共存不重复注册（两个包各自 `sql.Register("sqlite")`，并存会 init panic）；② sqlite 无 advisory lock，命令层探测方言 `AcquireLockSQL`，不可生成锁语句的方言（sqlite）`UseLock=false` 直接执行而非每条命令报错——mysql/postgres/sqlserver 锁语义不变（GET_LOCK / pg_advisory_lock / sp_getapplock）；③ `migrations/register.go` 桩未并入：迁移清单是应用侧资产，经 `commands.New(app, migrations)` 参数注入。

**验收证据**（合并后 main 上，CGO 经 scoop mingw gcc 启用）：

- `go build ./...` / `go vet ./...` / `gofmt -l .` 全部无输出。
- `CGO_ENABLED=1 go test -race -count=1 ./...` -> 全绿（无 data race）；全模块 161 个顶层测试 PASS，其中本票涉及 5 包 110 个（base / db / migrate / migrate/commands / migrate/internal/generator）。
- **原测试随迁可运行**：yiimigrate 的 migrate 包测试（含 817 行方言一致性矩阵、migrator fake-driver 全套、schema plan、column builder、dialect registry）与 generator 全套测试逐字迁移后全部 PASS；mysql/postgres/sqlserver 集成测试随迁，无 `*_TEST_DSN` 时按原样 skip。
- **SQLite 全流程**（`TestSQLiteCreateUpDownRedoFlow`）：`goyii migrate create create_post_table --fields=title:string(128):notNull` 生成 `migrations/m*_create_post_table.go`（含 `Add("id", m.PrimaryKey())` sqlite 模板、framework migrate 导入路径，go/parser 校验可解析）→ `up` 建出 first/second 两表 + 历史表 2 行 → `down`（默认 1）只回滚 second → `up` 补齐 → `redo`（默认 1）回滚并重建 second、历史回到 2 行，每步断言 sqlite_master 与 migration 表。
- **语义保持**：`TestMigrateHistoryNewToMarkSemantics` 覆盖 new/history（新到旧）/to（回退到指定版本）/mark（只改历史不执行，second 表保持不存在）；`TestMigrateDryRunExecutesNothing`：`MIGRATE_DRY_RUN=1` 下 up 后用户表与历史表均不存在（只打印 SQL）；`TestDialectLockableFollowsDialectLockSupport`：mysql 锁开启、sqlite 无锁直跑；锁获取/释放行为本体由随迁的 migrator 测试（fake driver 锁路径）覆盖。`TestMigrateDSNFromEnvironment`：config 无 DSN 时 `DB_DSN` 环境变量注入后 up 成功。
- **DSN 注入**（db 包）：`TestConnectDSNEnvOverridesConfiguredDSN` 断言环境 DSN 覆盖配置 DSN；`TestComponentOpensLazilyAndCaches`/`TestComponentRetriesAfterFailedOpen`/`TestComponentConcurrentGetOpensOnce`（-race 下 16 goroutine 单次开连）；base 侧 `TestApplicationDBComponent` 覆盖懒开/替换/复位，`TestApplicationDBWithoutDSNFailsWithConfigHint` 报错指明 DB_DSN 兜底。

**遗留**：gorm logger 仍为默认 stdout 输出（warn 级），与 app.Log 的整合留给 log 票；examples/ 的可运行示例（sqlite 端到端）在 15 号 P0 集成票统一落盘；golangci-lint 本机版本与 Go 1.27 stdlib 不兼容（06 号票已记录），本票继续以 go vet + gofmt 干净替代。
