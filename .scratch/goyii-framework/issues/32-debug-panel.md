# 32: debug 面板

Status: resolved

**What to build:** 开发期请求内省工具（debug 包）：App.Debug=true 时挂采集 middleware，按请求聚合七面板——请求/路由匹配、日志、SQL、时间线、Session/User、配置摘要（脱敏）、邮件；每请求一个 JSON 落 `runtime/debug/`（默认保留 200 个、超量淘汰）。内嵌 `/debug` 路由提供请求历史与详情页，并对 200 的 text/html 响应注入页底 toolbar（注入层在 PageCache 内层，复用 29 的 body 捕获 writer）。默认仅本机 IP 可访问，可配 CIDR 列表。

**Blocked by:** 29

- [x] 采集 middleware（仅 Debug=true）：七面板数据聚合；body 捕获复用 29 的 writer
- [x] 日志面板：log 包新增 per-request 内存 Target；SQL 面板从 db 类别记录提取语句/耗时/行数
- [x] 时间线复用 Profiler；配置摘要脱敏（DSN、密码等）
- [x] 每请求 JSON 落盘 + 保留上限淘汰（默认 200 可配）
- [x] /debug 历史 + 详情页；toolbar 仅对 200 text/html 注入、面板页自身不注入
- [x] 访问控制：默认 127.0.0.1/::1，可配 CIDR 列表
- [x] 单测：聚合、脱敏、淘汰、注入条件

## Comments

### 实现要点（2026-10-29，提交 c623f44）

- **独立 `debug` 包**（不进 web/base）：`debug.New(app)` -> `panel.Mount(handler)`（对现有组装点侵入最小：一次 `h.Use`；注释写明须在 `NewHandler` 后、其他全局中间件前挂载）。`Mount` 在 `app.Debug=false` 时为 no-op，生产可无条件调用；挂载时把日志根级别降为 debug 并经既有 `Logger.WithTarget` 机制绑定按请求路由的分发 Target（本票为 log 包新增 `log.MemoryTarget`（有界环形内存 Target）与 `Logger.WithLevel`）。
- **日志归属（无侵入埋点）**：请求的全部记录都由服务该请求的 goroutine 同步写出，分发 Target 按发出 goroutine 归属到当次请求的 MemoryTarget——并发请求互不串线，后台 goroutine 的记录不进面板。`TestConcurrentRequestsKeepTheirOwnLogs`（真实 server、8 并发、强制重叠，每条记录恰好归属自身请求）。
- **toolbar 注入与 PageCache 层级**：采集层位于全局链最内（其他全局中间件之外）、PageCache 等 Filter（控制器/模块级）之内——即 **PageCache 在注入层内层**。字面读法「注入层在 PageCache 内层」会把 toolbar 烧进缓存页（之后原样回放给所有访客）且回放请求无新记录，与验收项「仅对 200 text/html 响应注入」冲突，故取相反方向并在 `Mount` 注释与本条记录该解读。`TestPageCacheBelowCaptureStaysCleanAndReplayGetsToolbar`：缓存条目无 toolbar、两次请求各自拿到当次 id 的新 toolbar。
- **响应保持与直通**：动作经 Context writer 写响应，故经新增 web 接缝 `Context.SwapWriter`（PageCache.render 同型换写）挂 `web.CaptureWriter`（29 导出）+ holdWriter（持有 headers/body，Flush 即破持直通）。错误路径（`Context.Err()`）不提交持有字节，交统一错误渲染器作答；panic 路径记录后 re-panic，恢复的 500 页照常到达客户端（`TestPanickingActionGetsErrorPageAndRecord`）。restore 走 defer——内层 panic 时 Context writer 也必须还原（自审发现并修复的缺陷）。
- **七面板**：请求/路由（method/path/remote + `r.Pattern` 匹配 pattern 与通配参数 + 最终 status/耗时/错误）、日志（MemoryTarget 记录）、SQL（db 类别 `sql/elapsed_ms/rows/err` 属性）、时间线（profile begin/end 按 token 配对）、Session/User（`Session.All()` 快照 + 登录态，另为 web 新增 `Session.All`）、配置摘要（DSN/redis/SMTP 密码脱敏）、邮件（mail 类别记录）。
- **落盘与访问**：每请求一个 JSON 落 `runtime/debug/`（`Dir` 可配），文件名 = UTC 时间戳 + 序列（即详情页 `?id=`），默认保留 200（`KeepFiles` 可配）超量淘汰最旧；`/debug` 历史 + `/debug/view` 详情（templ，风格对齐 examples）；面板页自身不捕获不注入、GET only；访问控制默认 127.0.0.1/::1，`AllowCIDRs` 配置后替换默认（不再隐含放行 loopback），仅约束面板页不影响业务路由。

