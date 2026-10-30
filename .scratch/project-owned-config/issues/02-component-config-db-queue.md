# 02: 组件包新形态·大组件（db / queue）

Status: resolved
Blocked by: None（可与 01 并行）

**What to build:** db 与 queue 完成统一初始化形态：db 构造器统一为 `New(cfg)` 并把 Config 参数包迁入包内（原 Connect 语义并入，现有 Option 能力并入 With 族）；queue 的 sync/db/redis 驱动 switch 从 base 包迁回 queue 包，Config 迁入，db 连接与 redis 客户端经 `WithDB` / `WithRedis` 选项注入，构造期返回错误。`db.Component` 懒 holder 保留为「想要懒 / 可重试语义」的可选件，不再是主装配路径。

- [x] db 包导出 Config 与 `New(cfg, opts...)`，Connect 导出移除，现有 Option 能力并入 With 族
- [x] queue 驱动选择 switch 全部位于 queue 包内；base 不再包含任何 queue 驱动分支
- [x] queue 依赖经 `WithDB(*db.Conn)` / `WithRedis(UniversalClient)` 注入；缺必需依赖时构造期报错
- [x] db.Component 懒 holder 保留且测试通过（可选件地位）
- [x] `go build ./...` 与 `go test ./...` 全绿

## Comments

- 2026-10-30 实施记录（分支 feat/project-owned-config-02，已 --ff-only 合入 main，commit 5959c91）：
  - 参数包落位：`db.Config{Driver,DSN}`（db/config.go）、`queue.Config{Driver,TableName,PollIntervalMs}`（queue/config.go）；字段名/类型/yaml tag 与 config 包 section 一致。queue 的 db/redis 驱动均共用应用级连接（队列跨进程协调，专用连接会发出别人看不到的消息），故无需 db/redis 子段。
  - db 构造器：`New(cfg, opts...) (*Conn, error)`（原 Connect 语义全量并入：DB_DSN env 覆盖、驱动别名、DSN 必填）；`WithGormLogger` 保留（本就是 With 族依赖注入位）；Connect 导出删除、无 shim。`Component` 懒 holder 改持 `Config`，注释明确「急切装配（New）之外的可选懒/可重试件」，三条 Component 测试原样通过。
  - queue 构造器：`New(cfg, opts...) (*Queue, error)`，驱动 switch 全部在 queue.New（""/sync → Sync、db → NewDB、redis → NewRedis、其余 unknown driver 报错）；零值 Config = sync；TableName 空 → "queue"、PollIntervalMs <= 0 → 1s 默认内聚于 New。依赖注入：`WithDB(*db.Conn)` / `WithRedis(UniversalClient)`，缺依赖/传 nil/懒解析失败均在构造期报错（errDBRequired / errRedisRequired + resolveDB/resolveRedis 包装底层原因）。另设 `WithDBFunc` / `WithRedisFunc` 懒注入位：仅对应驱动在构造期解析，sync 驱动零外部依赖（base 过渡保语义所需，03 重审）。原程序化 `New(d, jobs, opts...)` 改名 `NewWithDriver`（与 01 的 log.NewWithTargets 同位）；`WithDispatcher` / `WithPollInterval` 并入统一 `Option`（QueueOption 类型移除）。
  - base 过渡：newDefaultQueue 为纯等值转换——config.Queue → queue.Config 逐字段转换，依赖经 `WithDBFunc(dbComp.Get)` / `WithRedisFunc(shared.get)` 懒注入，base 内零驱动分支（driver 仅出现在注释与值透传）；构造被拒时经 `queue.Failing(err)` 包 NewWithDriver 保持「首用报错」语义，既有 base 测试（unknown driver / db 无连接首用失败、sync 不初始化 redis、db/redis 端到端）全部原样通过。
  - 调用点一次迁移（干净破坏）：gii/commands openConn、base 装配点与 behaviors/dataprovider/db-ar/gii/rbac/validators/queue 共 17 处测试 `db.Connect(config.DB{...})` → `db.New(db.Config{...})`；docs/组件-db与ActiveRecord.md 单行片段同步（引用了被删 API）。config 包本票零改动（03 收口）。
- 2026-10-30 验收证据：`go build ./...` 通过；`go test ./...` 全绿（1463 tests / 35 packages，无 FAIL）；`golangci-lint run`（db/queue/base/gii）0 issues；gofmt 无差异。新测试：db 侧 Connect 系测试随 API 更名后全量通过；queue 侧新增 config_test.go 9 例——`TestNewZeroConfigIsSync`（零值→sync+默认 poll 1s+registry 就位）、`TestNewDispatch`（三驱动分发、大小写/空白容忍、表名默认）、`TestNewDbDriverPushesAndRuns`（db 驱动 New 端到端）、`TestNewPollIntervalFromConfig`、`TestNewRejectsUnknownDriver`、`TestNewDbDriverRequiresConnection`（缺 WithDB / WithDB(nil) / 懒开失败三例）、`TestNewRedisDriverRequiresClient`（缺 WithRedis / nil 两例）、`TestNewIgnoresForeignDependencies`（sync 不触碰 db/redis 依赖）、`TestNewWithDriverKeepsProgrammaticConstruction`。遗留：`WithDBFunc` / `WithRedisFunc` 与 base/queue.go 过渡体随 03（Application 重构）一并重审/拆除。
