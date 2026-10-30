# 09: validators（zog 场景集成）

Status: resolved

**What to build:** zog 验证与模型接线：模型 `Rules()` 返回 场景 -> zog schema 映射，`Validate(scenario)` 返回 `map[字段][]消息`；保存链路按 Insert/Update 场景自动触发，校验失败拒绝落库；消息模板可自定义（i18n 翻译在 18 接）。

**Blocked by:** 08

- [x] 不同场景启用不同规则集（如：必填仅 insert 场景；唯一性校验 update 时排除自身）
- [x] 保存链路自动校验：非法数据被拒绝且无 SQL 写入发生
- [x] 错误形态为 字段 -> 消息数组；消息模板可自定义
- [x] API 形态下错误可经 web.Context 以 422 JSON 返回

## Comments

**落地形态**（main `5a44fc1`，分支 feat/validators 已 ff 合入并删除；zog 固定 `github.com/Oudwins/zog v0.23.0`，其依赖 golang.org/x/exp 入 indirect）：

- **场景 `validators.Scenario`**（`type Scenario string`）：内置 `ScenarioInsert` / `ScenarioUpdate` 常量，自定义场景直接 `validators.Scenario("login")` 作 RuleSet 键。
- **规则集 `validators.RuleSet`** = `map[Scenario]*zog.StructSchema`：模型实现 `Rules() RuleSet`（`Ruled` 接口）即参与验证。schema 键寻址导出字段（首字母小写自动映射大写，如 "name" -> Name）；键无对应字段属编程错误，zog 会 panic（直通不吞）。
- **验证入口 `validators.Validate(model, scenario) *Errors`**：nil 即通过；模型无 Rules、或该场景无 schema（Yii2 语义：场景无规则＝合法）同样返回 nil。失败返回 `*validators.Errors{Scenario, Fields map[string][]string}`（zog `Issues.Flatten` 产物，record 级问题落 `$root` 键），`Error()` 字段排序稳定输出。PRD 共识形态由 `ar.Model.Validate(scenario) map[string][]string` 直接兑现（nil 即合法）。
- **API 分叉记录——RuleSet 类型形态**：选 `map[Scenario]*zog.StructSchema` 具体类型而非自定义接口/结构体包装。理由：① 工单与 PRD 逐字要求「场景 -> *zog.StructSchema 映射」，zog schema 本身就是可复用、可组合的规则载体；② 包装层只会转发 zog 构造器，徒增 API 面积；③ Rules() 按调用构建（Yii2 rules() 同款心智），需要实例状态的规则（唯一性排自身）以闭包捕获 receiver，静态 schema 可放包级 var 复用，两种写法都自然。
- **API 分叉记录——Save 挂钩方式**：选「08 的 Save 链路内挂接」（`validateBeforeSave` 在解析 GORM 句柄后、任何语句执行前运行），而非外部包装函数。理由：① 挂在链内才能保证「非法数据拒绝且无 SQL 写入」对一切调用方成立，包装函数依赖调用者自觉；② 场景判定（主键零值 -> insert，否则 update）与 Save 本身的写路径判定同源（`primaryKey` + `ValueOf`），链内可直接复用；③ 模型未定义 Rules() 时单次类型断言即直通，零 zog 调用，满足零开销直通。实现注意：成功路径必须显式 `return nil`，直接透传 `*Errors` 会把 nil 指针包成非 nil error（typed-nil 陷阱，测试曾全数踩中后修复）。
- **规则内查库（唯一性排自身）**：`ar.Model` 新增 `Context()` 暴露记录绑定 ctx；`Rules()` 里 `z.String().TestFunc(...)` 闭包经 `u.Context()` 走 `ar.New[T](ctx).Where(...)` 查询，update 场景加 `id <> ?` 排除自身行——Yii2 UniqueValidator 语义。
- **消息模板覆盖点**（i18n 翻译留 18 号票）：① 单规则 `z.Message("...")` / `z.MessageFunc(...)`（zog 原生，优先）；② 全局默认表 `conf.DefaultIssueMessageMap[zconst.TypeX][zconst.IssueCodeY] = "模板 {{value}}"`（zog conf 包，占位符 `{{参数名}}`/`{{value}}`）。validators 不另造转发层——覆盖点即 zog 自身 seam，18 号票接 i18n 时直接换 `conf.IssueFormatter` 或按语言切换 map。doc.go 记录两处用法。
- **web 接线**：`web.Context.JSONStatus(status, v)`（写 JSON + 显式状态码）。Action 侧范式：`errors.As(err, &verrs)` 后 `return ctx.JSONStatus(http.StatusUnprocessableEntity, verrs.Fields)`；验证错误不进统一错误处理器（其 body 形态 {name,message,status} 装不下字段映射表，HTML 协商渲染待 P1 视图层，本票按「经 web.Context」的工单措辞走显式路径）。