### 验收证据（2026-10-29，提交 c623f44）

- 注入条件：`TestToolbarInjectedIntoHtmlResponses`（200 text/html 注入且位于最后 `</body>` 前、Content-Type 保留）、`TestToolbarNotInjectedIntoNonHtmlResponses`（JSON 原样）、`TestToolbarNotInjectedIntoErrorResponses`（404 不注入且记录 Request.Error/Status）、`TestToolbarSkipsHeadRequests`、`TestStreamingResponsePassesThroughWithoutToolbar`（Flush 直通、不缓冲不注入）、`TestPanelPagesServeWithoutCapture`（历史/详情页无 toolbar、浏览不产生记录、坏 id 404）、`TestPanelPagesGetOnly`（405+Allow）、单元 `TestToolbarEligibility`/`TestInjectBeforeBodyEnd`（大小写、无 `</body>` 追加、`</body>` 后有尾注）。
- 聚合：`TestEntryCapturesRequestAndAllPanels`（路由 `GET /post/{id}`+params、日志 attrs、SQL 语句/行数、时间线、Session 值、guest、邮件、DSN 脱敏端到端）；`TestCollectLogPanelsExtractsSQL`/`PairsTimeline`/`CollectsMail`；`TestLogCaptureAttributesRecordsToTheirRequest`；`TestConcurrentRequestsKeepTheirOwnLogs`。
- 脱敏：`TestMaskDSN`（URL/MySQL/KV 三形态 11 例）、`TestMaskDSNNeverLeavesPassword`、`TestSummarizeConfigMasksSecrets`（空密码保持空、非密钥不损失）。
- 淘汰：`TestStoreWritesEntryWithID`、`TestStoreEvictsOldestBeyondKeep`（keep=3 存 5 剩最新 3）、`TestLoadRejectsUnsafeAndUnknownIDs`（`../`、路径分隔、不存在均 ErrNotExist）、`TestHistoryNewestFirst`、`TestHistoryEmptyDirectory`。
- 访问控制：`TestPanelAccessControlLoopbackByDefault`（127.0.0.1/::1 放行、10.0.0.1 403）、`TestPanelAccessControl`（配置网段放行、配置后 loopback 403、业务路由不受限）、`TestMountWithoutDebugDoesNothing`（/debug 404、不注入、无记录）。
- web/log 接缝：`TestContextErrRecordsAndReadsError`、`TestSwapWriterRedirectsActionWritesAndRestores`、`TestSessionAllReturnsIndependentCopy`、log 侧 `TestMemoryTarget*`、`TestWithLevelLowersDispatchFloor`。
- 测试摘要：`go test ./... -count=1` 25 包全绿（debug 包 28 用例/48 例含子测试，`-count=3` 稳定）；`go vet ./...`、`golangci-lint run ./...` 0 issues；gofmt 无差异（本机无 gcc，race 检测不可用——并发归属以真实 server 并发用例覆盖）。文档：新增 `docs/组件-debug面板.md`，`docs/组件-log.md` 增 MemoryTarget/WithLevel。
- 36 号集成票接缝：examples 挂载方式 = `debug.New(app)` + `panel.Mount(handler)`（`NewHandler` 后、其他 `Use` 前），见 docs/组件-debug面板.md。
