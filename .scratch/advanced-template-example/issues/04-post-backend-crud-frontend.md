# 04: Post 全链路——后台 CRUD、前台展示

Status: resolved
Blocked by: 03（backend 模块、RBAC 守卫与 seed）

**What to build:** common 分区新增 Post 模型（迁移 + ActiveRecord + 验证场景），backend 模块内文章管理（列表/新建/编辑/删除，服务端表单 + 校验错误回显），前台文章列表与详情页。演示高级模板核心场景：admin 在后台发布内容，匿名用户在前台浏览。

- [x] Post 迁移入 migrate 列表；模型验证覆盖必填/长度等场景（场景化规则自定但一致）
- [x] admin 在后台完成列表/新建/编辑/删除文章；校验失败回显错误且不落库
- [x] 匿名用户前台看到文章列表与详情；前后台链接用命名路由反向生成
- [x] 后台 CRUD 与前台展示 httptest 测试通过（含一条校验失败回显用例）
- [x] `go build ./...` 与 `go test ./...` 通过，gofmt 干净

## Comments

### 验收证据（2026-10-31）

**迁移与模型（common 分区）**
- `examples/advanced/common/migrations/m20260101_000004_create_post_table.go`：post 表（title/body/status/author_id/created_at/updated_at），追加进 `migrations.All()`（register.go）。
- `examples/advanced/common/models/post.go`：ActiveRecord（`ar.Model` + `Author *User` 关联 + `behaviors.Hooks`/`Timestamp` 自动时间戳）+ 场景化验证：insert 校验 title 必填/3..200、status OneOf(draft,published)、body ≤10000；update 复验 title+status。`AuthorName()` 供两分区显示。

**后台 CRUD（backend 模块，深色 Layout 内）**
- `backend/controllers/post.go`：列表（全部文章含草稿）/新建/编辑/删除，服务端 ActiveForm 表单；`Save()` 被 insert/update 场景拒绝时 422 回显字段消息 + 已填值，不落库。
- `backend/backend.go`：`mod.RegisterController("post", ...)` + 显式路由 `/backend/posts{,/new,/{id},/{id}/edit,/{id}/delete}`，命名 `backend/post/index|new|store|edit|save|delete`（模块前缀自动拼接）；守卫链（302 登录 / 403 非 admin）对新路由同样生效。
- 侧边栏加 Posts 入口（`navActive` 按请求路径高亮）。

**前台展示（frontend 根作用域，匿名可见）**
- `frontend/controllers/post.go`：`/posts` 仅列 published（最新在前），`/posts/{id}` 详情（published 之外一律 404，草稿不泄露）；导航加 Posts 链接。
- 链接全部命名路由反向生成：前台导航/详情/返回，后台侧栏/编辑/删除/跨分区 View（指向 `/posts/{id}`）、控制器重定向；`main.go` 安装 `url.SetDefault(handler.URLs())`。

**seed 演示内容（幂等语义保持）**
- `commands/seed.go`：新增 3 篇演示文章（2 published + 1 draft，作者为 seeded admin），按标题存在即跳过；重跑输出 `post "..." already exists`，计数不变。

**测试（go test ./examples/advanced/... 16 通过；全量 `go test ./...` 1465 通过 / 50 包；gofmt 干净）**
- `backend/post_test.go`：`TestBackendPostManagement`（游客 302→/login、admin 列表/新建/编辑/删除全流程、草稿行不渲染 View 链接、删除后 DB 计数 0）；`TestBackendPostValidationEchoesWithoutPersisting`（title=ab 触发长度消息 + status=bogus 触发 OneOf 消息，422 回显、body 回填、DB 计数 0 —— 校验失败回显用例）。
- `frontend/post_test.go`：`TestFrontendShowsPublishedPostsToGuests`（匿名列表两篇 published、draft 不出现、详情 byline/正文/返回链接、draft 与未知 id 404）。
- `commands/seed_test.go`：`TestSeedCreatesDemoPosts`（2 published/1 draft、作者=admin、时间戳已盖）；`TestSeedIsRepeatable` 增加 post 计数 3 断言。

**端到端冒烟（live server）**：`go run . migrate up` → `go run . seed`（重跑幂等）→ `serve` 后 curl：`GET /posts`=200（2 篇，无草稿）；`GET /posts/1`=200（by admin）；`GET /posts/3`(draft)=404；`GET /backend/posts`(guest)=302→/login；admin 登录后 `GET /backend/posts`=200（深色框、含 `href="/posts/1"` 跨分区链接与草稿行）；坏表单 POST=422 且两条校验消息渲染。

**审查**：自审两轴（编码规范 + 规格符合）通过；修正 2 处（author 提取下沉为 `models.Post.AuthorName()` 消除两控制器重复；`navActive` 参数名遮蔽 url 包改名 target）。templ 生成物已重新生成并随源码提交。
