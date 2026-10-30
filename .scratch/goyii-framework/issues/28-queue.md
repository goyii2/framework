# 28: queue

Status: resolved

**What to build:** 异步队列端到端：定义 Job（`Name()` + `Execute(ctx)`，应用级注册表）→ Push（JSON 序列化载荷；options：Delay/Priority/Attempts/TTR/Backoff，默认 Attempts=1、TTR=5m、退避 `2^attempt` 秒封顶 15m）→ worker 子命令消费（listen 常驻 / run 单轮 / retry 重推 failed）。驱动 sync/db/redis（file 不做，db+SQLite 覆盖零依赖）；db 驱动自带建表迁移；reserved 超 TTR 自动回炉；重试耗尽置 failed 保留。push/exec 前后五事件走 base 事件调度。Application 懒初始化组件 + config `Queue{Driver, TableName, PollIntervalMs}` 段。

**Blocked by:** 26

- [x] Job 接口 + 应用级注册表：push 序列化 JSON 载荷、worker 按名重建执行
- [x] Push options：Delay/Priority/Attempts/TTR/Backoff（默认值如上，Backoff 可自定义函数）
- [x] sync 驱动：即推即执行、失败即返错（测试用）
- [x] db 驱动：自带建表迁移（对齐 rbac 迁移先例）；延迟、优先级、重试退避、failed 保留、超 TTR 回炉
- [x] redis 驱动：与 db 驱动同等语义（复用共享连接）
- [x] queue/commands 子命令 listen/run/retry（对齐 migrate/commands 挂载先例）
- [x] 五事件 BeforePush/AfterPush/BeforeExec/AfterExec/ExecError 走 base On/Trigger
- [x] config Queue 段 + Application 组件挂接
- [x] 单测：延迟、优先级、退避重试、失败重推、TTR 回炉（sync 与 db[sqlite] 必测；redis 驱动测试手段实现时定并说明）

## Comments

### 验收证据（2026-10-29，提交 94f8b05）

