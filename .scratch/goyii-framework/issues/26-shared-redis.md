# 26: redis 共享连接

**Status:** resolved

**What to build:** 多处 redis 使用（cache、session，及后续 mutex/queue）共享一条连接：config 增设顶层可选 Redis 段，`Cache.Redis` / `Session.Redis` 降级为可选覆盖（空则继承顶层，非空覆盖）；Application 懒初始化共享 redis 连接组件，redis 后端从它取连接、不再各自建连。旧配置不改也能跑（向后兼容），未配置 redis 时零开销。本票是 mutex（27）与 queue（28）的装配前置。

**Blocked by:** None (can start immediately)

- [x] config 顶层可选 Redis 段；Cache.Redis/Session.Redis 空则继承顶层、非空覆盖，旧配置行为不变
- [x] Application 共享 redis 连接组件懒初始化；cache/session 的 redis 后端复用同一连接（不重复建连）
- [x] 未配置 redis 且无人使用时零开销
- [x] 单测覆盖：继承/覆盖解析、各处拿到同一 client 实例

## Comments

- 2026-10-29 实施完成（commit 9ba5e07，分支 feat/26-shared-redis 已 ff 合并回 main）。

  实现要点：
  - `config`：顶层可选 `Redis` 段（`Redis` 结构复用，yaml/env 双轨自动生效，`REDIS_ADDR` 等可覆盖）；`WithDefaults` 不伪造值、不回填子段——继承在装配期解析，配置保留原文。
  - `base`：`Application` 新增共享 redis 连接组件（既有 `component[T]` 懒初始化模式）：`Redis()` 返回共享连接（顶层段构建，重复调用同一实例）、`SetRedis()` 替换/还原、`RedisFor(section)` 是继承/覆盖解析的唯一实现——空段拿共享连接实例、非空段按本段值另开专用连接（不同服务器无法共享）。cache 工厂仅在 `driver == "redis"` 时才触碰该组件。
  - `cache`：`NewFromConfigWithRedis(cfg, client)` 接受注入连接（nil 回落到原自建行为），`NewFromConfig` 签名不变、行为等价。
  - `web`：`NewSessionManagerWithRedis(cfg, client)` 同形注入；`NewHandler` 在 `backend == "redis"` 时经 `app.RedisFor` 取连接（file 后端不触发共享组件）。
  - 向后兼容：旧配置（仅 Cache.Redis/Session.Redis 非空）走覆盖分支＝各自按本段建连，行为与之前一致；examples/basic 配置未改、全部测试通过。未引入 mutex/queue 本体（27/28）。

  验收证据（`go build ./...`、`go vet ./...`、`go test ./...` 22 包 1222 测试全绿；golangci-lint 0 issues；miniredis v2.38.0 可用，redis 测试均真实执行、无跳过）：
  - 继承/覆盖解析：`config.TestLoadRedisSection`（顶层段解析、空子段保持空、覆盖子段保留）+ `config.TestRedisEnvOverrides`（REDIS_ADDR/REDIS_DB 环境变量）+ `config.TestRedisDefaults`（不伪造值）
  - 共享组件与实例同一性：`base.TestApplicationRedisSharedComponent`（懒建、重复调用同实例、`RedisFor(空段) == Redis()`、SetRedis 替换/还原）；`cache.TestNewFromConfigWithRedisUsesGivenClient`（后端持有传入的精确 client 实例）；`web.TestNewHandlerSessionRedisSharesApplicationConnection`（handler 装配的 session store 持有 `app.Redis()` 同一实例，且 cache 与 session 记录落在同一服务器）
  - 覆盖走专用连接：`base.TestApplicationCacheRedisOverrideOpensOwnConnection`（cache 覆盖段落 override 服务器、共享连接仍指顶层）+ `web.TestNewHandlerSessionRedisOverrideUsesDedicatedConnection`（session 同理，实例 != 共享）
  - 零开销：`base.TestApplicationRedisZeroOverheadWithoutUser`（memory cache 应用不初始化 redis 组件；首次 redis 请求才构建）
  - 旧行为回归：`web.TestNewSessionManagerWithRedisNilClientKeepsOwnConnection` + 既有 cache/web session redis 测试全部通过

  备注：`go test -race` 本机不可运行（无 cgo/gcc，与 25 号票同况）；懒初始化沿用既有并发安全 `component` 模式，约定 `-race` 环境下由既有并发测试覆盖。无待人工项。
