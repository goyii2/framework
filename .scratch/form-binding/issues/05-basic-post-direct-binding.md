# 05: basic 文章 create/update 直绑迁移

Status: resolved

**What to build:** examples/basic 文章创建/编辑改为**直绑**：create 构造零值记录后 `form.Load(ScenarioInsert)`；update **先按 id 查出记录再 `Load(ScenarioUpdate)` 覆写**（未提交字段保留库值，防零值覆盖）；JSON 与 HTML 双路径共用同一表单模型（模型上的 `form` tag 即契约）。中间层 `postPayload`（JSON DTO）与 `postFormInput` / `readPostForm`（HTML 表单读取）淘汰删除；服务端补值的字段（如时间戳）在 Load 之后显式赋值。

**Blocked by:** 03

- [x] create：JSON 提交与 HTML 表单提交均走直绑，校验失败 422 / 回显路径行为不回退
- [x] update：先查再绑；HTML 表单未包含的字段不被零值覆盖（有测试证明）
- [x] `postPayload` / `postFormInput` / `readPostForm` 及其调用点删除
- [x] JSON 路径多余字段（含大小写变体）无法越权赋值
- [x] 既有 CRUD 相关测试通过（随迁移合理更新后通过）

## Comments

- **2026-10-31 实施验收（落地提交 `2efbc8e`，经 `704c299` 认领；feat/form-binding-05 已 ff-only 合并 main 并删除）**
- 交付物：`examples/basic/models/post.go`（Title/Body/Status 补 `form` tag——模型侧契约，ID/AuthorID/Author 及 Timestamp 行为字段无 tag 不入白名单）、`examples/basic/controllers/post.go`（postPayload/postFormInput/readPostForm 及全部调用点删除；ActionCreate/ActionStore 构造零值记录 → `c.LoadForm(post, ScenarioInsert)` → AuthorID 在 Load 后显式赋值（本示例的时间戳由 Timestamp 行为落值，服务端补值字段即作者引用）→ Save；ActionUpdate/ActionSave **先查再绑**：FindByID/findPost 取记录（404 语义不变）→ `c.LoadForm(post, ScenarioUpdate)` 覆写 → Save；ActionNew 零值记录置默认 status=draft 后经 `form.Values` 出回填；renderPostForm/replayPostForm 改收 `map[string]string` 回填值）、`examples/basic/controllers/post_test.go`（保留 likePattern 单测，新增端到端装配 newPostHandler：内存 sqlite + 真实迁移含 RBAC 表 + rbac.DbManager（editor 角色 → post/create、post/update 边，指派给 admin）+ 站点登录/CSRF 路由 + 文章全部写路由）。
- 测试证据：新增 3 端到端测试。`TestPostJSONCreateFlow`：合法 body 201 + Location `/post/{id}` + 库值/作者断言；规则拒绝 422 扁平 map（title 长度、status one-of）且库计数不变；坏 JSON 422 `$root`=[malformed JSON body]；**越权键全量证明**——`Title`（大小写变体）、`ID`/`Id`、`AuthorID`/`author_id`、`created_at`/`CreatedAt` 均不赋值（title 仅由精确 `title` 键绑定、作者保持登录账号、id 自增、时间戳非 2030/2031）。`TestPostJSONUpdateFlow`：PUT 省略 `body` 键 → 库中 body 保留原值（先查再绑的零值覆盖防护，JSON 路径）；带越权键 author_id/ID/created_at 同样不赋值；404（id 不存在）；title 空 422 扁平 map 且库值不变。`TestPostBrowserWriteFlow`：新表单页 draft 默认选中；表单创建 302 至 `/posts/{id}` + 库值/作者断言；拒绝提交 422 回显消息 + 回填值 + 库计数不变；编辑页预填；**保存表单不含 body 字段 → 库中 body 保留**（HTML 路径零值覆盖防护测试证明）；拒绝保存 422 + 回填。
- 全量回归：`rtk go test ./...` → **1522 passed in 52 packages**（前票 1519 + 本票 3）；`rtk go vet ./...` 干净；`golangci-lint run ./examples/basic/...` 0 issues；gofmt 干净。
- 行为裁决（迁移固有，已有测试固化）：① 坏 JSON body 从裸 400 改为统一 422 扁平 `$root` 键（02/04 票坏 body 统一 422 决定的延续）；② JSON/HTML update 省略字段从「清空为零值」（旧 payload 拷贝语义）改为「保留库值」——本票的核心目标；③ PUT 空 body / 无 Content-Type 从 400（EOF）变为合法空提交（绑定零字段、Save 无变化、200）；④ ActionCreate 身份检查先于 body 解析（访客由 AccessControl 中间件先行 401，action 内检查仅为类型断言兜底，顺序变化不可观测）。
- 范围说明：`examples/advanced/backend/controllers/post.go` 仍含同名 postFormInput/readPostForm——属 08 票（advanced 触点迁移）范围，本票未动；gii 模板侧 form tag 化属 07 票。
