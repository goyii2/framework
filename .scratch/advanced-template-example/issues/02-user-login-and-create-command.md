# 02: 用户登录与 user/create 命令

Status: ready-for-agent
Blocked by: 01（骨架与前台首页）

**What to build:** common 分区的 User 模型（ActiveRecord + 验证 + identity 查找）落地，`user/create` 自定义命令（对应 Yii2 高级模板同名命令）创建用户，前台登录/登出表单与会话状态。演示：CLI 建用户 → 前台表单登录 → 导航显示已登录、可登出；错误凭据回显错误。

- [ ] `go run . user/create`（参数形态自定，如 --username/--email/--password）创建用户成功并有输出确认
- [ ] common 的 identity 查找接入 handler 的 IdentityFinder；登录基于 User 模型验证
- [ ] 前台登录表单：凭据错误回显错误不落库会话；正确凭据登录后导航显示已登录，登出后回到匿名态
- [ ] 登录/登出流程 httptest 测试通过（错误凭据回显 + 正确凭据登录态 + 登出）
- [ ] `go build ./...` 与 `go test ./...` 通过，gofmt 干净

## Comments
