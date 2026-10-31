# 03: ADR 0007 与文档收尾

Status: claimed

**What to build:** 把 absolute-url 的决策落纸面。`dev-docs/adr/0007-*.md`：主机信息取**仅 boot 配置**来源的决策——绝对网址的真实消费者（邮件渲染、队列任务、OAuth 回调）全部运行在无请求上下文，配置来源恰好覆盖；明确**不做请求推断**（不从当前请求的 Host / X-Forwarded-Proto 推导）及理由（需要跟随当前访问域名的场景用相对 URL 即可）；两层 API 分层（`AbsoluteURL` 显性 error vs `ToAbsolute` 吞错返回 `"#"`，复刻 `URL`/`To` 分层）与 `SetHostInfo` 非法值 fail-fast 的理由。`docs/组件-web路由与中间件.md` 在反向生成小节后增补绝对生成用法（装配接线 + `ToAbsolute` 示例 + 未配置行为）。`CONTEXT.md` 术语「主机信息（hostInfo）」已先行入库，本票核对表述与实现一致即可，不重复添加。

**Blocked by:** 01, 02

- [ ] ADR 0007 落盘：来源决策、不做请求推断的边界、API 分层、fail-fast 理由，含被否备选
- [ ] `docs/组件-web路由与中间件.md` 增补绝对生成小节（装配 + 用法 + 失败行为）
- [ ] CONTEXT.md「主机信息」词条与实现行为核对一致
- [ ] 文档不与实现矛盾：API 名、签名、失败标记逐一对得上
