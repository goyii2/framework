# 13: cache 组件

**Status:** claimed

**What to build:** 缓存组件：Cache 接口 + memory/file/redis 三实现 + 依赖失效（Dependency：依赖值变化/时间条件触发失效）；TTL 与并发安全。

**Blocked by:** 02

- [ ] 三后端行为一致：get/set/delete/存在性/过期（redis 用 miniredis）
- [ ] Dependency 失效：依赖变化后缓存条目被判定失效并重建
- [ ] 并发访问安全（竞态测试）
- [ ] Cache 为接口，业务侧可注入 fake（单测）
