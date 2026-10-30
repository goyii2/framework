# 04: web / console 显式 option 化

Status: resolved
Blocked by: 03

**What to build:** web 与 console 的行为开关全部显式化：web.NewHandler 的 CSRF 开关与约定回退开关改为显式 option，session 改为消费 `app.Session()`（不再内部装配）；console serve 的监听地址由装配方注入（未注入时给出明确错误）；debug 面板删除 Config 摘要（保留请求 / 组件状态摘要）。

- [x] web.NewHandler 不再从任何配置读值；CSRF / 约定回退的默认值与显式 option 行为有测试
- [x] session 由 app.Session() 提供；替换组件经 SetSession 生效
- [x] serve 命令监听地址来自装配注入，未注入时报错信息明确
- [x] debug 面板无 Config 摘要，其余功能不回归
- [x] `go build ./...` 与 `go test ./...` 全绿；examples 同步最小编译修复

## Comments

- 2026-10-30 实施记录（分支 feat/project-owned-config-04，已 --ff-only 合入 main，commit d8fa356；claim 提交 3d300f1）：
  - **本票实际新代码仅 serve 监听语义收尾**，其余验收标准核实为 03 已完整落地（提交 91d4975），未重复实现：
    - 标准 1（由 03 落地）：`web.NewHandler(app, opts...)` 不读任何配置（web 包无 config import）；CSRF 默认开全套行为测试在 `web/filter_csrf_test.go`（TestCsrfSafeRequestIssuesCookieAndExposesToken / TestCsrfRejectsUnsafeRequestWithoutToken / TestCsrfRejectsUnsafeRequestWithCookieOnly / TestCsrfRejectsMismatchedToken / TestCsrfAcceptsMatchingHeaderToken / TestCsrfAcceptsMatchingFormFieldToken / TestCsrfQueryTokenIsNotAccepted），显式 option 关闭为 TestCsrfDisabledGloballyByConfiguration（web.WithoutCsrf）与 TestCsrfDisabledFlagSkipsHandMountedFilter；约定回退默认开覆盖于 `web/router_test.go` 各 convention 用例（如 TestConventionFallbackModuleScopeAndIsolation），显式关闭为 TestConventionFallbackDisabled（web.WithoutConventionFallback，含运行期 SetConventionFallback 重开）。
    - 标准 2（由 03 落地）：`NewHandler` 装配 `session: app.Session()` 并装 `sessionMiddleware`；「替换组件经 SetSession 生效」验收测试在 `web/handler_redis_test.go` TestNewHandlerSessionComesFromAppComponent（app.SetSession(redis 后端) → `h.SessionManager() == app.Session()` 指针同一 + 经替换后端写记录落到对应 miniredis），端到端请求链路由 `web/session_test.go` sessionApp 系列覆盖（SetSession 的 file/redis 管理器经完整请求管道读写、销毁、并发）。
    - 标准 4（由 03 落地）：debug 包 grep 无任何 Config 摘要残留（.go/.templ），debug 全部测试随全量绿。
  - **本票新落地（标准 3）**：`console.New` 删除 `listenAddr: ":8080"` 预填；`serve` 在未注入 WithListen（含注入空串）时返回 setup 错误，与缺 handler 报错同型且含修复指引：`console: serve has no listen address; construct the console with console.WithListen (typically the project's web.listen value)`；`WithListen` doc 同步（删除 Default ":8080" 表述）。新增 TestServeWithoutListenAddressReportsSetupError（断言报错含 "no listen address" 与 "console.WithListen"）。
  - 附带最小修复：`console/doc.go` 包示例原引用已删除的 `goyii.LoadConfig` 与 03 前的 `base.New(config)` 签名，改为现行 API 并补 WithListen 注入（该示例正描述本票定稿的 serve 契约）。
  - examples/basic 无需改动：03 已注入 `console.WithListen(cfg.Web.Listen)`（examples/basic/main.go），随 `go build ./...` 编译通过。
- 2026-10-30 验收证据：`go build ./...` 通过；`go test ./... -count=1` 全绿（exit 0，30 个包 ok，无 FAIL；verbose 顶层 PASS 786 例）；`go vet ./console/` 干净；`golangci-lint run ./...` 0 issues；gofmt 无差异。console 新增用例 TestServeWithoutListenAddressReportsSetupError 通过；serve 既有用例（TestServeServesRequestsThroughSharedApplication / TestServeGracefulShutdownWaitsForInFlightRequest / TestServeShutdownTimeoutFailsTheCommand / TestServeWithoutHandlerReportsSetupError）全部通过。环境备注：本机无 gcc（CGO），`-race` 无法运行（与 01–03 验收条件一致）。
