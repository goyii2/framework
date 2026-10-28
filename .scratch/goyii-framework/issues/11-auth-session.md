# 11: 认证与授权（P0 形态）

**Status:** resolved

**What to build:** User/Identity 认证与会话：Session（file/redis 后端）+ Cookie 管理、Identity 接口、HttpBasic/HttpBearer 认证中间件、会话登录/登出；AccessControl 以角色回调判定授权（P0 形态，P1 由 17 切换 AuthManager 后端，接口不变）。

**Blocked by:** 05, 10

- [x] HttpBasic 与 HttpBearer 分别保护 action，未认证返回 401
- [x] 会话登录后 FromContext 可取当前 Identity；登出后会话失效
- [x] AccessControl 按角色放行/拒绝（403），角色来源为 Identity 回调
- [x] session file/redis 后端行为一致（redis 用 miniredis 或测试容器验证）

## Comments

**落地形态**（main `c40139b`，分支 feat/auth-session 已 ff 合入并删除；全部在 web 包 + config.Session 增一字段）：

- **Identity**（`web/identity.go`）：`GetID() string / GetName() string / GetRoles() []string`——沿用 Yii2 `getId()` 心智；`GetRoles` 是角色回调的数据源。
- **Session 体系**（`web/session.go` + `session_file.go` + `session_redis.go`）：
  - 后端接口 `SessionStore{Load/Save/Destroy}`，ttl 随每次调用传入（生命周期单点归 manager：redis 落 SET EX、file 以 modtime+ttl 判过期即删）；`NewFileSessionStore(dir)` / `NewRedisSessionStore(redis.UniversalClient, prefix)`（单点/cluster/ring 皆可注入）。
  - 请求作用域 `Session`：`Get/GetString/Set/Remove/ID/RegenerateID/Destroy`，mutex 保护。**懒打开**：action 不碰 session 则不触碰后端、不发 cookie（对纯 API 应用零开销，有测试断言后端目录为空）。
  - `SessionManager{Store, CookieName, Timeout, HttpOnly, Secure, SameSite, IdentityFinder}`，`NewSessionManager(cfg)` 按 `config.Session.Backend` 选 redis/file（未知值回落 file）。cookie 属性默认 HttpOnly=true、SameSite=Lax、Path=/、浏览器会话期（无 Max-Age；服务端滑动过期）。
  - `NewHandler` 默认把 `SessionManager.Middleware()` 装入全局链（先于 Csrf 注册 = 更外层）；`Handler.SessionManager()` 为配置位——`IdentityFinder` 在此注入。`Context.Session()/Context.User()` 取请求作用域对象。
  - **API 分叉记录：cookie 在变更瞬间同步发布**（`Set/RegenerateID/Destroy` 内直接写 Set-Cookie 头）而非请求收尾统一发布——首个 body 字节写出后再补的头在真实 net/http server 与 httptest `Result()` 中都会被丢弃（实现期实测踩中）；同请求多次发布按「同名 cookie 替换」保证恰好一个 Set-Cookie（Destroy 覆盖此前发布的 id cookie）。后端 Save/Destroy 仍在请求收尾（defer），失败仅记日志。**销毁后的 Set 是 no-op**（防 zombie 复活，有专测）。
  - 安全：session id = crypto/rand 32B base64url（43 字符）；id 形状校验（[22,128]、base64url 字符集）——伪造 cookie 值永不触达后端，同时挡 file 后端的路径注入。
- **User**（`web/user.go`）：请求作用域认证状态。`Identity()` 懒解析（session 键 `__id` → `mgr.IdentityFinder`，解析失败降级 guest 并清残留键）；`Login(identity)` = **RegenerateID + 写 `__id`**（固定会话防护：登录必换 id、旧记录销毁、预登录数据随迁新 id——实现期红→绿：初版 RegenerateID 先换 id 后加载数据导致旧数据丢失，双后端一致性测试抓出后修复为先 `ensureLoaded` 再换 id）；`Logout()` 销毁 session（记录+cookie+瞬态身份全清）。无 session 的裸 User `Login` 返回错误而非静默丢登录。
- **认证中间件**（`web/auth.go`）：`HttpBasic{Realm, Authenticate func(username, password) (Identity, bool)}` 与 `HttpBearer{Realm, Authenticate func(token) (Identity, bool)}`——校验函数为注入缝；未认证 401 + `WWW-Authenticate`（Basic 带 `realm="…", charset="UTF-8"`；Bearer scheme 大小写不敏感）。认证成功经 `Context.ensureUser().setTransient` 绑**瞬态身份**（action/AccessControl 可见、不落 session）；session 中间件不在链上时 ensureUser 自动补裸 User。框架侧静态 token 表：`StaticBearerTokens(map[token]Identity)`——全表常量时间扫描、无早退（匹配序号不泄时序）；`ConstantTimeEqual(a, b)` 导出（空串不判等）供 Basic 自定义校验用，均有单测。
- **AccessControl**（`web/filter_access.go`）：`AccessRule{Allow, Roles, Verbs, Paths}` 有序规则，**首个适用规则裁决，无匹配默认拒绝（403 统一错误体）**；特殊角色 `?`（仅 guest）/`@`（任意已认证——session 登录与瞬态身份同等生效）；`RoleResolver func(*Context) []string` 为角色来源注入位（默认 `["@"] + Identity.GetRoles()`，guest 为 `["?"]`）——后续切 AuthManager/RBAC 后端时换 resolver 即可，规则与调用点不变。
- **Csrf 选择（10 号票接缝，本票决策）**：**保持双提交 cookie，不改 session 绑定 token**。理由：(a) 双提交无状态——匿名表单页不必被迫创建 session/写后端/发 cookie；(b) Csrf 不依赖 session 组件存在（纯 API 或自定义链下自洽）；(c) wire 形态不变，10 号票 9 项 Csrf 测试零改动全绿。session 中间件默认装在 Csrf 外层，懒打开下两者无行为耦合。
- **config**：`Session.Path`（file 后端目录）新增，默认 `os.TempDir()/goyii-session`；其余（Name/Backend/Timeout/Redis）沿用既有字段。
- **依赖锁定**：`github.com/redis/go-redis/v9 v9.22.0`（v9.23.0 声明 go 1.26 超基线）+ `github.com/alicebob/miniredis/v2 v2.38.0`（纯 Go，无需真实 redis）+ `golang.org/x/sys` 锁 v0.47.0（v0.48.0 同样要 go 1.26）；go 指令保持 **1.25.0**。

