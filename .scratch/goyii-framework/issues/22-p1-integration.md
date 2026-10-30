# 22: P1 集成验证

Status: resolved

**What to build:** examples/basic 升级为 P1 完整体：templ 页面（Layout + Widget）、登录后按 RBAC 差异化导航、i18n 语言切换、分页列表页；docs/ 补齐 P0+P1 完整使用文档；README 正式化。

**Blocked by:** 15, 16, 17, 18, 19, 20, 21

- [x] 浏览器完整走通：登录 -> 列表（分页/排序）-> 详情 -> 编辑（验证 + Csrf）-> 权限差异可见 -> 切语言生效
- [x] docs/ 覆盖各组件使用说明与配置参考
- [x] README 正式化（定位、快速开始、文档索引、路线图）
- [x] P0+P1 全量测试一键跑通

## Comments

### 落地形态（main `b5e28b8`，分支 feat/p1-integration 已 ff 合并并删除）

- **examples/basic 浏览器面**（`views.go` + `views.templ` + `pages.go`）：
  - 应用级 Layout：全局中间件给每个请求 `SetLayout`，布局闭包捕获请求 Context——菜单（view.Menu 组件）按 RBAC 权限增删项（post/create 持有者见 New post，admin 层级持有者见 Delete 按钮）、会话区（登录态 + 登出表单带 _csrf 隐藏字段）、语言切换链接（当前路径 + ?lang=）；JSON 响应不渲染故布局休眠。
  - 页面集：首页（介绍 + 测试邮件按钮）、登录页（表单 + 422/401 回显）、列表页（ActiveDataProvider[Post] 下推分页排序 + LinkPager 保留 sort 参数 + 列头排序 toggle）、详情页（Breadcrumbs + DetailView 六行，日期经 i18n.F 格式化）、创建/编辑表单（zog 场景验证 + CSRF + 422 状态码回显——错误消息已由保存链路 flatten 边界翻译、输入值回填）、删除按钮（POST /posts/{id}/delete）。
  - **页面写操作权限分叉记录**：AccessControl 的 Paths 是精确匹配（无法表达 `/posts/{id}` 模式），POST /posts（create）与 POST /posts/{id}（update）动词相同也无法区分——页面写操作在 action 内用 `requirePermission`（guest 重定向登录、无权限 403 错误页）判定；controller 级 AccessControl 保留给 API 路径（动词可区分）。17 号的「AccessControl 经 AuthManager 判定」仍由 API 的 delete 规则演示。
  - i18n：`//go:embed messages` + `app.SetI18n(i18n.NewMessageSource(messages))` 一行接入；zh-CN/app.json 覆盖界面文案 + 全部验证消息（"标题长度至少为 3 个字符" 等）；语言切换经 ?lang= 写会话粘住。
  - seed：admin/editor/demo 三账号，RBAC 层级 admin -> editor -> post/create,post/update、admin -> post/delete，25 篇文章撑起三页分页；Post 内嵌 behaviors.Hooks + behaviors.Timestamp（新迁移 m..._000004 加 created_at/updated_at 列）。
  - mail 示例（21 号遗留接本票）：首页 Send a test email -> mail.NewMessage 链式构造 -> app.Mailer().Send -> log 驱动完整消息落 serve 日志（From/To/Subject/双正文）。
- **框架改动（集成发现的两处，均小修 + 回归测试）**：
  1. `web.Context.RenderStatus / RenderComponentStatus`（web/render.go）：422 表单回显/401 登录页重渲染需要带状态码渲染 HTML，此前只有隐式 200 路径。回归测试 `TestRenderStatusCarriesStatusCode`（web/render_test.go：422 + text/html + 布局装帧三者齐断言）。
  2. **migrate SQLite 方言时间类型映射缺陷**：dateTime/timestamp/date/time 原映射为 TEXT，glebarez/mattn 驱动按声明类型做 time.Time 转换，TEXT 声明导致 GORM 时间列读写往返断裂（Scan error: driver.Value string into *time.Time）——behaviors 测试手写 DDL 用 DATETIME 声明所以未暴露。修复：dateTime/timestamp -> DATETIME、date -> DATE、time -> TIME（值仍文本存储，仅声明名变化）；dialect_conformance_test 七处 sqlite 期望同步更新即回归钉子；examples 端到端走真实迁移 + AR 往返再证。
