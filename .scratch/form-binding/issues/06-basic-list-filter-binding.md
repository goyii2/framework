# 06: basic 列表过滤迁移（BindQuery + Apply 保留）

Status: claimed

**What to build:** examples/basic 列表页的过滤读取改走表单绑定：`postFilters` 打 `form` tag 经 `form.BindQuery` 读取，`readPostFilters` 的手写 `Query().Get` 消失；`Apply`（title 的 LIKE 转义 + status 等值）与 `active()` 保留在项目代码——过滤**语义**显式、读取**机制**归框架。分页/排序 URL 保留过滤值的 `keep` 闭包改由过滤 struct 派生（不再逐字段手抄键名）。`dataprovider.FromRequest` 的分页排序解析**维持现状不动**。

**Blocked by:** 01

- [ ] 标题 / 状态过滤行为与迁移前一致（含 LIKE 转义防注入用例）
- [ ] `readPostFilters` 删除；`keep` 闭包不再逐字段手抄
- [ ] `Apply` / `active()` 保留在项目代码，查询组装仍显式
- [ ] `FromRequest` 调用方式不变
- [ ] 既有列表相关测试通过