**验收证据**（合并后 main `c40139b`，CGO 经 scoop mingw gcc 启用）：

- `go build ./...` / `go vet ./...` / `gofmt -l .` 全部无输出；`go test ./...` 全仓 17 包全绿；`CGO_ENABLED=1 go test -race -count=1 ./web/... ./config/... ./validators/... ./base/...` 四包 ok 无 data race。web 包 107 项顶层测试（10 号票后 74 → 107，本票新增 33 项）+ 6 项 file/redis 双后端子测试。
- **验收 1（Basic/Bearer 各自保护 action、未认证 401）**：`TestHttpBasicRejectsMissingCredentials`（401 + `WWW-Authenticate: Basic realm="api", charset="UTF-8"` + 统一 JSON 错误体）、`...RejectsWrongCredentials`（错密码/未知用户均 401 带 challenge）、`...RejectsMalformedHeader`（坏 base64 / Bearer scheme / 裸 Basic 均 401）、`...AcceptsAndBindsIdentity`（alice:secret-1 → 200，`User().Identity().GetName()=="alice"`、`IsGuest()==false`，且无中间件保护的 `/public/ping` 照常 200）；Bearer 同套：`RejectsMissingToken`（401 + `Bearer realm="api"`）、`RejectsUnknownToken`（未知 token / 空 token / 错 scheme）、`StaticTableAcceptsKnownTokens`（表内两 token 各自解析到对应 identity + 小写 `bearer` scheme 通过）、`CustomValidatorSeam`（自定义校验函数收到原样 token）；`TestConstantTimeEqual`（相等/不等/空串）。
- **验收 2（登录后可取 Identity、登出会话失效）**：`TestUserStartsAsGuest`；`TestLoginBindsIdentityAcrossRequests`（POST /login 发 cookie，后续带 cookie 请求 `c.User().Identity()` 为 alice——即 FromContext/web.Context 路径）；`TestLoginRegeneratesSessionID`（**file+redis 双后端子测试**：预置会话 cookie 与数据 → 登录后 cookie id 必换、旧 id 后端记录销毁、新 id 认证成功、旧 id 客户端变 guest、预登录数据随迁新 id）；`TestLogoutInvalidatesSession`（响应清 cookie Max-Age<0、后端记录销毁、旧 id 再访为 guest）；`TestIdentityDisappearingBetweenRequests`（finder 失效降级 guest 且 `__id` 残键被清）；`TestUserZeroValueRejectsLogin`。
- **验收 3（AccessControl 角色放行/拒绝 403）**：`AllowsHolderOfRequiredRole`（admin → 200）；`DeniesMissingRoleWith403`（editor 访 admin 路由：认证通过但 403，证明是授权拒绝非 401）；`FirstMatchingRuleWins`（deny 规则先匹配则 403，不匹配才走后续 allow）；`VerbScopedRules`（GET 规则放 editor、POST 无规则匹配默认拒 403、admin POST 200）；`GuestRequests`（`?` 放 guest、`@` 拒 guest）；`SessionLoginSatisfiesSpecialRoles`（session 登录满足 `@`、被 `?`-only 路由拒绝）；`CustomRoleResolver`（resolver 返回的角色覆盖 Identity 自带角色——缝的行为验证；未知 token 401 先于授权）。
- **验收 4（file/redis 行为一致）**：`TestSessionStoreBehaviorConsistency`（同一场景矩阵跑双后端：Save/Load 往返（数值经 JSON 回为 float64，按此断言）、unknown id `ok=false`、Save 整覆写不合并、Destroy 幂等、超时失效——file 用 Chtimes 回拨 modtime / redis 用 miniredis FastForward）；请求级行为（cookie 签发/持久化/销毁/轮换/过期/未知 id）file 后端全覆盖，redis 后端经登录端到端子测试覆盖。并发：`TestSessionStoreConcurrentAccess`（16 worker×25 op 双后端）与 `TestSessionConcurrentRequests`（16 并发请求全管道）均 -race 通过。
- 其他 session 行为测试：未用不发 cookie 且后端零记录、跨请求持久化 + 二次写合并语义、Remove、Destroy 清 cookie+后端、Regenerate 轮换 cookie 且旧 id 文件删除、未知 cookie id 全新会话且不落盘、file 后端超时后失效、销毁后再写不复活。

**遗留**：AuthManager/RBAC 后端切换（17 号票，`RoleResolver` 缝已留）；Flash 消息（PRD web 条目提及，非本票范围）；web.Context 的 `CsrfToken` 仍为双提交实现（本票决策保持，如需 session 化再评估）；golangci-lint 本机全包 panic 环境问题沿袭 10 号票记录（vet+gofmt 已覆盖等价检查）；go 基线升 1.26 后可解锁 go-redis v9.23+/x/sys v0.48+。
