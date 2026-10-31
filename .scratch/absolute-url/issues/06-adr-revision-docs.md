# 06: ADR 0007 修订 + docs 更新（builder 形态）

Status: resolved

**What to build:** 把 builder 取代两函数形态的决策写回纸面。修订 `dev-docs/adr/0007`：API 小节改为 builder 形态（`Build`/`String`/`Absolute`/`Host`），注明取代理由——链式调用是 Yii2 `Url::to($url, $scheme)` 可选参数语义的 Go 表达，「选择在调用点、值在配置」同时化解了显式性与重复的两难；被否备选补记三组：`WithHostInfo` 派生视图（被 `.Host()` 取代，机器更少）、调用点裸拼 host（本分区绝对链接每处重复部署知识、无单一校验点）、`To` 改返 builder（50+ 调用点机械扰动）。`docs/组件-web路由与中间件.md` 绝对生成小节同步为 builder 用法（含双域名 `.Host(...)` 示例与失败行为）。CONTEXT.md「主机信息」词条核对（语义不变：管理器上的默认域，builder 下为 `.Absolute()` 的缺省前缀）。

**Blocked by:** 04, 05

- [x] ADR 0007 API 小节改 builder 形态，取代理由与三组被否备选齐备
- [x] docs 绝对生成小节为 builder 用法，含双域名示例
- [x] 文档与实现逐一对得上：方法名、失败标记（`"#"`）、配置键（`hostInfo` / `backendHostInfo` / env 名）
- [x] CONTEXT.md 词条与实现一致，无重复添加

## Comments

- 2026-10-31 实施（commits deb6b62 + 2e9ce3a，feat/absolute-url-06 已 --ff-only 合回 main 并删分支）：ADR 0007 §3 重写为「API：链式 builder（Build / String / Absolute / Host）」——三方法语义（String 等价 To 失败 `"#"`；Absolute 任何失败含未配置/显式域非法/缺路径参数 `"#"` 不 panic；Host 本次生成覆盖、`.Absolute()` 时校验、空串清除）＋取代理由（Yii2 `Url::to($url, $scheme)` 可选参数的 Go 表达：选择在调用点、值在配置，化解显式性与重复两难）＋首版两函数形态的取代说明；§4 补 `normalizeHostInfo` 双语境共用一句；「后果」正负两面改写（默认域单值 + `.Host` 显式域、`Absolute()` 吞错兜底链）；新增「落地中的再决策」三条（builder 取代、Host 校验时机、跨分区域装配注入）沿用 0005/0006 修订体例；Considered Options「API 形态」选定项改 builder 链式、两函数分层标 superseded，并补「builder 形态被否备选三组」（`WithHostInfo` 派生视图 / 调用点裸拼 host / `To` 改返 builder）。docs/组件-web路由与中间件.md 绝对生成小节：示例改 `Build(...).Absolute()` / `.String()` / `.Host(backendHost).Absolute()` 三链（双域名示例输出 `https://backend.example.com/backend/posts/7/edit`），失败行为与 `.Host` 语义、配置键与 env 名、可跑示例指引各一条；删除已失效的 `AbsoluteURL` 公共 API 指引（04 已收窄为 unexported）。CONTEXT.md「主机信息」词条原位微调为「管理器上的默认域……builder 的 `.Absolute()` 未用 `.Host(...)` 显式指定域时以其为缺省前缀」，未新增条目。docs/示例-advanced模板.md 的 Register 签名核对与 advanced main.go:126 一致（05 已同步，本票未动）。
- 验收证据（文档 vs 实现逐项核对）：方法名与行为对 `url/url.go`——`Build`（L260，经 Default 与 To 同源）、`String`（L278，吞错 `"#"`）、`Absolute`（L292，三失败源皆 `"#"`、无 panic 路径）、`Host`（L270，只记值、空串清除）、`normalizeHostInfo`（L74，SetHostInfo boot 期 panic 与 builder 请求期 `"#"` 共用，SetHostInfo panic 消息含 "boot-time wiring mistake"）；配置键对示例——yaml `hostInfo`（examples/basic/config/app.yaml:20、examples/advanced/config/app.yaml:23）、`backendHostInfo`（examples/advanced/config/app.yaml:30、config.go:45 yaml tag）；env 名按 loader 整字段规则（config/load.go:149 yaml 键路径大写下划线拼接）为 `WEB_HOSTINFO` / `WEB_BACKENDHOSTINFO`——05 号 Comments 记录的规格偏差已作为实名录入，工单正文未采用的 `WEB_BACKEND_HOSTINFO` 全仓 grep 零出现；装配注入链对 advanced main.go:98-99（SetDefault 后紧接 SetHostInfo）、main.go:126 -> backend.go:96（`PostController{BackendHost: backendHost}`）-> post.go:192（`.Host(pc.BackendHost).Absolute()`）；双域名示例输出与 backend 路由 `/backend/posts/{id:[0-9]+}/edit`（名字 `backend/post/edit`）一致，且被 TestBackendPostEditRendersPermalink 钉住；ADR「未配置 == error」的内部断言对 `url/url_internal_test.go`。全仓 md 无 ToAbsolute/AbsoluteURL 残留（除 ADR 0007 内记录取代史的三处，属有意保留）。`scripts/p0-check.sh` 全绿（"P0 checks green."）：gofmt clean、go vet clean、`CGO_ENABLED=1 go test -race -count=1 ./...` 全部 ok、basic 冒烟 31/31。
- 双轴自审（编码规范＋规格符合）：规范轴——ADR 修订落 dev-docs/adr/ 且沿用 0005/0006 的「落地中的再决策」体例；无 ASCII 图表、无数学逻辑符号（仅 `->`）；ADR 内 ADR-0005 交叉引用与「第二轮工单」泛指沿仓库先例（0005 用「01/02 过渡期」、0006 用「05 票先例」），代码注释规范不约束 dev-docs；docs/ 仅动使用文档。规格轴——四条验收逐条满足；改动仅三个交付物文件＋本工单文件，无代码改动、无规格文档擅改之外的范围蔓延。判断性备注一处：非法值形状清单在 ADR §4 / docs 要点 / 代码注释三处复述，属决策记录与使用文档各自面向读者的重述，非重复代码异味。
- 待人工：无。
