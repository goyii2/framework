# 04: 登录表单迁移：表单模型闭环示范（basic + advanced）

Status: claimed

**What to build:** 两示例的登录表单改为**表单模型**：登录 payload struct 实现 `validators.Ruled`（login 场景规则集），action 用 `form.Load` 一步完成绑定+校验；删除裸 zog `loginSchema` 与手工翻译 `translateIssues` 的旁路写法，错误形状统一为 `validators.Errors`。basic 的 JSON/HTML 双模式分流保留：JSON 走 422 扁平错误，HTML 重渲染经 `form.Values` 回填用户名与错误回显。这是"非落库表单走闭环"的示范案例——登录表单不是 AR，独立 struct 本就是表单模型的正统形态。

**Blocked by:** 03

- [ ] basic：登录 HTML 提交（错误回显 + 值回填）、JSON 422、成功跳转全绿
- [ ] advanced：登录表单同形态迁移，错误展示统一 `validators.Errors`
- [ ] `loginSchema` 与 `translateIssues`（及其 basic 版本）删除，无残留调用
- [ ] 校验消息仍走 i18n，迁移后翻译行为不回退
- [ ] 既有登录相关测试通过（随迁移合理更新后通过）
