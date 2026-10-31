# 04: url 包 builder 形态改造（Build / Absolute / Host）

Status: ready-for-agent

**What to build:** 绝对生成从两函数形态（`ToAbsolute`/`AbsoluteURL`）迁到链式 builder：`url.Build(name, params)` 返回可链式 builder，`.String()` 产相对 URL（失败 `"#"`），`.Absolute()` 产绝对 URL（hostInfo 前缀，失败含未配置时 `"#"`），`.Host(h)` 为**本次生成**显式指定域（跨分区/多租户场景），值在 `.Absolute()` 时校验（解析合法 + 尾斜杠容错，非法产 `"#"`，请求路径不 panic）。builder 类型命名避免与 `net/url.URL` 混淆，公开面只需 `Build`/`String`/`Absolute`/`Host`。删除包级 `ToAbsolute`；`Manager.AbsoluteURL` 收为 unexported 供 builder 内部使用；`SetHostInfo` / `To` / `Manager.URL` 签名与行为不动。链式即 Yii2 `Url::to($url, $scheme)` 可选参数的 Go 表达：选择在调用点（链到哪级），值在配置（hostInfo 管理器持有）。不走 TDD（URL 拼装类），实现后全量测试验证。

**Blocked by:** None (can start immediately)

- [ ] `url.Build(name, params)` 可用，`.String()` 等价 `To(name, params)`（相对、失败 `"#"`）
- [ ] `.Absolute()` 默认走管理器 hostInfo；未配置产 `"#"`
- [ ] `.Host(h)` 显式域：合法值（含尾斜杠）拼前缀，非法值 `.Absolute()` 产 `"#"`，不 panic
- [ ] 包级 `ToAbsolute` 删除；`Manager.AbsoluteURL` 收为 unexported；`SetHostInfo`/`To`/`URL` 不动，既有测试绿
- [ ] 测试迁移：`ToAbsolute` 既有用例改 builder 断言，新增 Host 覆盖/非法/尾斜杠与 String 相对路径用例
- [ ] 全量测试绿；golangci-lint / gofmt 干净
