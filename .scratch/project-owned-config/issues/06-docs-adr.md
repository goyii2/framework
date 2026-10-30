# 06: 文档与决策记录

Status: claimed
Blocked by: 05

**What to build:** 按 spec 第 6 节产出文档：ADR 0002（project-owned-config-and-code-wiring，部分取代 0001——强类型 Application 保留；记录配置所有权反转、统一初始化五规则、急切装配语义、config 纯叶子工具包的取舍及否决项）；docs/配置参考.md 重写为 LoadAs + overlay + env 覆盖 + 组件参数包指南；docs/组件-base与config.md 同步访问器 / 装配 / 默认清单。文档内容与 05 落地后的真实 API 一致。

- [ ] dev-docs/adr/0002 落盘：背景 / 决策 / 后果 / 取代关系与备选否决理由齐备
- [ ] docs/配置参考.md 以 LoadAs 与组件参数包为主轴重写，含环境变量覆盖与 overlay 说明
- [ ] docs/组件-base与config.md 与最终访问器 / 急切装配 / 默认清单 / panic 语义一致
- [ ] 文档示例代码与 examples/basic 实际代码一致（无虚构 API）
