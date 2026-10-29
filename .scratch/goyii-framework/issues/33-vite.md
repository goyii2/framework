# 33: Vite 集成

**Status:** resolved

**What to build:** vite 包：构建期 manifest.json 解析 + `vite.Entry(name)` 产出带 hash 的 script/link templ 组件（含依赖 CSS）；开发模式（配置开关或 manifest 缺失）直连 localhost:5173 并注入 `@vite/client`；ReverseProxy middleware 转发静态前缀（含 WebSocket 透传，供 HMR）。深度止于此：不做 AssetManager、不做通用资源注册表。

**Blocked by:** None (can start immediately)

- [x] manifest.json 解析：entry 名 -> 带 hash URL + 依赖 CSS 清单
- [x] vite.Entry(name) 渲染 script/link（含依赖 CSS）
- [x] dev 模式回退：直连 5173 + @vite/client 注入
- [x] ReverseProxy middleware：静态前缀转发 + ws 透传（HMR）
- [x] 单测：解析、hash URL、CSS 依赖、dev 回退

## Comments

### 实现要点（2026-10-29，提交 3f1ff87）

- **独立 `vite` 包**，仅依赖 templ + 标准库（`vite/doc.go`/`vite.go`/`manifest.go`/`proxy.go`/`entry.templ`）。结构体配置 + 指针使用，模式在首次使用时采样一次、进程内固定（`IsDev()` 与 `Entry()` 共用同一次采样，永不打架）：`Dev` 开 -> dev（不读 manifest）；manifest 文件缺失（或 `Manifest` 为空）-> dev；否则 build 模式。零值 `Vite{}` 可用（= 默认 dev server）。
- **manifest 解析**按 Vite 5+ 官方 `.vite/manifest.json`：键 -> `{file, src, isEntry, css[], imports[]}`（未知字段忽略）；拒不可用形态（坏 JSON、结构错、分块缺 file、空清单）。CSS 沿 imports 链递归收集：被依赖分块在前（对齐模块执行顺序）、逐文件去重、环与悬空 import 键有保护。
- **`Entry(name)` 显式报错**（非 fail-soft）：哨兵 `ErrInvalidManifest` / `ErrEntryNotFound`，`%w` 包装带肇事路径/名字。build：`<link rel="stylesheet">`（每个依赖 CSS）+ `<script type="module" src="Base+hash 文件">`；Base 归一化（""/"/" -> 站根，补首尾斜杠）。dev：先 `@vite/client` 再 `http://localhost:5173/<entry>` 两个 module script，**不输出 CSS link**（dev 由 JS 模块图注入样式，注释写明于 `Entry` 文档与 templ 组件）。
- **`DevProxy(prefixes...)`**：net/http middleware 形态（可直接赋 `web.Middleware` 进 `handler.Use`/`web.Chain`）；前缀按路径段匹配（`/assets` 不误伤 `/assetsfoo`），无前缀（或 `/`）转发一切；`httputil.ReverseProxy`（Rewrite+SetXForwarded、`FlushInterval:-1` 不缓冲）指向 DevServer（默认 localhost:5173，host:port 自动补 `http://`）；dev server 不可达答 502 + `vite: dev server ... unreachable` 提示。docs 建议装配 `if v.IsDev() { handler.Use(v.DevProxy("/assets")) }`——生产绝不转发静态路径到 localhost。
- **WebSocket 透传的实现取舍**：工单注记「标准库 ReverseProxy 需显式处理 Upgrade」——经核对 Go 1.26 标准库源码，该要求已由库内置完成（`removeHopByHopHeaders` 后若请求带 upgrade 即在出站请求补回 `Connection/Upgrade`，101 时 `handleUpgradeResponse` 双向劫持直通），在 `Rewrite` 里再做一遍头部手术是无害冗余，故不重复实现，改为以**真实 RFC 6455 握手端到端测试**证明透传成立（见下）。`DevProxy` 注释记录此判定。
- **深度边界**：无 AssetManager、无通用资源注册表、无 modulepreload、不动 examples（web/ 前端工程归 36 号票）；交付 = vite 包 + 单测 + `docs/组件-vite.md` 一页（接线方式即交付语义）。`docs/README.md` 索引补 vite 行，顺带补上 32 号票漏挂的 debug 面板行（一行事实修正）。

### 验收证据（2026-10-29，提交 3f1ff87）

- 解析：`TestParseManifest`（双 entry + 共享分块，file/src/isEntry/imports/css 全字段）、`TestParseManifestRejectsBrokenInput`（坏 JSON/错结构/缺 file/空清单四形态）。
- hash URL + Entry 渲染片段断言：`TestEntryBuildModeRendersHashedTags`（整段 HTML 精确比对：依赖 CSS link 在前 + `type="module"` hash script）；`TestEntryCollectsCSSRecursively`（二级 imports 链：shared/theme/admin 三 CSS 各恰好一次且依赖在前）。
- base 前缀：`TestBasePrefix`（""/"/"/"/my-app"/"/my-app/"/"my-app" 五形态归一）。
- dev 回退：`TestDevFallbackWhenManifestMissing`（manifest 缺失 -> IsDev 且整段 HTML == `@vite/client` + entry 直连 5173、无 `<link`）、`TestDevSwitchIgnoresManifest`（开关优先于已存在的 manifest）、`TestDevServerConfig`（自定义 origin 尾斜杠归一）、`TestZeroViteIsDevAgainstDefaultServer`、`TestModeIsStickyOnceResolved`（采样后构建出现仍 dev，重启才翻转）。
- 错误哨兵：`TestEntryNotFound`（errors.Is + 消息含名字）、`TestEntryInvalidManifest`（IsDev=false 且 errors.Is）。
- 代理：`TestDevProxyForwardsUnderPrefix`（前缀内转发：path/query/X-Forwarded-Proto 到达后端；前缀外走内层 418）、`TestDevProxyPrefixMatching`（段匹配含尾斜杠/空格归一）、`TestDevProxyNoPrefixForwardsEverything`、`TestDevProxyHostOnlyConfigGetsScheme`（补 scheme/坏值回退默认）、`TestDevProxyUnreachableReturns502`（正文含 vite 提示）。
- ws 透传：`TestDevProxyPassesWebSocketUpgrade`——裸 TCP 真实 RFC 6455 握手过代理：后端确实收到透传的 `Upgrade: websocket` + `Connection: Upgrade` + 原样 `Sec-WebSocket-Key`；客户端收到 101、`Sec-WebSocket-Accept` 与规范样例摘要一致（`s3pPLMBiTxaQ9kYGzzhZRbK+xOo=`，证明头部往返未被改动），其后端写入的 `hi!` 文本帧经劫持连接原样到达。零第三方依赖（sha1+base64 手搓摘要）。
- 测试摘要：`go test ./... -count=1` 27 包全绿（vite 包 17 用例，`-count=3` 稳定）；`go vet ./...`、`golangci-lint run ./vite/...` 0 issues；gofmt 无差异（本机无 gcc，`-race` 不可用——ws 透传与并发以真实 socket 端到端用例覆盖）。
- 36 号集成票接缝：examples 挂载方式 = `&vite.Vite{Manifest: "web/dist/.vite/manifest.json"}` + `IsDev()` 守卫 `DevProxy("/assets")` + layout Head 槽放 `v.Entry("src/main.ts")`，见 docs/组件-vite.md。
