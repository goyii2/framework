# 04: url 包 builder 形态改造（Build / Absolute / Host）

Status: resolved

**What to build:** 绝对生成从两函数形态（`ToAbsolute`/`AbsoluteURL`）迁到链式 builder：`url.Build(name, params)` 返回可链式 builder，`.String()` 产相对 URL（失败 `"#"`），`.Absolute()` 产绝对 URL（hostInfo 前缀，失败含未配置时 `"#"`），`.Host(h)` 为**本次生成**显式指定域（跨分区/多租户场景），值在 `.Absolute()` 时校验（解析合法 + 尾斜杠容错，非法产 `"#"`，请求路径不 panic）。builder 类型命名避免与 `net/url.URL` 混淆，公开面只需 `Build`/`String`/`Absolute`/`Host`。删除包级 `ToAbsolute`；`Manager.AbsoluteURL` 收为 unexported 供 builder 内部使用；`SetHostInfo` / `To` / `Manager.URL` 签名与行为不动。链式即 Yii2 `Url::to($url, $scheme)` 可选参数的 Go 表达：选择在调用点（链到哪级），值在配置（hostInfo 管理器持有）。不走 TDD（URL 拼装类），实现后全量测试验证。

**Blocked by:** None (can start immediately)

- [x] `url.Build(name, params)` 可用，`.String()` 等价 `To(name, params)`（相对、失败 `"#"`）
- [x] `.Absolute()` 默认走管理器 hostInfo；未配置产 `#`
- [x] `.Host(h)` 显式域：合法值（含尾斜杠）拼前缀，非法值 `.Absolute()` 产 `"#"`，不 panic
- [x] 包级 `ToAbsolute` 删除；`Manager.AbsoluteURL` 收为 unexported；`SetHostInfo`/`To`/`URL` 不动，既有测试绿
- [x] 测试迁移：`ToAbsolute` 既有用例改 builder 断言，新增 Host 覆盖/非法/尾斜杠与 String 相对路径用例
- [x] 全量测试绿；golangci-lint / gofmt 干净

## Comments

- 2026-10-31 实施（commit 864047e，feat/absolute-url-04 已 --ff-only 合回 main 并删分支）：`url` 包新增 `Builder` 值类型（字段 m/name/params/host）与包级 `Build(name, params)`（经 Default 管理器，与 `To` 同源）；`String()` 直走 `m.URL` 吞错返回 `"#"`（与 `To` 等价）；`Absolute()` 无显式域时走收窄为 unexported 的 `Manager.absoluteURL`（未配置 hostInfo 显性 error → builder 转 `"#"`），有显式域时经共享校验后拼前缀；`Host(h)` 记录本次生成的域（空串清除覆盖，`String()` 不受影响）。host 值校验抽为 `normalizeHostInfo`（`SetHostInfo` boot 期 panic 与 builder 请求期 `"#"` 共用同一合法形状：scheme+host 必备、尾斜杠容错、userinfo/多余路径/query/fragment 拒绝），`SetHostInfo` 的 panic 消息仍含 host info + wiring 字样。内部函数 `build` 更名 `renderPattern`（避免与新导出 `Build` 大小写混淆）。包级 `ToAbsolute` 删除，全仓 Go 代码无残留引用。
- 超出工单正文的最小必要改动（保持 main 绿所需）：examples 两处 Permalink 触点（basic post.go / advanced frontend post.go）从 `url.ToAbsolute(...)` 机械迁移为 `url.Build(...).Absolute()`，行为逐字节等价（既有断言测试全绿未动）；示例的完整 builder 迁移与双域名演示归 05 号工单。docs/ADR 0007 中旧两函数表述按工单分工留给 06 号修订，本票不改文档。
- 验收证据：`go test -count=1 ./url/` 23 例全过（TestBuildAbsolutePrefixesHostInfo / TestBuildAbsoluteKeepsPort / TestSetHostInfoTrailingSlashTolerated / TestBuildAbsoluteConventionFallback / TestBuildAbsoluteWithoutHostInfoIsHash / TestBuildAbsoluteMissingPathParamIsHash / TestBuildUsesDefaultManager / TestBuildStringMatchesTo（4 例断言 String 与 To 逐一一致）/ TestBuildHostOverridesPrefix（覆盖默认域、尾斜杠、String 忽略覆盖、管理器未配置时显式域可用）/ TestBuildHostInvalidValuesAnswerHash（8 例非法值，无 recover——panic 即测试失败）/ TestSetHostInfoConcurrentWithReads（builder 形态，含 race 检测）/ 内部 TestAbsoluteURLInternalErrors（未配置显性 error、缺路径参数 error、成功路径）；既有 To/URL/SetHostInfo 测试未改动全绿）。`scripts/p0-check.sh` 全绿：gofmt clean、go vet clean、`CGO_ENABLED=1 go test -race -count=1 ./...` 37 个含测试包全部 ok、examples/basic 构建 + 冒烟 31/31 通过（"P0 checks green."）。`golangci-lint run ./url/... ./examples/basic/... ./examples/advanced/...` 0 issues。
- 双轴自审（编码规范＋规格符合）：规范轴——注释自含、无工单/ADR 编号引用（Yii2 为领域参照，与包内既有注释一致）；`Builder` 命名规避 net/url.URL 混淆；host 一体字符串为既有 ADR 值形状决策（Primitive Obsession 嗅觉被仓库决策压制）；`Absolute`/`String` 的吞错 `"#"` 形态沿用包内 `To` 惯用法。规格轴——六条验收逐条满足；`SetHostInfo`/`To`/`URL` 签名行为未动（仅内部抽共享校验与内部函数更名）；无半成品落地。环境备注：首次 p0-check 冒烟被一残留 basic.exe 进程（占用 examples/basic/runtime 的 sqlite 文件）卡住，结束该残留进程后完整重跑全绿。
- 待人工：无。
