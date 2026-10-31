# 04: 登录表单迁移：表单模型闭环示范（basic + advanced）

Status: resolved

**What to build:** 两示例的登录表单改为**表单模型**：登录 payload struct 实现 `validators.Ruled`（login 场景规则集），action 用 `form.Load` 一步完成绑定+校验；删除裸 zog `loginSchema` 与手工翻译 `translateIssues` 的旁路写法，错误形状统一为 `validators.Errors`。basic 的 JSON/HTML 双模式分流保留：JSON 走 422 扁平错误，HTML 重渲染经 `form.Values` 回填用户名与错误回显。这是"非落库表单走闭环"的示范案例——登录表单不是 AR，独立 struct 本就是表单模型的正统形态。

**Blocked by:** 03

- [x] basic：登录 HTML 提交（错误回显 + 值回填）、JSON 422、成功跳转全绿
- [x] advanced：登录表单同形态迁移，错误展示统一 `validators.Errors`
- [x] `loginSchema` 与 `translateIssues`（及其 basic 版本）删除，无残留调用
- [x] 校验消息仍走 i18n，迁移后翻译行为不回退
- [x] 既有登录相关测试通过（随迁移合理更新后通过）

## Comments

- **2026-10-31 实施验收（落地提交 `07c3895`，经 `70d87d9` 认领；feat/form-binding-04 已 ff-only 合并 main 并删除）**
- 交付物：`examples/basic/controllers/site.go`（loginPayload+loginSchema+translateIssues 删除，改为 `loginForm` 实现 `validators.Ruled`——自定义 `scenarioLogin` 场景，Required 前缀规则链保留原消息键；ActionLogin 改 `c.LoadForm(&f, scenarioLogin)` 一步绑定+校验，JSON/HTML 分流保留，HTML 两处失败重渲染均经 `form.Values(&f)["username"]` 回填，错误统一 `errs.Fields`）、`examples/advanced/frontend/controllers/site.go`（同形态迁移，`z.Issues.Flatten` 旁路删除）、`examples/basic/controllers/site_test.go`（新增 3 测试）、`examples/advanced/frontend/frontend_test.go`（既有空提交测试扩展半填场景）。
- 测试证据：basic 新增 `TestLoginBrowserFlow`（401 错误回显+用户名回填 / 422 半填仅报空字段+回填 / 成功 302 /posts + /me 会话绑定）、`TestLoginJSONFlow`（空提交 422 扁平 `{"username":["username cannot be blank"],"password":["password cannot be blank"]}` / 坏 JSON 422 `{"$root":["malformed JSON body"]}` / 错误凭据 401 / 正确凭据 200 账号回复）、`TestLoginTranslatesValidationMessages`（zh-CN 请求回显"用户名不能为空""密码不能为空"——消息键未变，走 validators.MessageCategory 翻译路径）；advanced `TestLoginBlankFieldsAnswerPerFieldErrors` 扩展（半填提交 422 仅报 password 空值 + `value="demo"` 回填），`TestLoginLogoutFlow` 原样通过。
- 全量回归：`rtk go test ./...` → **1519 passed in 52 packages**（前票 1516 + 本票 3）；`rtk go vet ./...` 干净；`golangci-lint run ./examples/basic/... ./examples/advanced/frontend/...` 0 issues；gofmt 干净。
- 行为裁决（迁移固有，已有测试固化）：basic 坏 JSON body 从原裸 400（`web.BadRequest`）改为统一 422 扁平错误 `$root` 键——LoadForm 闭环的错误形状统一所要求；消息键/文案零改动（"username cannot be blank" 等在 `examples/basic/messages/zh-CN/app.json` 既有覆盖，无需补齐语言文件）。advanced 无 i18n 源，消息按原文渲染，与迁移前一致。
- 遵循 03 票 zog 陷阱记录：username/password 规则均为 `Required(z.Message(...))` 前缀，空提交触发可断言错误（basic/advanced 空提交测试均按消息文案断言）。
