# 03: base / config 反转（核心收口）

Status: resolved
Blocked by: 01、02（均已 resolved）

**What to build:** 所有权反转的收口大张。config 包只剩 `LoadAs[T](dir)` 与 `Env()`：三台机器（GOYII_ENV、app.yaml + web-<env>.yaml overlay、反射 env 覆盖）作用于任意项目自定义 struct；删除大一统 Config / Load / WithDefaults，此后任何框架包不得 import config 的具体配置定义。`base.New` 改为 `New(opts...)` + `base.WithApp(name, env, debug)` 身份；全部组件访问器急切化、零错误返回，未装配即访问 panic（db / redis 无兜底，其余按 spec 默认清单兜底）；Session 升格一等组件槽（默认 file），Redis 槽直收 go-redis UniversalClient。删除 `app.Config()` 访问器并机械重定向全部内部消费者：migrate / gii 改用 `app.DB()` 不再自行开连接，queue worker 用 `app.Queue()`，web 错误页与 debug 面板的 Debug/Name 改读身份字段，console serve 地址改由装配注入。examples/basic 本票只做最小编译修复（正式样板在 05）。01 / 02 的过渡等值转换与 config 包残留 section 在本票删除。

- [x] config 包仅剩 LoadAs[T] / Env()，模块内无任何包 import 它的具体配置定义
- [x] LoadAs 对任意项目 struct 生效：overlay 与 env 覆盖（含自定义段字段路径）有测试
- [x] base.New(opts...) + WithApp 落地；访问器零错误；未装配 db/redis 访问 panic 有测试；默认清单（log console / cache memory / queue sync / session file / mutex process / mail log driver / i18n 空）行为有测试
- [x] Session 槽（Session / SetSession）与 Redis 槽（SetRedis 收 UniversalClient）可用
- [x] migrate / gii / queue-worker / web / debug / console 不再读取任何配置对象
- [x] `go build ./...` 与 `go test ./...` 全绿；examples/basic 编译可跑

## Comments

