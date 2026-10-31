# 05: basic 文章 create/update 直绑迁移

Status: claimed

**What to build:** examples/basic 文章创建/编辑改为**直绑**：create 构造零值记录后 `form.Load(ScenarioInsert)`；update **先按 id 查出记录再 `Load(ScenarioUpdate)` 覆写**（未提交字段保留库值，防零值覆盖）；JSON 与 HTML 双路径共用同一表单模型（模型上的 `form` tag 即契约）。中间层 `postPayload`（JSON DTO）与 `postFormInput` / `readPostForm`（HTML 表单读取）淘汰删除；服务端补值的字段（如时间戳）在 Load 之后显式赋值。

**Blocked by:** 03

- [ ] create：JSON 提交与 HTML 表单提交均走直绑，校验失败 422 / 回显路径行为不回退
- [ ] update：先查再绑；HTML 表单未包含的字段不被零值覆盖（有测试证明）
- [ ] `postPayload` / `postFormInput` / `readPostForm` 及其调用点删除
- [ ] JSON 路径多余字段（含大小写变体）无法越权赋值
- [ ] 既有 CRUD 相关测试通过（随迁移合理更新后通过）
