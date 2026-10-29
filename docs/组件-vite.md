# 组件：Vite 前端构建

`vite` 包把 Vite 构建的前端接进 templ 页面：生产按资源清单（manifest，Vite 5+ 默认 `dist/.vite/manifest.json`）产出带 hash 的 script/link 标签；开发直连 Vite dev server 并注入 `@vite/client`；一个仅开发期挂载的反向代理把静态路径转发到 dev server（WebSocket 透传，HMR 必需）。

刻意不做的：AssetManager、通用资源注册表——清单即全部契约，构建产物由 `go:embed` 或静态文件服务直接分发，本包只画标签。

## 接线

```go
v := &vite.Vite{
    Manifest:  "web/dist/.vite/manifest.json", // vite build 的产物清单
    Base:      "/",                             // 与 vite.config 的 base 一致，默认 "/"
    Dev:       false,                           // 显式开发开关（清单缺失也会自动进 dev）
    DevServer: "http://localhost:5173",         // 默认值
}

handler := web.NewHandler(app)
if v.IsDev() {
    handler.Use(v.DevProxy("/assets")) // 仅开发期挂载，转发静态前缀到 dev server
}
```

页面头部引用入口（Entry 返回单个 templ 组件，含入口的全部依赖 CSS）：

```go
head, err := v.Entry("src/main.ts") // 名字即清单键：入口的源码路径
if err != nil {
    return err // 显式报错：入口名写错或构建产物损坏在首屏渲染即暴露
}
page := view.Page{Title: "...", Head: head, Content: body}
```

## 模式判定（首次使用时采样一次，进程内固定）

| 条件 | 模式 | Entry 产出 |
|---|---|---|
| `Dev: true` | dev | `@vite/client` + `http://localhost:5173/<entry>`，两个 module script |
| 清单文件不存在（含 `Manifest` 为空） | dev | 同上——还没跑 `vite build` 时直接得到 dev 地址而非报错；构建后重启应用即翻到 hash 地址 |
| 清单存在且可解析 | build | 每个依赖 CSS 一个 `<link rel="stylesheet">`（依赖分块在前、入口自身在后、去重），随后入口 `<script type="module" src="<Base>+hash 文件">` |

dev 模式不输出 CSS link：dev server 经 JS 模块图注入样式，link 会重复。`IsDev()` 与 `Entry()` 共用同一次采样，二者永不打架；生产装配用 `IsDev()` 守住 `DevProxy`，绝不把静态路径转发到 localhost。

## CSS 递归收集

清单条目形如 `入口名 -> {file, src, isEntry, css[], imports[]}`。CSS 收集沿 `imports` 链递归（被引分块先于引用者，对齐模块执行顺序），同一文件只出一次；环与缺失的 import 键都有保护。

## DevProxy

`v.DevProxy(prefixes...)` 返回标准 net/http middleware（可直接赋给 `web.Middleware`，进 `handler.Use` / `web.Chain`）：

- 前缀按路径段匹配：`/assets` 覆盖 `/assets` 与 `/assets/main.js`，不误伤 `/assetsfoo`；不传前缀（或传 `/`）转发一切；
- WebSocket 升级透传：标准库 `httputil.ReverseProxy` 会在出站请求上补回 `Connection/Upgrade` 头，遇 101 双向劫持直通——HMR 的 ws 连接因此可用；`FlushInterval` 负值保证热更新不被代理缓冲拖延；
- dev server 不可达时答 502，正文带 `vite: dev server ... unreachable` 提示。

dev server 里页面由 Go 应用渲染、模块从 5173 加载，无需 CORS 配置。

## Vite 侧约定

```js
// vite.config.js
export default {
  base: "/",                    // 与 vite.Vite.Base 一致
  build: { manifest: true },    // 产出 dist/.vite/manifest.json
}
```

入口名即构建输入路径（`src/main.ts`）；开发流程 `npm run dev` 起服务后照常访问 Go 应用，改前端代码热更新直达。

## 错误（哨兵，可 `errors.Is` 判别）

| 错误 | 含义 |
|---|---|
| `vite.ErrInvalidManifest` | 清单存在但不可用：读失败、JSON 损坏、结构不对、分块缺 `file`、空清单（文件不存在**不是**错误——那是 dev 回退） |
| `vite.ErrEntryNotFound` | 名字不是清单键（多因入口路径写错，如 `.ts` 写成 `.js`） |

错误信息带肇事路径/名字；`Entry` 显式返回错误而非渲染空组件，问题在开发期第一屏就暴露。

## 相关文档

- [组件：view 与 templ](组件-view与templ.md)（`Page.Head` 槽位、Layout 组合）
- [组件：web 路由与中间件](组件-web路由与中间件.md)（`handler.Use`、middleware 链）
