# 02: form JSON 源 + Content-Type 分发

Status: resolved

**What to build:** `form.Bind` 按 Content-Type 分发：`application/json` 走 **map 中转赋值**（解码到 `map[string]any` 后按 `form` tag 反射赋值，不经 `encoding/json` 直解 struct），`application/x-www-form-urlencoded` 维持既有路径。三源白名单语义统一：JSON body 中的多余字段——包括与未打 tag 导出字段大小写相近的键——赋不进任何 struct，从机制上防住直绑模型时的 over-posting；JSON 类型不匹配同样逐字段归并进 `validators.Errors.Fields`（非整包失败）。body 为空或 Content-Type 缺省/不支持时的行为做出裁决并测试固化。**走 TDD**。

**Blocked by:** 01

- [x] Content-Type 分发：JSON → map 中转赋值；form-urlencoded → 既有路径；分发行为有单测
- [x] 多余 JSON 字段（含大小写变体）不产生任何赋值（白名单语义与另两源一致）
- [x] JSON 类型不匹配逐字段报错（如字符串给 int 字段），同 body 内合法字段正常绑定
- [x] JSON 数组值可绑定 `[]string`；嵌套对象对非 `TextUnmarshaler` 字段报类型错误
- [x] 空 body / 缺省 Content-Type / 不支持类型的行为已裁决并有测试
- [x] TDD：测试套件先红后绿

## Comments

- **2026-10-31 实施验收（落地提交 `ec5969a`，经 `0cae984` 认领；feat/form-binding-02 已 ff-only 合并 main 并删除）**
- 交付物：`form/bind.go`（Bind 按 Content-Type 分发 + bindJSONBody map 中转 + setJSONField 类型转换 + binder 共享白名单走查）、`form/json_test.go`（新增 12 测试）、`form/bind_test.go`（TestBindIgnoresNonFormBodies 改写为 TestBindIgnoresUnsupportedBodies）、`form/i18n_test.go`（+2 测试）、`form/doc.go`（JSON 源语义全量落档）。
- TDD 七轮红绿（提交即轮次）：① JSON 分发+字符串赋值（红：Login 未赋值）→ 19749c3；② 全类型矩阵+逐字段类型错误（红：数字/bool 不赋值、无错误归并）→ 6a11c7a；③ 数组绑 []string（红：panic not supported）→ f4c952d；④ 空串/null 视为未填写（红：`""`→int 报 not a valid integer）→ 2512ff3；⑤ 坏 JSON 归并 $root（红：返回 nil）→ aefbdd0；⑥ 空 body 合法空提交（红：误报 $root）→ 907e932；⑦ 自审补洞：整包 `null` body 判 malformed（红：静默 nil；Go 对 null→map 不报错的坑）→ ec5969a。另有两类"到达即绿"的固化测试如实说明：缺省/不可解析 Content-Type 不绑定（既有门禁行为，本票裁决固化并落 doc.go）；JSON 源 i18n 翻译两例（翻译路径与前票共享，fixture 新增 malformed JSON body 词条验证）。
- 测试证据：`rtk go test ./form/... -count=1 -cover` → **29 passed, coverage 93.3%**（前票 17）。JSON 源用例：TestBindJSONBodyAssignsTaggedStringFields、TestBindJSONAssignsAllSupportedScalarTypes、TestJSONTypeErrorsCollectPerField、TestJSONValueKindMismatchOnStringFieldFails、TestJSONArrayValuesBindStringSlice、TestJSONArraysOfNonStringsAndNestedObjectsFailPerField、TestJSONEmptyTextAndNullMeanUnfilled、TestMalformedJSONBodyReportsRootError、TestJSONEmptyBodyBindsNothing、TestBindWithoutContentTypeBindsNothing、TestJSONTypeErrorsAreTranslated、TestMalformedJSONBodyIsTranslated。全量回归：`rtk go test ./...` → **1498 passed in 52 packages**；`rtk go vet ./...` 干净；`golangci-lint run ./form/...` 无问题；gofmt 干净。
- Content-Type 边界行为裁决（已落 doc.go「Sources」节）：① 空/纯空白 body + `application/json` = 合法空提交（不赋值不报错，与空表单对齐；`{}` 同义）；② Content-Type 缺省或不可解析（mime.ParseMediaType 报错）= 未声明任何 body 源，不绑定不报错——只显式声明支持类型者才路由；③ 不支持的类型（text/plain、multipart 等）同 ②不绑定不报错；④ 坏 JSON（截断/非对象/整包 null/尾随垃圾/多值）归并 `$root`："malformed JSON body"（走 validators.MessageCategory i18n 路径）；⑤ 分发容忍 charset 等参数后缀。
- 设计决定（03 票接续时注意）：JSON 值强类型匹配——字符串给数值字段报该字段自身消息（integer/number/boolean/date-time），数值给文本字段报 not a valid value，不双向隐式转型；数字保留字面量（json.Number）经 strconv 解析，19.5/溢出不静默取整；数组仅入 []string（元素须全为文本），单字符串=单元素列表（镜像表单单值），`[]`=显式空列表，嵌套对象/数组给标量字段=类型错误，TextUnmarshaler 字段仅吃字符串形式（time.Time 仍走 RFC3339 专属消息）；JSON `""`/null 对非文本字段=未填写（与前票空值语义一致）；三源共享 binder 走查（newBinder/eachTagged/fail/errs），白名单与错误形态由结构保证一致；错误载体沿用 `*validators.Errors{Scenario: form.ScenarioBind("bind")}` + translateMessages 镜像翻译。
