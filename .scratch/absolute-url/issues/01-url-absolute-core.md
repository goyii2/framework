# 01: url 绝对生成核心（SetHostInfo / AbsoluteURL / ToAbsolute）

Status: resolved

**What to build:** `url.Manager` 增加主机信息与其上的绝对网址生成能力。`SetHostInfo(s)` 接收一体字符串 `scheme://host[:port]`（如 `https://example.com`），写读走既有 RWMutex 模式（boot 期写、请求期读）；尾斜杠容错（设 `https://example.com/` 与不带斜杠产出一致）；缺 scheme 或缺 host 的值是装配错误，boot 期 fail-fast（与 `Add` 重复注册即 panic 同哲学）。`Manager.AbsoluteURL(name, params)` 生成完整网址：路由解析行为与 `URL` 完全一致（注册名走 pattern、未注册走约定回退），仅在结果前拼 HostInfo；HostInfo 未配置时返回明确 error（装配错误显性）。包级 `url.ToAbsolute(name, params) string` 是 `AbsoluteURL` 的吞错版：任何失败（含未配置 HostInfo、注册路由缺路径参数）返回 `"#"`，与 `To` 的 cannot-create 标记一致。`To` / `URL` 签名一律不动。属 URL 拼装而非编解码类高风险，**不走 TDD**：直接实现，用测试套件验证，收尾跑全量测试。

**Blocked by:** None (can start immediately)

- [x] `SetHostInfo` 可用：一体 `scheme://host[:port]`，尾斜杠容错，RWMutex 保护并发
- [x] 非法值（缺 scheme / 缺 host）boot 期 fail-fast（panic，消息指明是装配错误）
- [x] `AbsoluteURL`：注册名走 pattern、未注册走约定回退，输出 `HostInfo + 相对路径 + 查询串` 完整网址
- [x] HostInfo 未配置时 `AbsoluteURL` 返回明确 error，不产出半截 URL
- [x] 包级 `ToAbsolute` 任何失败返回 `"#"`；成功路径与 `AbsoluteURL` 一致
- [x] `To` / `URL` 既有签名与行为不变，既有测试全绿
- [x] 新增分支全覆盖单测：配置/未配置、尾斜杠、非法值、约定回退、查询串、失败标记

## Comments

- 2026-10-31 实施（commit e581982，feat/absolute-url-01 已 --ff-only 合回 main 并删分支）：`url/Manager` 增私有 `hostInfo` 字段 + `SetHostInfo`（neturl 解析校验，尾斜杠归一；缺 scheme/host、userinfo/多余路径/query 一律 panic，消息含 "boot-time wiring mistake"）；`AbsoluteURL` 复用 `URL` 生成路径后拼接前缀，未配置时 `errors.New` 显性报错；包级 `ToAbsolute` 经 `Default()` 吞错返回 `"#"`。`To`/`URL` 未动。
- 验收证据：`go test -race -count=1 ./url/` ok（TestAbsoluteURLPrefixesHostInfo / TestAbsoluteURLKeepsPort / TestSetHostInfoTrailingSlashTolerated / TestAbsoluteURLConventionFallback / TestAbsoluteURLWithoutHostInfoIsError / TestAbsoluteURLMissingPathParamIsError / TestSetHostInfoPanicsOnInvalidValues（8 例非法值）/ TestToAbsoluteUsesDefaultManager / TestSetHostInfoConcurrentWithReads 全绿，含 race 检测）；全量 `CGO_ENABLED=1 go test -race -count=1 ./...` 全部 ok；`golangci-lint run ./url/...` 0 issues；gofmt clean。
- 双轴自审（编码规范＋规格符合）：无硬性违规；`ToAbsolute` 沿用 `To` 的吞错形态为既有包内惯用法；hostInfo 用一体字符串为规格指定（不引入结构体类型）。