- **docs/**：组件指南 14 篇（base 与 config / web 路由与中间件 / session 与认证 / db 与 AR / validators / migrate / log / cache / rbac / i18n / dataprovider / behaviors / mail / view 与 templ）+ 配置参考（全 config 段落 + 默认值 + 环境变量对照）；安装.md（P1 包表 + 目录图）、快速上手.md（浏览器链路 walkthrough + p1-check）、docs/README.md 索引；README.md 正式化（定位 + 能力一览 + 真实 API 快速开始 + 文档索引 + 路线图一句带过 P2）。文档规范遵守：图表全 Mermaid、逻辑符号 && || == ->、无 ASCII 字符画。
- **scripts/p1-check.sh**（新增，Git Bash/Linux/macOS）：gofmt -> go vet -> `CGO_ENABLED=1 go test -race -count=1 ./...` -> 编译示例 -> 浏览器链路冒烟 55 断言（匿名浏览/布局/分页/排序含白名单 400/详情/404 错误页装帧/登录表单/editor 422 回显与 403/demo 无权限/admin 删除/i18n 切换与验证消息翻译/邮件 log 记录/API 共存/登出/日志可观察）。p0-check.sh 同步升级（seed 25 篇后创建 id 变 26，三处断言更新），31 断言保持全绿。
- **工程卫生**：golangci-lint 升 2.15.0（本机 2.13.1 在 go1.27 GOROOT 上 panic 的环境问题就此绕开），全仓 **0 issues**——存量 44 findings 清理：errcheck 34 项按项目约定收敛进 .golangci.yml exclusions（defer Close / stdout 写 / os.Remove 尽力而为调用；exclude-functions 按名匹配对跨包方法失效，改用 v2 exclusions.rules 文本规则）；unused 6 项删除（migrate discardLogger/writerLogger、测试残留 helper、本票引入的未用常量）；staticcheck 3 项（error.go 改 tagged switch；optimisticlock 的 Statement.DB 选择器语义故意、加 nolint 注明；application_test 的 Default()!=Default() 改两变量比较）。

### 验收证据（合并后 main `b5e28b8` 上，CGO 经 scoop mingw gcc 启用）

- `bash scripts/p1-check.sh` -> **55 passed, 0 failed, P1 checks green**：gofmt 无文件、go vet 无输出、`go test -race -count=1 ./...` 22 包全绿（无 data race）、示例编译通过。冒烟覆盖验收链路全项：登录（表单 + CSRF，错 token 400）-> 列表（默认 id DESC、LinkPager、`?page=2` 取到末段行、`?sort=title` 字母序、白名单外 sort 400）-> 详情（breadcrumbs/detail-view/作者预加载/时间列）-> 编辑（空标题 422 回显 value 回填 + 双字段消息、合法提交 302、编辑表单 action 指向行 save 路由、坏 CSRF 400）-> 权限差异（editor 见 New post/Edit 无 Delete 按钮、demo 全无、admin 全有且删除 302 后 404、editor 删 403）-> 切语言（zh 主页/导航/总数行/状态词/验证消息全翻译、会话粘住、切回英文）-> 邮件（notify 200 + serve 日志含完整消息）-> API 共存（/post 200 JSON）-> 登出（302 + guest:true）-> runtime/app.log 含 [db]/[cache]。
- `bash scripts/p0-check.sh` -> **31 passed, 0 failed, P0 checks green**（P0 API 面回归：约定路由/CSRF/415/405/授权分档/422/201+Location/缓存 MISS-HIT/404 协商/日志）。
- `go test ./...` 全仓 22 包 1196 项全绿；`golangci-lint run ./...`（2.15.0）**0 issues**；`gofmt -l .` 无输出。
- 流程：claimed 单独提交 `a9f7985` -> feat/p1-integration 上 `b5e28b8` -> main `--ff-only` 合并并删除任务分支。

### 集成观察（非缺陷，记档备查）

1. **登录响应与会话落盘的微竞态**：会话记录在中间件收尾（defer finish）持久化，登录 action 的 302 字节可能先于落盘到达客户端——紧邻的下一个请求（curl 回环背靠背）偶见 guest 视图，浏览器场景（人手点击/页面跳转间隔）不触发。p1-check 的 login 助手以 0.3s 间隔规避。如需彻底消除可考虑把会话保存提前到首字节写出前（接口变更，未做）。
2. AccessControl 的 Paths 为精确匹配（本票页面写操作改走 action 内判定即因此），如后续需要模式级规则可考虑扩展（接口变更，未做）。
3. templ 文本插值只写字符串（int 先格式化）——本票页面数据结构全部预格式化为 string，规避非字符串插值的行为差异。
