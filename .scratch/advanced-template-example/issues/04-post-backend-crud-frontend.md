# 04: Post 全链路——后台 CRUD、前台展示

Status: ready-for-agent
Blocked by: 03（backend 模块、RBAC 守卫与 seed）

**What to build:** common 分区新增 Post 模型（迁移 + ActiveRecord + 验证场景），backend 模块内文章管理（列表/新建/编辑/删除，服务端表单 + 校验错误回显），前台文章列表与详情页。演示高级模板核心场景：admin 在后台发布内容，匿名用户在前台浏览。

- [ ] Post 迁移入 migrate 列表；模型验证覆盖必填/长度等场景（场景化规则自定但一致）
- [ ] admin 在后台完成列表/新建/编辑/删除文章；校验失败回显错误且不落库
- [ ] 匿名用户前台看到文章列表与详情；前后台链接用命名路由反向生成
- [ ] 后台 CRUD 与前台展示 httptest 测试通过（含一条校验失败回显用例）
- [ ] `go build ./...` 与 `go test ./...` 通过，gofmt 干净

## Comments