- 2026-10-30 实施记录（分支 feat/project-owned-config-03，已 --ff-only 合入 main，commit 91d4975）：
  - **config 收口**：删除 config.go（大一统 Config 与全部 section、Default、WithDefaults）与旧 doc.go；load.go 重写为 `LoadAs[T](dir)`（app.yaml → web-<env>.yaml overlay → 反射 env 覆盖，非 struct 目标报错）+ `Env()`（GOYII_ENV，默认 dev）+ `EnvVarName` 常量；loader 不再填任何默认值。TDD：先写新 load_test.go（红：LoadAs/Env 未定义），再实现转绿。
  - **session 独立组件包**：新增 `session` 包（web 导入 base 导致 base 不能反向导入 web，故数据面必须迁出）：Store 接口 / FileStore / RedisStore（自 web/session_file.go、session_redis.go 迁入）、请求作用域 Session、`Config{Name,Backend,Path,Timeout}` + `New(cfg, WithRedis(client))`（零值=file 后端，redis 缺 client / 未知 backend 构造期报错）、Manager（含 Begin/Finish 生命周期）。`Identity` 移入 session，web 以 `type Identity = session.Identity` 别名再导出（behaviors/examples 等既有消费者零改动）；web/session.go 缩为中间件胶水。
  - **base 重构**：`New(opts...)` + `WithApp(name, env, debug)`（默认 myapp/dev/false）；组件字段全部急切化——New 即构建七项兜底默认（log console info / cache memory / mutex process / mail log mailer / session file / queue sync（带事件分发）/ i18n 空源），兜底构造失败=框架 bug 直接 panic；`DB()`/`Redis()` 未装配 panic（消息指名 SetDB/SetRedis），`SetDB(nil)`/`SetRedis(nil)` 回到未装配态，其余 `SetXxx(nil)` 恢复默认。新增 `ConnectDB(cfg, gormCfg)`（一行装配 db + GORM 查询日志适配器，错误由调用方处理、失败不半装）与 `QueueDispatcher()`（queue.New 的事件路由 option）；`Config()` 访问器删除；component.go 懒 holder 删除。
  - **消费者重定向**：migrate 命令全部经 `app.DB()` 共享连接（dialect 跟随已开连接，不再自开自关；DB_DSN 语义保留在 db.New 开库时生效，DB_DIALECT 仅保留于 create 模板默认且改为运行期惰性解析）；gii openConn 默认 `app.DB()`（--dsn 覆盖时 driver 取自 app 连接）；queue listen 日志改打 `%T` 驱动类型；web 错误页 `app.Debug()`、debug 面板 `app.Debug()/app.Name()` 且 Config 摘要面板整体删除（templ 同步再生成）；console serve 监听地址 `console.WithListen` 注入（未注入默认 :8080——按工单「最小实现，04 完善」，04 的「未注入明确报错」仍留待 04）。
  - **web option 化**：`NewHandler(app, opts...)` + `WithoutCsrf()` / `WithoutConventionFallback()`（默认仍开，对应旧 config 开关语义）；session 中间件消费 `app.Session()`。
  - **过渡体拆除**：logConfigOf、newDefaultQueue 等值转换（新版为纯 sync 默认）、queue.Failing 及其测试、WithDBFunc/WithRedisFunc（base 默认队列不再需要，queue.Config 的 options 回归直接值）、base/RedisFor/redisClientFor/newDefaultRedis（共享/专用连接的抉择移交装配代码）全部删除。`log.NewWithTargets` 复核结论：**保留**——与 cache.NewMemory 等同位的程序化底层构建件，console 测试在用，非过渡体。
  - **examples/basic 最小修复**：main.go 定义项目自有 Config struct（身份段 + db/queue/cache/session/mail/log 参数包直嵌）+ 全急切装配（SetLog/ConnectDB/SetCache/SetMailer/SetQueue(WithDB)/SetSession/SetI18n/SetDefault），console.WithListen 注入；site/views 改读 `App().Name()/Env()`；EmailJob 的 From 改为包内常量（与 mail.from 值一致）。正式样板（config 包化 + Email 段 + env/overlay 演示）留 05。
  - 04 范围提示：04 的「web 显式 option、session 消费 app.Session()、debug 面板去 Config 摘要」因 config 类型删除而随本票必然落地（web/tests 已覆盖默认与 option 两态）；04 剩余事项为 serve 未注入时的明确报错与验收测试补全。
  - 环境备注：本机无 gcc（CGO），`-race` 无法运行（与 01/02 验收条件一致）；并发相关测试按急切语义改写为只读并发（TestApplicationConcurrentReads 等）。
- 2026-10-30 验收证据：`go build ./...` 通过；`go test ./...` 全绿（1429 tests / 36 packages，无 FAIL）；`golangci-lint run ./...` 0 issues；gofmt 无差异。config 9 例（TestLoadAsAppliesDevOverlay / TestLoadAsSelectsEnvironmentFromGOYIIEnv / TestLoadAsEnvOverrides（含 EMAIL_APIKEY、EMAIL_SENDTIMEOUT=90s 自定义段与 duration）/ TestLoadAsWithoutOverlayUsesBaseOnly / TestLoadAsFillsNoDefaults / TestLoadAsEnvOverrideInvalidValueFails / TestLoadAsMissingFilesFail / TestLoadAsRejectsNonStructTarget / TestEnv）；session 10 例（双后端一致性矩阵、并发、New 分发与缺依赖报错）；base 54 例（TestNewAppliesIdentity / TestNewIdentityDefaults / TestNewInstallsFallbackDefaults（七项默认清单）/ TestApplicationDBPanicsWhenNotWired / TestApplicationRedisPanicsWhenNotWired / TestApplicationSessionSlot / TestApplicationConnectDB 与 GORM 查询日志路由等）；web 155 例（TestNewHandlerSessionComesFromAppComponent、TestConventionFallbackDisabled、CSRF 默认开 + WithoutCsrf 全套）。examples/basic 冒烟：`go run . migrate up`（4 条迁移应用）→ `seed` → `serve` 后 `/site/index` 返回 `{"app":"basic","env":"dev"}`（身份字段 + 约定回退）、`/login` 200、未知路径 404 JSON 协商 → `queue run` 处理 3 条种子消息。
