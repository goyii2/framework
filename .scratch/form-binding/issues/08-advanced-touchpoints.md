# 08: advanced 其余绑定点迁移

Status: resolved

**What to build:** examples/advanced 收尾迁移：backend 文章表单的 `readPostForm` 改直绑（create 零值记录、update 先查再绑，同 basic 形态）；system 用户列表触点核查——`FromRequest` 分页排序**维持不动**，仅确认是否存在应顺手迁移的字段读取（如 per-page 判断），核查结论写进工单 Comments。advanced 全量回归绿。

**Blocked by:** 03

- [x] backend 文章 create/update 直绑，`readPostForm` 及调用点删除
- [x] system 用户列表核查结论（保持现状的项及理由）记录在工单 Comments
- [x] `FromRequest` 分页排序调用不变
- [x] advanced 全量测试通过

## Comments

- **2026-10-31 实施验收（落地提交 `30e3535`，经 `e85312d` 认领；feat/form-binding-08 已 ff-only 合并 main 并删除）**
- 交付物：`examples/advanced/common/models/post.go`（Title/Body/Status 补 `form` tag——对齐 05 票白名单裁决：主键 ID、author_id 引用列、Author 关联与 Timestamp 行为字段均无 tag 不入白名单，struct 文档注释说明该策略）、`examples/advanced/backend/controllers/post.go`（`postFormInput`/`readPostForm` 及全部调用点删除；ActionNew 零值记录置 status=draft 后经 `form.Values` 出回填；ActionStore 零值记录 → `c.LoadForm(post, ScenarioInsert)` → AuthorID 在 Load 后显式赋值（服务端补值字段）→ Save；ActionSave **先查再绑**：findPost（404 语义不变）→ `c.LoadForm(post, ScenarioUpdate)` 覆写 → Save；renderPostForm/replayPostForm 改收 `map[string]string` 回填值）、`examples/advanced/backend/post_test.go`（既有流程测试扩展 + 新增 1 端到端测试）。views 无需改动（ActiveForm 消费 map 与错误集，形状不变）；frontend 侧（登录表单/文章只读页）04 票已迁移，本票核查无残留触点。
- 测试证据：`TestBackendPostManagement` 扩展——compose 表单断言 `<option value="draft" selected>`（form.Values 零值记录回填默认选中）；compose 提交携带越权键 `id=999`/`author_id=2`，断言行 id 非注入值、AuthorID 保持服务端赋值的 admin(1)（白名单不赋值）。新增 `TestBackendPostSaveOmittedFieldKeepsStoredValue`——save 表单不含 body 字段提交后库中 body 保留原值、title/status 正常覆写（**先查再绑的零值覆盖防护，HTML 路径测试证明**；旧 readPostForm 形态此处会清空 body）。既有 `TestBackendPostValidationEchoesWithoutPersisting` 原样通过（422 回显 + 回填 + 不落库）。`rtk go test ./examples/advanced/... -count=1` → **21 passed in 12 packages**（前基线 20 + 本票 1）。
- 全量回归：`rtk go test ./... -count=1` → **1533 passed in 52 packages**（前票 1532 + 本票 1）；`rtk go vet ./...` 干净；`golangci-lint run ./examples/advanced/...` 0 issues；gofmt 干净。
- **system 用户列表触点核查结论（保持现状的项及理由）**：advanced 非测试代码的请求参数触点全量清点仅两处，均在 `examples/advanced/backend/system/user.go` ActionList——① `dataprovider.FromRequest(c.Request(), sortableUserFields...)`：分页排序解析，规格第 10 项与工单正文均明确**维持不动**（排序白名单属取数层语义，与字段绑定不同层），本票一字未改；② `c.Request().URL.Query().Get("per-page") == ""` 的页大小默认判断：属**分页参数层**读取（决定页面固定默认页大小是否生效），非"应顺手迁移的字段读取"——绑定层管模型字段（过滤/表单值），分页参数语义归 dataprovider 一侧；且该列表无过滤行、无任何模型字段读取，不存在可建 filter struct 的对象；与 06 票 basic 列表同位置 per-page 读取保持现状的先例一致。结论：**两处均保持现状**。
- 行为裁决（迁移固有，已有测试固化）：update 表单省略字段从「清空为空串」（旧 readPostForm 缺省值语义）改为「保留库值」——本票核心目标；compose 提交的越权字段（id/author_id 等）旧代码因逐字段读取同样忽略，新形态由模型白名单结构性保证（测试固化）。
- 遗留/待人工项：无；09 票（ADR 0006 + 使用文档收尾）承接整批裁决文档化。
