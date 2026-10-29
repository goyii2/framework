# 23: P2 规划拷问

**Status:** resolved

**What to build:** P2 范围（queue、debug 调试面板、Gii 代码生成器、Vite manifest 集成、mutex、PageCache、GridView/ListView/ActiveForm 服务端渲染大件与客户端校验、httpclient/authclient 扩展、嵌套 Module 完整验证、Casbin 适配器）进入实施前，以 grill 会话逐项拷问细节（PRD 开放问题），产出决策记录与下一批工单拆解。本票不产出代码，优先级最低（建议在 22 之后启动）。

**Blocked by:** None (can start immediately)

- [x] P2 各项经拷问形成共识并拆解为新工单
- [x] 明确不做项以 wontfix 落盘

### 2026-10-29 拆解落盘

工单 25–36 已按共识发布（`.scratch/goyii-framework/issues/25..36-*.md`），blocked-by 链对齐实施顺序；可并行起步 frontier：25 / 26 / 29 / 30 / 31 / 33 / 35。本票 resolve；集成收口见 36。

## Comments

### 2026-10-29 grill 会话（Round 1 + Round 2 全部接受推荐）

共识：12 项 P2 语义定稿，已回写 `dev-docs/PRD.md`「P2 生态」节与放弃清单；术语九条入根目录 `CONTEXT.md`（队列/队列任务/互斥锁/整页缓存/活动表单/列表大件/调试面板/代码生成器/资源清单）。

**wontfix（本仓库不做）**：

- httpclient 扩展——Go 标准生态 net/http 已完备，fluent 包装无净收益
- mongo / es 的 AR 扩展对应物——AR 绑 SQL 语义，直接用官方 client
- 服务端渲染 Widget 最小资源注册表——客户端校验不内建（zog 无公开内省 API），注册表无消费方
- Casbin 适配器实现——ADR-0003 社区位维持，交付改为 docs「自定义 AuthManager 适配指南」

**实施顺序**（拆票时转为 blocked-by 链）：① 嵌套 Module → ② redis 共享段 + queue/mutex 装配 → ③ mutex → ④ queue → ⑤ PageCache → ⑥ ActiveForm → ⑦ GridView/ListView → ⑧ debug 面板 → ⑨ Vite → ⑩ Gii → ⑪ authclient → ⑫ P2 集成验收票（对齐 15/22 先例）。

集成边界：examples/basic 增嵌套 admin demo、queue demo、PageCache 页、列表/表单页换三件套、debug 面板开启；Gii 产物不入库；Vite 加最小 `web/` 前端工程（构建产物 manifest 入库）；authclient 不进 examples 主线。

剩余动作：按共识拆解 P2 工单后本票 resolve。
