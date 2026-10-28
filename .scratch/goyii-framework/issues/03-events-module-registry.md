# 03: 事件调度 + Module 注册表

**Status:** ready-for-agent

**What to build:** base 包两件语义：类型化事件调度（On 订阅 / Trigger 触发，含 BeforeRequest/AfterRequest 应用生命周期事件）；Module 单层注册表（路由前缀、controller 归属登记，组件作用域与嵌套接口预留）。模型生命周期事件不走此机制（由 GORM 钩子承担，见 PRD 放弃清单）。

**Blocked by:** 02

- [ ] On/Trigger 并发安全，事件携带请求上下文，支持注销
- [ ] BeforeRequest/AfterRequest 单测模拟触发（真实请求接线在 04）
- [ ] Module 注册表可登记前缀与 controller 归属；嵌套能力接口预留（P2 验证）
- [ ] 订阅/触发/注销全路径单测