**验收证据**（合并后 main `5a44fc1` 上，CGO 经 scoop mingw gcc 启用）：

- `go build ./...` / `go vet ./validators/... ./db/ar/... ./web/...` / `gofmt -l validators db/ar web` 全部无输出；`go test ./...` 全仓 17 包 760 测试全绿；`CGO_ENABLED=1 go test -race -count=1 ./validators/... ./db/ar/... ./web/...` 三包 ok（64 项，无 data race）。
- **不同场景不同规则集**（validators 11 项 + db/ar 8 项）：`TestScenariosSelectDifferentRuleSets` 断言同一记录（空 name）insert 拒 / update 过（必填仅 insert）、自定义场景 "login" 走自己的 schema、无 schema 场景直通；`db/ar.TestSaveValidatesUpdateScenarioWithSelfExclusion` 断言 update 保留自身 email 通过（排除自身行）而抢占他人 email 被拒。
- **保存链路自动校验且无 SQL 写入**：`TestSaveRejectsInvalidInsertBeforeWriting` 断言 insert 场景拒绝后表计数 0；`TestSaveValidatesInsertUniqueness` 断言唯一性冲突拒绝后仅存首行；update 拒绝后断言库中原行未被改写（`FindByID` 复核 email 原值）。`TestSavePassesModelWithoutRules` / `TestSavePassesScenarioWithoutSchema` 覆盖零开销直通两分支。
- **错误形态 + 模板可自定义**：`TestValidateCollectsPerFieldMessages`（字段 -> 消息数组精确断言，含 Scenario 回填）、`TestValidateCollectsSeveralMessagesPerField`（一字段多条消息）、`TestStructLevelIssuesLandUnderRootKey`（record 级问题落 `$root`）、`TestErrorsErrorString`（Error() 排序稳定）；`TestGlobalTemplateOverride` 以 `conf.DefaultIssueMessageMap[zconst.TypeString][zconst.IssueCodeRequired] = "{{value}} 不能为空"` 覆盖全局模板并断言含中文与占位符渲染（t.Cleanup 还原全局表）；单规则 `z.Message` 在各用例中直接断言。
- **422 JSON 经 web.Context**（validators/web_test.go，包外 `validators_test`）：`TestValidationErrorsReturnAsUnprocessableJSON` 端到端——httptest 打真实 Handler（应用带 sqlite 连接、显式路由 POST /user），Action 内 JSON 绑定 -> `ar.New[apiUser](c.Context()).New()` -> Save -> `errors.As` -> `ctx.JSONStatus(422, verrs.Fields)`；断言状态码 422、Content-Type application/json、body 恰为 `{"name":["name cannot be blank"],"email":["email is not a valid address"]}`，且库中计数 0；`TestValidRecordSavesThroughTheEndpoint` 断言合法 body 201 落库。

**遗留**：i18n 翻译按 18 号票接（届时换 `conf.IssueFormatter` 或按语言 map，本票 seam 已留）；本机 golangci-lint v2.13.1 以 go1.26 构建而仓库文件要求 go1.27，对包括未改动包在内的所有包 panic（环境问题，`go vet` + `gofmt` 已覆盖等价检查，非本票引入）。
