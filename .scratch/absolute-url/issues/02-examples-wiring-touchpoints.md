# 02: examples 装配接线与可见触点

Status: claimed

**What to build:** basic 与 advanced 两个示例在装配期接上主机信息：`main.go` 里 `url.SetDefault(handler.URLs())` 之后按项目配置调用 `SetHostInfo`（值来源遵循 ADR 0005 的「项目自有配置 + 代码装配」——示例自定取值方式，如环境变量或配置 overlay，不硬编码魔法值）。每个示例落一处**可见触点**：用 `url.ToAbsolute` 渲染完整网址进入页面输出（例如文章页的分享链接或 canonical 链接），让新能力不是只接线不使用。示例行为不回归：既有页面、重定向、表单 action 全部保持相对 URL 原样（只有触点用绝对生成）。

**Blocked by:** 01

- [ ] basic 装配期 `SetHostInfo`（值来自项目配置，非硬编码）
- [ ] advanced 装配期 `SetHostInfo`（同上）
- [ ] 每示例一处可见触点：页面渲染输出含 `scheme://host/path` 形态、由 `url.ToAbsolute` 生成的链接
- [ ] 既有页面/重定向/表单 action 保持相对 URL 不变（触点除外）
- [ ] 两个示例构建通过，全量测试绿；触点渲染可自动化验证的部分自动化（如视图函数直调断言），纯目视项标注「待人工」
