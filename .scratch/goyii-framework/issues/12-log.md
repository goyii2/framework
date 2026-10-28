# 12: log 组件

**Status:** ready-for-agent

**What to build:** 基于 log/slog 的日志组件：分级/分类（category 过滤）、多 Target（console/file，可扩展）、性能 profile（计时区间产出开始/结束与耗时）；日志器经 Application 取用。

**Blocked by:** 02

- [ ] 分级与 category 过滤正确（命中/排除规则各自验证）
- [ ] console 与 file Target 输出验证（file 含滚动或至少追加写入）
- [ ] profile 计时区间产出配对的开始/结束日志与耗时
- [ ] Target 为接口，自定义 Target 单测注入可行