- Job + 注册表：`queue.Job`（`Name()` + `Execute(ctx)`，指针注册）；`queue.Registry` 按名重建（`Build` 反射 new 原型类型 + JSON 解码，原型不外泄）。`TestPushSerializesPayload`（载荷 == job JSON）、`TestRegistryBuildRebuildsFromPayload`（重建实例非原型、载荷解码、未知名/坏载荷报错）、`TestRegistryBasics`（重名/nil/空名/值类型拒绝，Names 排序）；端到端载荷送达：`TestRunDeliversPayload` / `TestDBRunThroughQueueDeliversPayload` / `TestRedisRunThroughQueueDeliversPayload`。
- Push options：`WithDelay/WithPriority/WithAttempts/WithTTR/WithBackoff`；默认 Attempts=1、TTR=5m（`DefaultTTR`）、退避 `2^attempt` 秒封顶 15m（`DefaultBackoff`；`TestDefaultBackoff` 覆盖 1/2/3/9/10(越顶)/12/40/0/负数）。`TestPushOptionsDefaults`、`TestPushOptionsFunctional`（含 Attempts<1 钳到 1）。Backoff 函数在 push 时解析成延迟表随消息存储——跨进程重试不依赖闭包（`TestRunRetryDelayFallsBackToDefault` 兜底缺表回退默认）。
- sync 驱动：`TestSyncExecutesInline`（push 内联执行 + 五事件序）、`TestSyncFailureReturnsToPusher`（失败直接返错给 pusher，Attempts 忽略——无存储无重试，文档注释明确）、`TestSyncIgnoresWorkerSurface`（Run=0、Listen 随 ctx 退出、RetryFailed=0）。
- db 驱动（sqlite）：`queue.NewMigration(table)` 对齐 rbac 先例（`m20260101_000004_create_queue_table`，dispatch 索引 status/priority/due_at/id；`TestDBMigrationCreatesUsableTable` 默认表名+自定义表名、`TestDBCustomTableNameIsolation`）。延迟 `TestDBDelayHoldsUntilDue`；优先级 `TestDBPriorityOrdersReserve`（[10 5 1]）；退避重试 `TestDBEndToEndRetryAndBackoff`（失败后 400ms 退避窗口不可取、窗口过后可取、成功后 Complete）；failed 保留+重推 `TestDBAttemptsExhaustedKeptFailedAndRetryFailed`（failed 保留不可取、RetryFailed=1 且尝试次数归零、新周期再失败照常 requeue）；TTR 回炉 `TestDBTTRReclaimRequeuesReservation`（持有预留越过 TTR 自动回 waiting、attempt 不变）。互斥领取：guarded UPDATE + claim token，`TestDBReserveIsExclusiveAcrossWorkers`（4 worker goroutine × 2 驱动实例争 20 条，各恰一次）；`TestDBConcurrentQueuePush`（8×10 并发 push 全部落队）。
- redis 驱动（miniredis，26 已引入的依赖）：与 db 同等语义逐条对齐——`TestRedisPushReserveComplete`、`TestRedisDelayHoldsUntilDue`、`TestRedisPriorityOrdersReserve`、`TestRedisReserveIsExclusiveAcrossWorkers`、`TestRedisEndToEndRetryAndBackoff`、`TestRedisAttemptsExhaustedKeptFailedAndRetryFailed`（failed zset 保留断言）、`TestRedisTTRReclaimRequeuesReservation`、`TestRedisCustomPrefixIsolatesQueues`。实现要点：优先级分桶 zset + ZREM 竞争领取；分数用 unix 毫秒（float64 53 位安全域内——纳秒会因舍入翻转 due 判定，已实测修复）；两命令领取窗口（ZREM→ZADD）在 Reserve 文档注释明确说明，db 驱动单语句领取无此窗口。
- 子命令：`queue/commands`（`New(app)` 挂 `queue listen/run/retry`，对齐 migrate/commands）；listen 走 SIGINT/SIGTERM + ctx 双通道优雅退出。`TestCommandTreeShape`、`TestRunDrainsDueMessages`、`TestRetryRepushesFailed`、`TestRunUnknownJobFailsTheMessageNotTheCommand`（未注册名停 failed 不崩命令）、`TestListenStopsOnCanceledContext`。
- 五事件：`queue.EventBeforePush/AfterPush/BeforeExec/AfterExec/ExecError`（untyped 常量，调用点自动转 `base.EventName`，无导入环）；`Queue.WithDispatcher` 注入适配器路由进 `app.Trigger`。核心序：`TestEventSequence`（[BeforePush AfterPush BeforeExec AfterExec] + PushEventData/ExecEventData 字段）、`TestExecErrorEventCarriesError`（错误与尝试号）；经 base 的端到端：`TestApplicationQueueEventsFlowThroughBase`（app.On 收五事件 + AfterPush Data）。未知/坏载荷任务：`TestRunUnknownJobParksFailed`（ExecError + 停 failed 不打转）。
- config + Application：`config.Queue{Driver,TableName,PollIntervalMs}`（默认 sync/queue/1000ms；`TestLoadQueueSection`、`TestQueueDefaults`、`TestQueueEnvOverrides`——QUEUE_DRIVER/QUEUE_TABLENAME/QUEUE_POLLINTERVALMS）。`app.Queue()/app.SetQueue()` 懒初始化（`TestApplicationQueueComponent`）；默认 sync 零 db/redis 开销（`TestApplicationQueueDefaultSyncZeroOverhead`）；db 驱动走 app.DB 且经 NewMigration 建表端到端（`TestApplicationQueueDbDriver`）；redis 走 26 共享连接、键落 `goyii:queue:` 前缀（`TestApplicationQueueRedisUsesSharedConnection`）；未知驱动/db 打不开首用报错不炸装配（`TestApplicationQueueUnknownDriverFailsOnUse`、`TestApplicationQueueDbWithoutConnectionFailsOnUse`）。
- 测试摘要：`go test ./...` 24 包全绿（queue 45 用例、queue/commands 5、base 队列相关 7、config Queue 3）；queue 包 `-count=3` 重复运行稳定（db 时序余量按慢盘 fsync 尖峰 >200ms 放宽到 400ms+）；`go vet ./...`、`golangci-lint run ./queue/... ./base/... ./config/...` 0 issues；gofmt 无差异。本机无 C 工具链 `-race` 不可用（与 27 相同限制），并发正确性以多轮重复 + 4 worker 竞争测试覆盖。
- 文档：`docs/配置参考.md` 增 queue 段；`docs/组件-base与config.md` 增 `app.Queue()` 组件行与队列语义小节（注册名+JSON 载荷、状态机、退避表跨进程、sync 忽略重试的边界）。
