# 29: PageCache

**Status:** claimed

**What to build:** 整页缓存 Filter（web 包，形态对齐 HttpCache）：`PageCache{Duration, Dependency, VaryBy}`——命中直接回放 body + status + Content-Type，未命中捕获响应后写入应用 cache 组件。key = 路由 + 规范化查询串 + VaryBy；仅 200 的 GET 文本响应参与；失效走 cache.Dependency。body 捕获的 ResponseWriter 包装导出，供 debug 面板 toolbar 注入复用（32 依赖此点）。

**Blocked by:** None (can start immediately)

- [ ] PageCache Filter 落地：仅 200 GET 文本响应缓存，POST/写动作不参与
- [ ] 命中回放（含 Content-Type）；未命中捕获后按 Duration/Dependency 写入 cache 组件
- [ ] key = 路由 + 规范化查询串 + VaryBy；依赖失效重建
- [ ] body 捕获 writer 导出可复用
- [ ] 单测：命中/未命中/依赖失效重建/VaryBy 变体分裂
