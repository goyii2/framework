# 06: ADR 0007 修订 + docs 更新（builder 形态）

Status: ready-for-agent

**What to build:** 把 builder 取代两函数形态的决策写回纸面。修订 `dev-docs/adr/0007`：API 小节改为 builder 形态（`Build`/`String`/`Absolute`/`Host`），注明取代理由——链式调用是 Yii2 `Url::to($url, $scheme)` 可选参数语义的 Go 表达，「选择在调用点、值在配置」同时化解了显式性与重复的两难；被否备选补记三组：`WithHostInfo` 派生视图（被 `.Host()` 取代，机器更少）、调用点裸拼 host（本分区绝对链接每处重复部署知识、无单一校验点）、`To` 改返 builder（50+ 调用点机械扰动）。`docs/组件-web路由与中间件.md` 绝对生成小节同步为 builder 用法（含双域名 `.Host(...)` 示例与失败行为）。CONTEXT.md「主机信息」词条核对（语义不变：管理器上的默认域，builder 下为 `.Absolute()` 的缺省前缀）。

**Blocked by:** 04, 05

- [ ] ADR 0007 API 小节改 builder 形态，取代理由与三组被否备选齐备
- [ ] docs 绝对生成小节为 builder 用法，含双域名示例
- [ ] 文档与实现逐一对得上：方法名、失败标记（`"#"`）、配置键（`hostInfo` / `backendHostInfo` / env 名）
- [ ] CONTEXT.md 词条与实现一致，无重复添加
