# 01: migrate 多目标（--db 具名数据库目标）

Status: resolved
Blocked by: None（可立即开工）

**What to build:** `migrate --db=stats up` 能对项目装配注入的副库执行迁移：migrate 命令树接收具名目标（`migratecommands.Target(name, conn, migrations)` 可变参数），`--db` 挂根命令为 persistent flag，缺省仍为主库；up/down/redo/history/new/mark/to 全支持，未知目标报错列出已注册名，重名装配 panic；examples/basic 加 stats 副库演示目标（独立 sqlite + 自有迁移列表），docs/配置参考.md 补「多数据库」小节。详见同目录 spec.md。

- [x] migratecommands.New 接受可变 Target 参数；--db persistent flag 生效于七个数据库子命令；缺省行为与现状完全一致
- [x] 未知 --db 值报错并列出已注册目标名；重名目标装配期 panic；有测试
- [x] --db=stats 全链路有测试（up/history 针对副库 sqlite 生效，主库不受影响）
- [x] examples/basic：stats 副库目标可 `migrate --db=stats up` 冒烟通过；主库冒烟不回归
- [x] docs/配置参考.md「多数据库」小节与示例一致
- [x] go build ./... && go test ./... 全绿；lint/gofmt 干净

## Comments

- 实施：`migrate/commands/commands.go` 增 `Target`（Name/Conn/Migrations）与 `migrateTree`，`--db` 为根命令 persistent flag，`openMigrator` 按 flag 解析连接与迁移列表；`create` 不变（只生成文件）。
- 测试（`migrate/commands/commands_test.go`，包内 29 例全绿）：`TestMigrateNamedTargetRunsOnItsOwnDatabase`（副库 up/history 生效、主库零触碰、随后默认 up 主库正常）、`TestMigrateUnknownTargetListsRegisteredNames`、`TestMigrateDuplicateTargetPanicsAtAssembly`。
- 调试记录：验收用例曾稳定失败，根因是 cobra 的 flag 值在同一命令树上跨 Execute 残留——测试助手 `runMigrate` 复用命令树时，前一次 `--db=stats` 会把后续不带 flag 的 `up` 也路由到副库（迁移全已应用 → 零输出、主库从未被建表）。产品无此问题（真实进程单次 Execute）。修复：`runMigrate` 每次调用前复位 `--db`（每次调用即新进程语义）。
- 示例冒烟：`go run . migrate --db=stats up` / `history`（列出 m20260101_000001_create_visit_log）/ `new`（空）；`runtime/stats.sqlite`(+wal/shm) 生成；`migrate up` 主库历史完好；`--db=analytics` 报错并列出 `stats`。
- 全量：go build 通过；go test 1449/39 全绿；golangci-lint 0 issues；gofmt 无差异。
