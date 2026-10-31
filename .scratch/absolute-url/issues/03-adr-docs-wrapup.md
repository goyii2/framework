# 03: ADR 0007 与文档收尾

Status: resolved

**What to build:** 把 absolute-url 的决策落纸面。`dev-docs/adr/0007-*.md`：主机信息取**仅 boot 配置**来源的决策——绝对网址的真实消费者（邮件渲染、队列任务、OAuth 回调）全部运行在无请求上下文，配置来源恰好覆盖；明确**不做请求推断**（不从当前请求的 Host / X-Forwarded-Proto 推导）及理由（需要跟随当前访问域名的场景用相对 URL 即可）；两层 API 分层（`AbsoluteURL` 显性 error vs `ToAbsolute` 吞错返回 `"#"`，复刻 `URL`/`To` 分层）与 `SetHostInfo` 非法值 fail-fast 的理由。`docs/组件-web路由与中间件.md` 在反向生成小节后增补绝对生成用法（装配接线 + `ToAbsolute` 示例 + 未配置行为）。`CONTEXT.md` 术语「主机信息（hostInfo）」已先行入库，本票核对表述与实现一致即可，不重复添加。

**Blocked by:** 01, 02

- [x] ADR 0007 落盘：来源决策、不做请求推断的边界、API 分层、fail-fast 理由，含被否备选
- [x] `docs/组件-web路由与中间件.md` 增补绝对生成小节（装配 + 用法 + 失败行为）
- [x] CONTEXT.md「主机信息」词条与实现行为核对一致
- [x] 文档不与实现矛盾：API 名、签名、失败标记逐一对得上

## Comments

- 2026-10-31 实施（commit 00a9e8a，feat/absolute-url-03 已 --ff-only 合回 main 并删分支）：新增 `dev-docs/adr/0007-absolute-url-host-info-from-boot-config.md`（§1 仅 boot 配置来源＋消费者无请求上下文论证；§2 不做请求推断的四条理由＋相对 URL 替代；§3 两层 API 分层表（URL/To × AbsoluteURL/ToAbsolute）＋装配形态；§4 非法值 fail-fast 与 ADR-0005 boot 期暴露原则的延续；Considered Options 四组被否备选：来源三选一、API 形态四选一、非法值处理、值形状）。`docs/组件-web路由与中间件.md` 反向生成小节后新增「绝对网址生成」：装配接线（`url.SetDefault` 后接 `SetHostInfo`）+ `ToAbsolute` 示例 + 失败行为（panic 条件、`"#"` 标记、`AbsoluteURL` 显性 error）+ 可跑示例指引。CONTEXT.md 词条核对一致未改动。
- 验收证据（文档 vs 实现逐项核对）：`SetHostInfo(raw string)` 一体字符串、尾斜杠归一（url.go TrimRight）、非法值 panic 条件逐项对上（缺 scheme/host、userinfo、多余路径、query、fragment，消息含 boot-time wiring mistake）；`AbsoluteURL(name, params) (string, error)` 复用 `URL` 生成路径、未配置显性 error、缺路径参数 error 透传；包级 `ToAbsolute(name, params) string` 经 Default 吞错返回 `"#"`；`To`/`URL` 未动；装配形态与 examples/basic main.go:182/187、examples/advanced main.go:98/99 一致；yaml key `hostInfo` 与两示例 config.go/app.yaml 一致；docs 示例输出 `https://example.com/post/7` 与该页运行示例 pattern `/post/{id:[0-9]+}`（第 25 行）一致。基线全量 `CGO_ENABLED=1 go test -race -count=1 ./...` 37 包全部 ok（文档票无代码改动，测试确认基线绿）。
- 双轴自审（编码规范＋规格符合）：规范轴——ADR 落 dev-docs/adr/（非 docs/adr/）、编号 0007 递增、结构与 0005/0006 一致、无 ASCII 图表（无需图）、docs/ 增补属使用文档；规格轴——四条验收标准逐条满足，无超范围改动（spec.md、CONTEXT.md、代码均未动）。自审修正一处：ADR 装配表述「之后一行」改为「之后紧接调用」（basic 两调用间隔一段注释，advanced 相邻两行）。
- 待人工：无。
