# 05: examples 迁移 builder + advanced 双域名

Status: resolved

**What to build:** 两个示例迁到 builder 形态并演示双域名。basic：文章页 Permalink 触点从 `url.ToAbsolute` 迁到 `url.Build(...).Absolute()`（单域名，默认 host）。advanced：`Config.Web` 增 `BackendHostInfo`（yaml `backendHostInfo`，env `WEB_BACKEND_HOSTINFO` 覆盖，prod overlay 插值——形态与既有 `HostInfo` 一致）；backend 分区的 Permalink 触点用 `url.Build(...).Host(cfg.Web.BackendHostInfo).Absolute()` 演示跨分区显式域（后台在独立域名的部署形态），frontend 触点走默认 `.Absolute()`。装配代码把 backend host 传进 backend 分区（装配函数注入，不引全局）。既有相对链接/重定向/表单 action 保持不变。

**Blocked by:** 04

- [x] basic 触点迁移 `url.Build(...).Absolute()`，行为与断言不回退
- [x] advanced `BackendHostInfo` 配置全链路（yaml 基值 + env 覆盖 + prod overlay 插值）
- [x] backend 触点 `.Host(backendHost).Absolute()` 生效：渲染输出为 backend 域名的完整网址
- [x] 既有绝对链接断言测试迁移 builder 形态并覆盖 backend 显式域触点
- [x] 既有相对链接/重定向/表单 action 零改动；两示例构建通过，全量测试绿

## Comments

- 2026-10-31 实施（commit e0d6821，feat/absolute-url-05 已 --ff-only 合回 main 并删分支）：advanced `Config.Web` 增 `BackendHostInfo`（yaml `backendHostInfo`）；app.yaml 落基值 `http://127.0.0.1:8090`（单进程单端口，两分区同 loopback——基值不撒谎，独立域名经环境注入）；web-prod.yaml 增 `${ADVANCED_BACKEND_HOST_INFO:-http://127.0.0.1:8090}`（与既有 hostInfo 同形）。装配注入链：main.go `backend.Register(handler, app, auth, cfg.Web.BackendHostInfo)` -> `controllers.PostController{BackendHost: backendHost}`（无全局）。backend 触点落在 post 编辑页（backend 分区最接近"详情页"的页面）：`url.Build("backend/post/edit", url.P{"id": ...}).Host(pc.BackendHost).Absolute()` 渲染 Permalink 行（compose 页无行不渲染；非法 host 降级 `"#"` 不 panic，空 host 亦 `"#"`——不猜域名）；frontend 触点保持默认 `.Absolute()` 未动；backend 既有列表/守卫/表单/重定向零改动，表单 action 与取消链接保持相对。`renderPostForm`/`replayPostForm` 由包级函数转为 `PostController` 方法以触达注入的 host（调用点机械更新）。
- env 名与工单正文的偏差说明：工单正文写 env `WEB_BACKEND_HOSTINFO`，但整字段覆盖机器的命名规则是「yaml key 大写拼接」（config/load.go walkOverrides：`web.backendHostInfo` -> `WEB_BACKENDHOSTINFO`），与既有 `hostInfo` -> `WEB_HOSTINFO`（01-04 号既成事实）同一条规则；工单自身的约束「形态与既有 HostInfo 一致」在此规则下只能得出 `WEB_BACKENDHOSTINFO`（若要 `WEB_BACKEND_HOSTINFO` 须改 yaml key 为 `backend_host_info`，与工单指定的 yaml key 矛盾，或改框架 loader 按驼峰切分——会连带破坏 `WEB_HOSTINFO` 既有名）。故按「形态一致」条款实现为 `WEB_BACKENDHOSTINFO`，未改任何规格文档；06 号修订文档时 env 名请以此为准。
- 验收证据：`go test -count=1 ./examples/advanced/config/` 8 例全过（新增 TestBackendHostInfoEnvironmentOverride：整字段 `WEB_BACKENDHOSTINFO` 压过 prod overlay 的 ADVANCED_BACKEND_HOST_INFO 插值与基值；TestLoadProdOverlayInterpolatesDefaults/InterpolatesEnvironment 增 backendHostInfo 默认值/插值断言；TestLoadFillsIdentityAndComponentSections 增基值断言与 WEB_BACKENDHOSTINFO 屏蔽）；`go test -count=1 ./examples/advanced/backend/` 全过（新增 TestBackendPostEditRendersPermalink：manager hostInfo 设 `https://frontend.example.com`、Register 注入 `https://backend.example.com`，断言编辑页 Permalink 为 `https://backend.example.com/backend/posts/{id}/edit`、permalink 全页恰好 2 次（href+文本）、页面零处出现 frontend 域名、`action="/backend/posts/{id}"` 与 `href="/backend/posts"` 保持相对、compose 页无 Permalink 行）；既有 TestBackendPostManagement / TestBackendGuardThreeStates / frontend TestFrontendPostViewRendersPermalink / basic TestPostShowRendersPermalink 未改动全绿。`go build ./examples/basic ./examples/advanced` 通过；`golangci-lint run ./examples/basic/... ./examples/advanced/...` 0 issues。`scripts/p0-check.sh` 全绿：gofmt clean、go vet clean、`CGO_ENABLED=1 go test -race -count=1 ./...` 全部 ok、basic 冒烟 31/31（"P0 checks green."）。
- 双轴自审（编码规范＋规格符合）：规范轴——注释自含、无工单/ADR 编号引用；配置值全部走 yaml/overlay/env（dev 基值同 loopback 是单端口事实而非魔法值）；templ 以钉版 v0.3.1100 再生成（从 examples/advanced 目录运行，FileName 路径前缀与入库约定一致；Windows 下 templ 误把 frontend 生成物刷成 CRLF 字面量，已还原为 main 版本不入库）。规格轴——五条验收逐条满足；`SetHostInfo`/`To`/`Build`/`Host`/`Absolute` 框架面零改动；超出工单正文的最小必要改动一处：`docs/示例-advanced模板.md` 分区表中 `backend.Register(h, app, auth)` 签名同步为新签名（防止使用文档与实现矛盾，属 02/03 号先例的 drift 修正，ADR/组件文档修订仍归 06 号）。
- 待人工：浏览器目视确认 backend 编辑页 Permalink 行展示效果（自动化已断言 href 与文本渲染，纯外观留人工）。

