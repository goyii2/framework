# 02: examples 装配接线与可见触点

Status: resolved

**What to build:** basic 与 advanced 两个示例在装配期接上主机信息：`main.go` 里 `url.SetDefault(handler.URLs())` 之后按项目配置调用 `SetHostInfo`（值来源遵循 ADR 0005 的「项目自有配置 + 代码装配」——示例自定取值方式，如环境变量或配置 overlay，不硬编码魔法值）。每个示例落一处**可见触点**：用 `url.ToAbsolute` 渲染完整网址进入页面输出（例如文章页的分享链接或 canonical 链接），让新能力不是只接线不使用。示例行为不回归：既有页面、重定向、表单 action 全部保持相对 URL 原样（只有触点用绝对生成）。

**Blocked by:** 01

- [x] basic 装配期 `SetHostInfo`（值来自项目配置，非硬编码）
- [x] advanced 装配期 `SetHostInfo`（同上）
- [x] 每示例一处可见触点：页面渲染输出含 `scheme://host/path` 形态、由 `url.ToAbsolute` 生成的链接
- [x] 既有页面/重定向/表单 action 保持相对 URL 不变（触点除外）
- [x] 两个示例构建通过，全量测试绿；触点渲染可自动化验证的部分自动化（如视图函数直调断言），纯目视项标注「待人工」

## Comments

- 2026-10-31 实施（commit da0dff6，feat/absolute-url-02 已 --ff-only 合回 main 并删分支）：两示例 `Config.Web` 增 `HostInfo`（yaml key `hostInfo`，env 覆盖 `WEB_HOSTINFO`），app.yaml 落基值 `http://127.0.0.1:8090`；advanced 的 web-prod.yaml 以 `${ADVANCED_HOST_INFO:-...}` 插值部署主机（与 listen 同形态）。main.go 均在 `url.SetDefault(handler.URLs())` 后一行调用 `handler.URLs().SetHostInfo(cfg.Web.HostInfo)`——非法值由 SetHostInfo 的 boot 期 fail-fast 兜住。可见触点：basic 文章详情页（post/show）与 advanced frontend 文章页（post/view）各渲染一行 Permalink，值来自 `url.ToAbsolute(命名路由, id)`；basic 经 i18n 标签（zh-CN「固定链接」）。其余链接/重定向/表单 action 零改动。
- 验收证据：`go build ./examples/basic ./examples/advanced` 通过；`go test -count=1 ./examples/basic/config/... ./examples/advanced/config/...` 10 例全过（含新增 Web.HostInfo 基值断言、advanced prod overlay 插值默认与环境变量拼接断言、WEB_HOSTINFO 屏蔽）；`go test -count=1 ./examples/basic/controllers/... ./examples/advanced/frontend/...` 20 例全过（新增 TestPostShowRendersPermalink / TestFrontendPostViewRendersPermalink：真 HTTP 渲染断言 `href="https://…/posts/{id}"` 存在、permalink 全页恰好出现 2 次（href+文本，无其他绝对链接泄漏）、相对链接 `href="/posts"` 保持）；全量 `CGO_ENABLED=1 go test -race -count=1 ./...` 37 包全 ok；`golangci-lint run ./examples/basic/... ./examples/advanced/...` 0 issues；gofmt clean。
- 待人工：浏览器目视确认两处 Permalink 行展示效果（含 zh-CN 下「固定链接」文案）——自动化已断言 href 与文本渲染，纯外观留人工。
- 双轴自审（编码规范＋规格符合）：注释自含、无工单/ADR 编号引用；值全部走项目配置（yaml/overlay/env），行为在 main.go 装配代码，符合 ADR 0005 形态；`To`/`URL`/既有路由与页面行为未动，仅增不改。
