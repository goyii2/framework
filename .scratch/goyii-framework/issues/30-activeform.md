# 30: ActiveForm

**Status:** claimed

**What to build:** 服务端活动表单 widget：ActiveForm + ActiveField（label/input/hint/error 渲染），从 validators.Errors 自动回显字段错误并回填提交值，与既有 CSRF 联动。input 原生校验属性（required/maxlength/pattern 等）由字段配置显式声明——不做校验规则自动导出（zog 无公开内省 API，见 PRD 放弃清单），JS 客户端校验不在框架内。

**Blocked by:** None (can start immediately)

- [ ] ActiveForm/ActiveField 以现有 widget 形态（配置 struct + Render() templ 组件）落地
- [ ] 常用字段类型：text/textarea/password/hidden/select/checkbox/radio/number/datetime-local/file
- [ ] validators.Errors 自动错误回显 + 提交值回填
- [ ] CSRF 联动（沿用既有机制）
- [ ] 单测：渲染断言（错误回显、值回填、默认转义）
