# 30: ActiveForm

**Status:** resolved

**What to build:** 服务端活动表单 widget：ActiveForm + ActiveField（label/input/hint/error 渲染），从 validators.Errors 自动回显字段错误并回填提交值，与既有 CSRF 联动。input 原生校验属性（required/maxlength/pattern 等）由字段配置显式声明——不做校验规则自动导出（zog 无公开内省 API，见 PRD 放弃清单），JS 客户端校验不在框架内。

**Blocked by:** None (can start immediately)

- [x] ActiveForm/ActiveField 以现有 widget 形态（配置 struct + Render() templ 组件）落地
- [x] 常用字段类型：text/textarea/password/hidden/select/checkbox/radio/number/datetime-local/file
- [x] validators.Errors 自动错误回显 + 提交值回填
- [x] CSRF 联动（沿用既有机制）
- [x] 单测：渲染断言（错误回显、值回填、默认转义）

## Comments

### 验收证据（2026-10-29，提交 302df2f）

- **widget 形态**：`view.ActiveForm{Action, Method, ID, Fields, Errors, Values, CSRF, CSRFName}` + `Render() templ.Component`，形态与 LinkPager/DetailView/Menu 一致（配置 struct + Go 侧预计算 + templ 渲染）；ActiveField 携带 Name/Label/Type/Hint/Options/Required/MaxLength/Pattern/Placeholder。类型常量 `FieldText/Textarea/Password/Hidden/Select/Checkbox/Radio/Number/DatetimeLocal/File` 十种齐备，未知值按 `input type` 透传（`TestActiveFormCustomInputTypePassesThrough`）；原生校验属性全部显式声明，无 zog 规则导出（`TestActiveFormTextDefaults` 断言 required/maxlength/pattern 渲染）。
- **错误回显**：`Errors *validators.Errors`，按字段 Name 取 `Fields map[string][]string` 回显为 `help-block` div、组 div 加 `has-error`（`TestActiveFormEchoesValidationErrors`：双消息全回显、未失败字段不带错误标记、help-block 计数=2）；`$root` 记录级问题无字段可挂、不渲染（doc 注释与测试注释写明）。
- **提交值回填**：选了表单级 `Values map[string]string` 单一机制（无字段级 Value 覆盖，doc 注释写明）：text/password/number/datetime-local 回填 value、textarea 回填内容、hidden 回填；select/radio 按「选项 Value == 回填值」标记 selected/checked（`TestActiveFormFieldTypes`）；checkbox 语义为「非空即 checked」（提交未勾选时键缺失自然为假，`TestActiveFormUnselectedControlsWithoutValue` 断言无 selected/checked）。
- **CSRF 联动**：`CSRF` + `CSRFName`（默认 `_csrf`，与 web.Csrf 默认 FormField 一致）渲染隐藏字段，token 来自 `web.Context.CsrfToken()`；空 token 省略字段（`TestActiveFormCSRFHiddenField`：默认名/自定义名/省略三态）。不另造机制。
- **默认转义**：label/hint/错误消息/值/选项值与文本全走 templ 转义（`TestActiveFormEscapesDynamicValues`：`<script>` 注入在 label、value 属性、hint、help-block、option value/label 五处全部转义，输出无裸 `<script>`）。
- **边界**：file input 不渲染 value 属性（浏览器所有且 HTML 不允许）；placeholder/maxlength/pattern 按类型适用性丢弃（number 无 maxlength 等）；hidden 字段裸渲染不带 group wrapper。
- **测试摘要**：`go test ./... -count=1` 25 包全绿（含 examples/basic 未动、11.8s 通过）；`go test ./view/ -count=2` 重复稳定；`go vet ./view/...`、`golangci-lint run ./view/...` 0 issues；gofmt 无差异。examples 表单页换用 ActiveForm 属 36 号集成票，本票未动 examples；docs 使用文档随 36 号一并补。view 包因引入 validators 依赖（Errors 类型），doc.go 依赖声明相应更新（同模块内无环）。
