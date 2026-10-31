# 03: form.Load 闭环 + web.Context 便捷方法 + form.Values 回填导出

Status: resolved

**What to build:** 表单模型闭环的机制层收口：`form.Load(ctx, r, &f, scenario)` 按分发完成绑定后，对实现了 `validators.Ruled` 的 struct 调 `validators.Validate`，绑定错误与校验错误**统一**为 `*validators.Errors` 返回（nil = 通过）；未实现 `Ruled` 时仅绑定不校验。`web.Context` 增薄委托便捷方法（一行转发到 form 包，不引入第二套逻辑）。`form.Values(f)` 反射导出 `map[string]string`（按 `form` tag 取键，各类型导出为字符串），供 ActiveForm 的 `Values` 回填，替代手写 map。

**Blocked by:** 02

- [x] `form.Load` = 绑定 + 按场景校验一步；返回统一 `*validators.Errors`，nil = 通过
- [x] 绑定错误与规则错误合并在同一 `Errors.Fields`（同字段并存时有序、可断言）
- [x] 未实现 `Ruled` 的 struct 走 Load 仅绑定，不 panic 不报错
- [x] `web.Context` 便捷方法为薄委托，行为与直接调 form 包一致
- [x] `form.Values`：各类型字符串化（time 按 RFC3339、bool `true`/`false` 等），未打 tag 字段不出现在结果里
- [x] 单测覆盖上述全部分支

## Comments

- **2026-10-31 实施验收（落地提交 `8973379`，经 `d80c880` 认领；feat/form-binding-03 已 ff-only 合并 main 并删除）**
- 交付物：`form/load.go`（Load + mergeFields）、`form/values.go`（Values + exportString）、`form/doc.go`（Load 闭环与 Values 回填语义两节 + 接线示例改用 LoadForm/Values）、`web/context.go`（`LoadForm`）、`form/load_test.go`（10 测试）、`form/values_test.go`（6 测试）、`web/context_test.go`（+2 测试）。
- Load 语义：Bind（错误仍为 ScenarioBind）→ 对实现 Ruled 的模型调 `validators.Validate(ctx, dst, scenario)` → 两阶段 Fields 合并为一；**绑定失败后校验照跑**（校验面对绑定后的模型），同字段消息顺序固定为「解析错误在前、规则错误在后」，`$root`（坏 body / 结构级规则）同样按此序并存；合并结果的 `Scenario` = Load 所请求的场景；未实现 Ruled 仅绑定（Validate 对非 Ruled 返回 nil），无 schema 的场景无可失败。
- Context 方法名裁决：**`LoadForm(dst any, scenario validators.Scenario) *validators.Errors`**——一行转发 `form.Load(c.Context(), c.r, dst, scenario)`，无第二套逻辑；`TestLoadFormMatchesCallingFormLoadDirectly` 以 DeepEqual 断言与直调 form.Load 结果及绑定后模型完全一致。
- Values 字符串化裁决（已落 doc.go「Refilling」节）：string 原样；整型/浮点最短精确表示（strconv 'g' -1）；bool `true`/`false`；**[]string 逗号连接，nil/空列表渲染 `""`**；`encoding.TextMarshaler` 字段渲染其 MarshalText——time.Time 即 RFC3339（**小数秒保留**，编辑表单回填再提交不丢精度，零值渲染 `0001-01-01T00:00:00Z`）；仅有 UnmarshalText 无 MarshalText 的类型（如自定义 role）**不入结果**（无诚实文本形态，不猜测）；未打 tag / `form:"-"` 字段不出现在结果；非 struct / nil 指针 panic（对齐 newBinder 的 fail-fast）。model 接受值或指针。
- 测试证据：`rtk go test ./form/... -count=1 -cover` → **45 passed, coverage 93.5%**（前票 29）。用例清单：TestLoadBindsAndValidatesInOneStep、TestLoadBindsTheJSONSource、TestLoadMergesBindAndRuleErrorsInOrder（同字段两消息顺序断言：`["not a valid integer", "age cannot be blank"]`）、TestLoadRejectsParsedValuesTheRulesDislike、TestLoadReportsRuleFailuresAlone、TestLoadReportsBindFailuresOnRulelessFields、TestLoadWithoutRulesOnlyBinds、TestLoadScenarioWithoutSchemaPasses、TestLoadKeepsRootErrorsFromBothStages（`$root` = `["malformed JSON body", "record as a whole is off"]`）、TestLoadTranslatesThroughTheRequestContext；TestValuesExportsEveryTaggedFieldAsText、TestValuesRendersZeroAndEmptyStates、TestValuesOmitsFieldsWithoutATextForm、TestValuesOmitsUntaggedAndOptedOutFields、TestValuesAcceptsValueAndPointerModels、TestValuesPanicsOnNonStructModels；web：TestLoadFormBindsAndValidates、TestLoadFormMatchesCallingFormLoadDirectly。
- 全量回归：`rtk go test ./...` → **1516 passed in 52 packages**（前票 1498 + 本票 18）；`rtk go vet ./...` 干净；`golangci-lint run ./form/... ./web/...` 无问题；gofmt 干净。
- 实现备注：zog 原生类型默认可选——零值字段不触发无 Required 前缀的规则链，故测试模型的 age 规则为 `Required(...).GT(17,...)` 才能在「绑定失败保零值」场景触发规则消息（Required 失败短路后续规则，两消息断言稳定）。
