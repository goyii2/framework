# 09: validators（zog 场景集成）

**Status:** ready-for-agent

**What to build:** zog 验证与模型接线：模型 `Rules()` 返回 场景 -> zog schema 映射，`Validate(scenario)` 返回 `map[字段][]消息`；保存链路按 Insert/Update 场景自动触发，校验失败拒绝落库；消息模板可自定义（i18n 翻译在 18 接）。

**Blocked by:** 08

- [ ] 不同场景启用不同规则集（如：必填仅 insert 场景；唯一性校验 update 时排除自身）
- [ ] 保存链路自动校验：非法数据被拒绝且无 SQL 写入发生
- [ ] 错误形态为 字段 -> 消息数组；消息模板可自定义
- [ ] API 形态下错误可经 web.Context 以 422 JSON 返回
