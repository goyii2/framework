# 33: Vite 集成

**Status:** ready-for-agent

**What to build:** vite 包：构建期 manifest.json 解析 + `vite.Entry(name)` 产出带 hash 的 script/link templ 组件（含依赖 CSS）；开发模式（配置开关或 manifest 缺失）直连 localhost:5173 并注入 `@vite/client`；ReverseProxy middleware 转发静态前缀（含 WebSocket 透传，供 HMR）。深度止于此：不做 AssetManager、不做通用资源注册表。

**Blocked by:** None (can start immediately)

- [ ] manifest.json 解析：entry 名 -> 带 hash URL + 依赖 CSS 清单
- [ ] vite.Entry(name) 渲染 script/link（含依赖 CSS）
- [ ] dev 模式回退：直连 5173 + @vite/client 注入
- [ ] ReverseProxy middleware：静态前缀转发 + ws 透传（HMR）
- [ ] 单测：解析、hash URL、CSS 依赖、dev 回退
