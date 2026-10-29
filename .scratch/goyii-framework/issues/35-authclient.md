# 35: authclient

**Status:** resolved

**What to build:** 第三方登录薄封装：基于 golang.org/x/oauth2 的 provider 接口（BuildAuthURL / Exchange / FetchIdentity）+ GitHub、Google 内置实现，与 web Identity 登录流联动（回调 -> FetchIdentity -> 既有 login 路径）。不进 examples 主线（避免外部凭据依赖）；交付为 docs 教程 + mock OAuth server 单测。

**Blocked by:** None (can start immediately)

- [x] provider 接口 + GitHub/Google 内置（x/oauth2 基座）
- [x] Identity 联动路径：回调 -> FindIdentity -> login，文档化
- [x] docs/ 教程：配置 provider、回调路由、身份映射
- [x] mock OAuth server 单测：授权跳转、code 交换、身份获取

## Comments

**2026-10-29 实施完成（agent）**

- 交付物：`authclient` 包（`provider.go` 接口 + `Identity` 结构、`github.go` / `google.go` 内置实现、`state.go` CSRF 助手、`doc.go` 包文档）；教程 `docs/组件-authclient第三方登录.md`（配置 provider、授权跳转、回调处理、身份映射到登录，含 Mermaid 联动时序图与完整最小代码示例）；docs/README.md 索引更新。新依赖仅 `golang.org/x/oauth2 v0.37.0`（go.mod 单行新增，符合 PRD 选型）。
- 接口签名与共识一致：`Name()` / `BuildAuthURL(state string) string` / `Exchange(ctx, code) (*oauth2.Token, error)` / `FetchIdentity(ctx, token) (*Identity, error)`；`Identity{Provider, ID, Name, Email, Raw}` 为本包结构（非 web.Identity），映射到应用 user 模型 + `web.User.Login` 的路径已文档化。GitHub 差异内部消化（token 走 Authorization 头、Accept json）；Google 固定 `AuthStyleInParams`、默认 scopes openid/email/profile、ID 用 sub。state 助手 `GenerateState`/`ValidState`（常数时间、空值 fail-closed），会话存取留在应用层。
- 测试证据（mock OAuth server = httptest 起 authorize/token/userinfo 三端点）：
  - `TestGitHubFullFlow` / `TestGoogleFullFlow`：授权 URL 构造（client_id/redirect_uri/state/response_type/scope）-> 真实 HTTP 跟随 302 取 code -> code 交换（grant_type/code/redirect_uri/凭据到达）-> FetchIdentity 字段映射；断言 GitHub/Google userinfo 的 token 在 Authorization 头且不在 URL 参数。
  - 错误路径：`TestGitHubExchangeTokenFailure` / `TestGoogleExchangeTokenFailure`（token 端点非 2xx，errors.As 可达 `*oauth2.RetrieveError`）；`TestGitHubFetchIdentityFailures` / `TestGoogleFetchIdentityFailures`（userinfo 401/403 非 2xx、缺 id/sub、JSON 损坏、nil token）；`TestGitHubNameFallsBackToLogin`（name null 回退 login）；`TestGenerateStateIsRandomAndURLSafe` / `TestValidState`。
  - 汇总：`go test ./...` 全量 1449 passed / 30 packages（基线 1428 + 本票 21）；`go vet` 与 `golangci-lint run ./authclient/...` 0 issues；gofmt 干净。
- 提交：17a4b10 claim、62d5e52 feat(authclient)（feat/35-authclient 分支已 --ff-only 合入 main 并删除）。未动 examples 主线、未改规格文档。真机（真实 GitHub/Google 凭据）联调属人工项，框架侧以 mock server 覆盖。
