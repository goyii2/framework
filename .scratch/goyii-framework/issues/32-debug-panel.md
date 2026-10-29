# 32: debug 面板

**Status:** ready-for-agent

**What to build:** 开发期请求内省工具（debug 包）：App.Debug=true 时挂采集 middleware，按请求聚合七面板——请求/路由匹配、日志、SQL、时间线、Session/User、配置摘要（脱敏）、邮件；每请求一个 JSON 落 `runtime/debug/`（默认保留 200 个、超量淘汰）。内嵌 `/debug` 路由提供请求历史与详情页，并对 200 的 text/html 响应注入页底 toolbar（注入层在 PageCache 内层，复用 29 的 body 捕获 writer）。默认仅本机 IP 可访问，可配 CIDR 列表。

**Blocked by:** 29

- [ ] 采集 middleware（仅 Debug=true）：七面板数据聚合；body 捕获复用 29 的 writer
- [ ] 日志面板：log 包新增 per-request 内存 Target；SQL 面板从 db 类别记录提取语句/耗时/行数
- [ ] 时间线复用 Profiler；配置摘要脱敏（DSN、密码等）
- [ ] 每请求 JSON 落盘 + 保留上限淘汰（默认 200 可配）
- [ ] /debug 历史 + 详情页；toolbar 仅对 200 text/html 注入、面板页自身不注入
- [ ] 访问控制：默认 127.0.0.1/::1，可配 CIDR 列表
- [ ] 单测：聚合、脱敏、淘汰、注入条件
