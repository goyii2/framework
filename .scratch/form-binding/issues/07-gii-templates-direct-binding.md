# 07: gii 模板迁移：直绑 + 列表过滤骨架

Status: claimed

**What to build:** 把 examples 已证明的形态固化进 gii 生成物：模型模板给可编辑列生成 `form` tag；CRUD 控制器模板淘汰 `FormInput`（全 string 字段中间层）、`read{{.Model}}Form`、`apply`、`FormValues` 三件套，create/update 改直绑（create 直绑零值记录、update 先查再绑，ActiveForm 回填经 `form.Values`）；ActionList 生成过滤骨架——filter struct（`form` tag）+ `form.BindQuery` 读取 + `Apply`（字符串列 LIKE+转义、其余列等值，转义辅助随生成物自带，生成的是项目代码允许携带）+ `keep` 闭包。生成物测试断言同步更新。

**Blocked by:** 03, 05, 06

- [ ] 模型模板：可编辑列生成 `form` tag
- [ ] CRUD 生成物不再含 `FormInput` / `read{{.Model}}Form` / `apply` / `FormValues`；create/update 直绑形态与 examples 一致
- [ ] ActiveForm 的 Values 经 `form.Values` 导出，Errors 直接来自 Load/Save 链
- [ ] 列表骨架生成 filter struct + `BindQuery` + `Apply`（LIKE 转义自带）+ `keep` 闭包
- [ ] gii 生成测试断言更新且通过；生成物经编译与基本功能验证
