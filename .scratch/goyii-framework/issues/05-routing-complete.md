# 05: 路由完整化

**Status:** claimed

**What to build:** 完整路由能力（ADR-0002）：基于标准库 ServeMux 的显式绑定，`{id:[0-9]+}` 参数语法、`.Name()` 可选命名、`url.To(name, url.P{...})` 反向生成（命名表 -> 约定拼装兜底）、`/{controller}/{action}` 约定回退（默认开、可关、显式路由优先）、Module 分区前缀挂载。

**Blocked by:** 04

- [ ] 显式绑定路由的参数可经 web.Context 类型化取值（ParamInt 等）
- [ ] 命名路由反向生成正确；未命名路由按约定拼装兜底（与 Yii2 Url::to 语义对齐）
- [ ] 约定回退解析未声明的 controller/action；可配置关闭；显式路由始终优先
- [ ] Module 前缀下的路由挂载与隔离验证
- [ ] 路由表单测 + httptest 集成测试
