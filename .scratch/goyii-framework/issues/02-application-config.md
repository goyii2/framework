# 02: Application 强类型容器 + config 双轨

**Status:** claimed

**What to build:** 框架的心脏（ADR-0001）：Application 为强类型组件容器——组件接口字段 + 懒初始化（并发安全），经 context 携带贯穿请求链路（FromContext），另有包级默认实例供 CLI/后台使用。config 双轨：值走 yaml 多环境 + 环境变量覆盖，行为走代码组装。yaml/env 加载库选型在本票定稿。

**Blocked by:** 01

- [ ] New(Config) 装配应用；组件首次访问时懒初始化，并发安全（竞态测试）
- [ ] FromContext 取回请求作用域应用实例；包级默认实例在无 context 场景可用
- [ ] 多环境配置加载（命名方案按 PRD 草案定稿），环境变量可覆盖任意配置值
- [ ] 组件可替换：接口注入 fake 的单测
- [ ] yaml 库选型决定记入 dev-docs（勾销 PRD 开放问题）
