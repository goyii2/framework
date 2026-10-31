# 03: form.Load 闭环 + web.Context 便捷方法 + form.Values 回填导出

Status: claimed

**What to build:** 表单模型闭环的机制层收口：`form.Load(ctx, r, &f, scenario)` 按分发完成绑定后，对实现了 `validators.Ruled` 的 struct 调 `validators.Validate`，绑定错误与校验错误**统一**为 `*validators.Errors` 返回（nil = 通过）；未实现 `Ruled` 时仅绑定不校验。`web.Context` 增薄委托便捷方法（一行转发到 form 包，不引入第二套逻辑）。`form.Values(f)` 反射导出 `map[string]string`（按 `form` tag 取键，各类型导出为字符串），供 ActiveForm 的 `Values` 回填，替代手写 map。

**Blocked by:** 02

- [ ] `form.Load` = 绑定 + 按场景校验一步；返回统一 `*validators.Errors`，nil = 通过
- [ ] 绑定错误与规则错误合并在同一 `Errors.Fields`（同字段并存时有序、可断言）
- [ ] 未实现 `Ruled` 的 struct 走 Load 仅绑定，不 panic 不报错
- [ ] `web.Context` 便捷方法为薄委托，行为与直接调 form 包一致
- [ ] `form.Values`：各类型字符串化（time 按 RFC3339、bool `true`/`false` 等），未打 tag 字段不出现在结果里
- [ ] 单测覆盖上述全部分支
