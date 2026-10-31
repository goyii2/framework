# 07: gii 模板迁移：直绑 + 列表过滤骨架

Status: resolved

**What to build:** 把 examples 已证明的形态固化进 gii 生成物：模型模板给可编辑列生成 `form` tag；CRUD 控制器模板淘汰 `FormInput`（全 string 字段中间层）、`read{{.Model}}Form`、`apply`、`FormValues` 三件套，create/update 改直绑（create 直绑零值记录、update 先查再绑，ActiveForm 回填经 `form.Values`）；ActionList 生成过滤骨架——filter struct（`form` tag）+ `form.BindQuery` 读取 + `Apply`（字符串列 LIKE+转义、其余列等值，转义辅助随生成物自带，生成的是项目代码允许携带）+ `keep` 闭包。生成物测试断言同步更新。

**Blocked by:** 03, 05, 06

- [x] 模型模板：可编辑列生成 `form` tag
- [x] CRUD 生成物不再含 `FormInput` / `read{{.Model}}Form` / `apply` / `FormValues`；create/update 直绑形态与 examples 一致
- [x] ActiveForm 的 Values 经 `form.Values` 导出，Errors 直接来自 Load/Save 链
- [x] 列表骨架生成 filter struct + `BindQuery` + `Apply`（LIKE 转义自带）+ `keep` 闭包
- [x] gii 生成测试断言更新且通过；生成物经编译与基本功能验证

## Comments

- **2026-10-31 实施验收（落地提交 `f3bff89`，经 `f9b6c1d` 认领；feat/form-binding-07 已 ff-only 合并 main 并删除）**
- 交付物：`gii/templates/model.go.tmpl`（可编辑列 `form` tag + struct 文档注释说明白名单策略）、`gii/templates/crud_controller.go.tmpl`（直绑 + 过滤骨架全量重写）、`gii/generate.go`（formTagged 白名单谓词、buildFilterField/filterType、淘汰 applyCode/valueStmt/typedAssign/intBits 与五个 Uses* 助手旗标）、`gii/generate_test.go` / `gii/examples_test.go`（断言全面更新 + assertNotContains 守卫）、`docs/组件-gii代码生成器.md`（model/crud 节同步新形态）。
- **白名单裁决（已固化测试）**：主键、`..._id` 后缀引用列、二进制列、`created_at`/`updated_at` 不打 `form` tag（05 票先例的 gii 化；`_id` 后缀即外键探测——gii 无可移植 FK 内省）；`_id` 列在 Rules 骨架追加「绑定后服务端补值」TODO 提示。测试表 product 新增 `category_id INTEGER NOT NULL` 列固化该裁决（TestGenerateModelMapsColumnsAndRules 断言 `form:"category_id"` 不存在、TestGenerateCrud 断言无对应表单/过滤字段）。
- **datetime 列裁决**：绑定层日期时间线上格式为 RFC 3339（规格第 4 项），datetime-local 控件提交的 `YYYY-MM-DDTHH:MM` 无法解析——生成物把 datetime 列渲染为带 RFC 3339 占位符的 text 输入（`Placeholder: "2026-01-02T15:04:05Z"`），JSON 与 HTML 双路径均可往返；旧 datetime-local 输入不再生成。datetime 列照常进白名单与过滤（等值、`IsZero` 判空）。
- **直绑形态（与 05 票一致）**：ActionSave 以 `c.Param(pk) != ""` 分流——create 构造零值记录 + `c.LoadForm(record, ScenarioInsert)`；update `find{{.Model}}(c)` 先查再绑 + `ScenarioUpdate`；Save 的 validators 错误经 `errors.As` 回 422 重渲染。render{{.Model}}Form 改收 `verrs *validators.Errors`（Errors 直接来自 Load/Save 链），`Values: form.Values(record)`。`FormInput`/`readForm`/`apply`/`FormValues` 及 stringPtr/boolPtr/checkboxValue/timeValue/formTimeLayout 助手全部淘汰（测试有 assertNotContains 守卫清单）。
- **过滤骨架（与 06 票一致）**：`{{.ModelVar}}Filters` struct（导出字段 + `form` tag，字符串列 string/LIKE、其余列按列类型等值——零值即无过滤，bool 只能过滤 true 的限制写入 Apply 注释）；`form.BindQuery` 读取 + 错误 `web.BadRequest` 归档（06 裁决）；`Apply` 挂 `ar.Query`；likeEscapeClause/likeEscaper/likePattern 转义辅助随生成物自带（仅存在字符串列时生成，同 basic 示例形态）；keep 闭包 = `form.Values(&filters)` 派生；网格字符串列带 `Filter: &view.ColumnFilter{}` 过滤行输入 + `FilterValues: view.QueryValues(...)` 回显。
- **机制缺陷小修（随票附带回归测试，均已报告）**：① form 包此前不支持指针字段——生成模型的可空列即 `*string`/`*time.Time` 等，提交非空值会 panic。修复：`nullableTarget` 解引用按需分配 pointee，解析失败回滚保持 NULL（form/bind.go + form/pointer_test.go 7 测试：body/query/JSON 三源、空值/null 不分配、坏值不产生 pointee、Values 导出 nil→""/指针接收者 MarshalText 类型照常渲染）；② view.ActiveForm checkbox 原以「非空即选中」判定，`form.Values` 的 bool `false` 渲染为非空 "false" 会把未勾选框错误回填为选中。修复：仅 "1"/"true"/"on" 判选中（view/activeform.go + TestActiveFormCheckboxRefillStates 4 分支）。
- 顺带修复的既有潜伏缺陷：旧生成器对「仅 created_at 为时间列」的表会置 UsesTime 而控制器已无 time 引用（未用 import 编译失败）；新代码仅在时间主键或可编辑时间列时引入 time。
- 测试证据：`rtk go test ./gii/... -count=1` → **16 passed**（gii 10 + commands 5 + examples 界内）；全量 `rtk go test ./... -count=1` → **1532 passed in 52 packages**（前票 1524 + form 7 + view 1）；`rtk go vet ./...` 干净；`golangci-lint run ./form/... ./view/... ./gii/...` 0 issues；gofmt 干净。
- **生成物编译级验证**：product 全列型测试表生成三件套到模块内临时包 `tmp-gencheck`，`rtk go build ./tmp-gencheck/` 与 `rtk go vet` 均干净（生成物对真实框架包做完整类型检查；探针文件验证后删除，未入库）；既有 gii 测试机制（go/parser 可解析 + gofmt clean + examples/basic 迁移表生成验收 TestGenerateCrudAgainstExamplePostTable）保留并加强。
- 行为变化说明（迁移固有）：① update 省略字段保留库值（旧 apply 全字段覆写）——本票核心目标；② 可空列空提交不再置 NULL（旧 stringPtr("")→nil 可清空；绑定层「空=未填」语义，机制既有裁决）；③ checkbox 未勾选提交不置 false（HTML 勾选框无法区分省略与假值，骨架注释已说明按需加哨兵）；④ `_id` 列不再出现在生成的表单（旧骨架允许手输引用 id）——05 票引用列服务端赋值教义的落实。
- 遗留/待人工项：09 票（ADR 0006 + 使用文档收尾）承载本批裁决的文档化；GridView 过滤回显按请求原值（未 trim）与 06 票一致，非本票范围。
