# 01: form 核心绑定（form-urlencoded body + query 两源）

Status: resolved

**What to build:** 新 `form` 包的请求绑定机制：`form.Bind(r, &f)` 解析 x-www-form-urlencoded body、`form.BindQuery(r, &f)` 解析 URL query，赋值到带 `form` tag 的 struct——tag 即白名单（未打 tag 的字段绝不赋值），请求中未知字段静默忽略（`_csrf` 等隐藏字段天然兼容）。类型覆盖标量全集（string / 各定长整型 / float / bool）、`[]string`、`time.Time`（RFC3339）、实现 `encoding.TextUnmarshaler` 的自定义类型。类型不匹配或解析失败**逐字段**归并进 `validators.Errors.Fields`，消息走 i18n（复用 validators 的翻译路径）；全部成功返回 nil。本工单属编解码类高风险逻辑，**走 TDD**（先红后绿）。

**Blocked by:** None (can start immediately)

- [x] `form.Bind`（form-urlencoded body）与 `form.BindQuery`（query）可用，共用同一反射赋值核心
- [x] tag 白名单：未打 tag 的导出字段保持零值；未知字段忽略不报错
- [x] 类型集全覆盖：标量、`[]string`、`time.Time`(RFC3339)、`TextUnmarshaler`
- [x] 类型错误逐字段进 `validators.Errors.Fields` 且经 i18n；全部成功返回 nil
- [x] 空表单 / 空 query 绑定零值 struct 不报错
- [x] TDD：测试套件先红后绿，覆盖上述全部分支

## Comments

- **2026-10-31 实施验收（落地提交 `b4294cd`，经 `7a885d9` 认领；feat/form-binding-01 已 ff-only 合并 main 并删除）**
- 交付物：`form/bind.go`（Bind / BindQuery / 共享核心 bindValues+setField）、`form/doc.go`（包定位与安全模型）、`form/bind_test.go`（13 测试）、`form/i18n_test.go`（4 测试）。
- TDD 三轮红绿：① Bind/BindQuery 未定义（build fail 红）→ 实现 tag 扫描 + string 赋值核心；② 全类型表测试触发 `unsupported type` panic（红）→ 实现整型/浮点/bool/[]string/time.Time/TextUnmarshaler；③ 翻译与坏 body 三个用例红（`Fields["age"]` 未翻译、无 $root）→ 接通 i18n 翻译与 `$root` 归并。
- 测试证据：`rtk go test ./form/... -count=1 -cover` → **17 passed, coverage 95.1%**。用例清单：TestBindFormBodyAssignsTaggedStringFields、TestBindQueryAssignsTaggedStringFields、TestEmptySourcesLeaveZeroValues、TestBindIgnoresNonFormBodies、TestNonStructDestinationsPanic、TestBindAssignsAllSupportedTypes、TestBindQueryAssignsAllSupportedTypes、TestTypeErrorsCollectPerField、TestEmptyValuesLeaveNonStringFieldsUntouched、TestScalarFieldsReadTheFirstValue、TestIntOverflowFailsPerField、TestUnsupportedTaggedFieldTypePanics、TestMalformedBodyReportsRootError、TestTypeErrorsAreTranslated、TestTypeErrorsWithoutSourceStayVerbatim、TestTypeErrorsOtherLanguageFallsBack、TestQueryTypeErrorsAreTranslated。
- 全量回归：`rtk go test ./...` → **1486 passed in 52 packages**；`rtk go vet ./...` 干净；`golangci-lint run ./form/...` 无问题；gofmt 干净。
- 设计决定（已写入 doc.go，02/03 票接续时注意）：错误载体为 `*validators.Errors{Scenario: form.ScenarioBind("bind")}`，消息 id（not a valid integer / number / boolean / date-time / value、malformed form body）走 `validators.MessageCategory` i18n 路径；Bind 以 Content-Type 门禁只认 x-www-form-urlencoded（02 票加分发）；Body 与 query 严格分源（PostForm vs URL.Query）；单个空值视为"未填写"——非文本字段跳过不报错（必填与否交 validators 裁决）；无法解码的 body 归并 `$root` 键；不支持的 tag 字段类型收到值时 panic（fail-fast）。
