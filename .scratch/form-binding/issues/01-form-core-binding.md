# 01: form 核心绑定（form-urlencoded body + query 两源）

Status: claimed

**What to build:** 新 `form` 包的请求绑定机制：`form.Bind(r, &f)` 解析 x-www-form-urlencoded body、`form.BindQuery(r, &f)` 解析 URL query，赋值到带 `form` tag 的 struct——tag 即白名单（未打 tag 的字段绝不赋值），请求中未知字段静默忽略（`_csrf` 等隐藏字段天然兼容）。类型覆盖标量全集（string / 各定长整型 / float / bool）、`[]string`、`time.Time`（RFC3339）、实现 `encoding.TextUnmarshaler` 的自定义类型。类型不匹配或解析失败**逐字段**归并进 `validators.Errors.Fields`，消息走 i18n（复用 validators 的翻译路径）；全部成功返回 nil。本工单属编解码类高风险逻辑，**走 TDD**（先红后绿）。

**Blocked by:** None (can start immediately)

- [ ] `form.Bind`（form-urlencoded body）与 `form.BindQuery`（query）可用，共用同一反射赋值核心
- [ ] tag 白名单：未打 tag 的导出字段保持零值；未知字段忽略不报错
- [ ] 类型集全覆盖：标量、`[]string`、`time.Time`(RFC3339)、`TextUnmarshaler`
- [ ] 类型错误逐字段进 `validators.Errors.Fields` 且经 i18n；全部成功返回 nil
- [ ] 空表单 / 空 query 绑定零值 struct 不报错
- [ ] TDD：测试套件先红后绿，覆盖上述全部分支
