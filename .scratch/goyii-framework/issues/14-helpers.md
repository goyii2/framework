# 14: helpers 包

**Status:** resolved

**What to build:** 惯用帮助函数包：Array/Str/File/Json/Url/Html 的 Yii2 高频能力对应物（数组取值/合并/多维查询、字符串转写、文件查找/复制、JSON 安全编码、URL 与 HTML 生成），命名遵循 Go 惯用法而非 PHP 下划线风格。

**Blocked by:** 01

- [x] 各 helper 单测覆盖 Yii2 高频使用场景
- [x] 命名与 Go 惯用法一致（包限定短名，如 helpers.ArrayGet）
- [x] Html 输出默认转义，显式 raw 才输出原文

## Comments

### 验收证据（2026-10-28）

- 提交：feat `e15ade1`（main，经 feat/helpers ff-only 合并，分支已删）；认领 `1bf16f5`（chore: claim ticket 14 helpers）。
- 测试：`go test ./...` 17 包 992 项全过（本票新增 helpers 包 43 项）；`CGO_ENABLED=1 go test -race -count=1 ./helpers/` 全过；`go vet ./...`、`gofmt -l`、`golangci-lint run ./helpers/...` 全部干净。
- 命名：包限定短名——`helpers.ArrayGet/ArrayGetOr/ArrayMerge/ArrayMap/ArrayFilter/ArrayColumn/ArrayIndex`、`helpers.StrRandom/StrCamel/StrSnake/StrTruncate/StrUpperFirst/StrLowerFirst`、`helpers.FileExists/FileIsDir/FileFind/FileCopyDir`、`helpers.JsonMarshal/JsonMarshalString/JsonUnmarshal`、`helpers.UrlQuery/UrlBuild/UrlAddQuery`、`helpers.HtmlEscape/HtmlRaw/HtmlDecode/HtmlTag/HtmlA`；Map/Filter/Index 用泛型，无 PHP 下划线搬运。
- XSS 硬标准（`html_test.go`）：`HtmlTag` 文本内容默认经 `html.EscapeString`（`<img onerror>` payload 全量转义）；属性值引号转义（`&#34;`，`" onmouseover="` 无法破出引号）；标签名/属性名注入校验（非法即拒绝/丢弃）；verbatim 输出仅两条显式通道——`HtmlRaw`（信任标记）与 `HtmlEscape`（已转义标记，builder 不二次转义，`TestHtmlTagRendersHtmlValuesVerbatim` 验证）；`JsonMarshal` 默认 HTML 转义（`</script>` 不落地为原文）。
- Array：`ArrayGet` 多级点路径（map 下钻 + `items.0.name` 切片索引，JSON 形状）；`ArrayMerge` 对齐 Yii2 ArrayHelper::merge（map 递归、slice 追加、标量覆盖），返回全新树（别名测试证明改结果不污染输入）；Map/Filter 空结果保持非 nil（JSON 编码为 `[]` 而非 `null`）。
- File：`FileFind` 通配查找（段内 `*`/`?`、跨段 `**`、无 `/` 模式匹配文件名即任意深度命中、`CaseInsensitive` 选项、坏模式即报错非静默不匹配）；`FileCopyDir` 递归复制保留权限位、跳过符号链接、拒绝 dst 与 src 嵌套/相等。
- Url：纯函数拼装（无全局状态，不依赖路由表）——`UrlQuery` 键排序 + 百分号编码、`UrlBuild` 字面路径拼 query（已有 `?` 则 `&` 续接）、`UrlAddQuery` 解析-合并-重编码（同键覆盖、fragment 保留、坏 URL 报错）；与 `url.To`（命名路由）分工互补，共用 `url.P` 参数类型。

### API 分叉与决策（相对 Yii2）

1. **Html 用类型而非开关管转义**：`HTML` 类型（`HtmlRaw`/`HtmlEscape` 产出）是唯一 verbatim 通道，builder 对普通 string/Stringer/数值一律转义——比 Yii2 的 encode 参数更防呆，编译器可见"已处理"标记，杜绝二次转义陷阱。
2. **`StrRandom` 用 crypto/rand + 拒绝采样**（模偏置剔除），非 Yii2 的 OpenSSL 随机等价物 mcrypt；熵源失败 panic（进程无法安全制造"随机"值）。
3. **`StrSnake` 缩略语边界比 Yii2 underscore 聪明**：`XMLParser -> xml_parser`（Yii2 会产出 `x_m_l_parser`）；边界 = 小写/数字后遇大写、或大写连串末尾接小写。
4. **`FileFind` 语义显式化**：无 `/` 的模式匹配文件名（任意深度，用户直觉，对应 Yii2 `*.php` 用法习惯）、有 `/` 匹配相对路径、`**` 跨段；大小写敏感为默认（Go 惯例），`CaseInsensitive` 显式开关；模式预先校验，坏模式报错。
5. **`ArrayGet` 只遍历 JSON 形状**（`map[string]any`/`[]any`）：覆盖 JsonUnmarshal/配置数据主场景；类型化结构体取值本就该用 Go 原生字段访问。
