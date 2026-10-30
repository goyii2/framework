# 36: P2 集成验收

Status: resolved

**What to build:** P2 端到端收口（对齐 P0/P1 集成票先例）：examples/basic 增嵌套 admin demo、queue demo（db 驱动 seed + drain）、PageCache 演示页、列表/表单页重构为 GridView/ListView/ActiveForm、debug 面板开启、最小 `web/` 前端工程（构建产物 manifest.json 与 hash 资源入库，head 经 vite.Entry 引用，Node 仅构建期需要）；docs/ 补齐 P2 组件使用文档；one-shot 校验全绿；resolve 23 号票。

**Blocked by:** 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35

- [x] examples/basic：嵌套 admin demo、queue demo、PageCache 页、三件套重构、debug 面板——全部可浏览器访问验收
- [x] Vite 最小前端工程：构建脚本与产物入库，head 引用生效
- [x] docs/ 覆盖 queue/mutex/PageCache/大件/debug/Gii/Vite/嵌套模块（authclient 教程随 35）
- [x] one-shot 校验脚本全绿（对齐 15/22 先例）
- [x] 23 号票 resolve（共识、wontfix、拆解三项均完成）

## Comments

### 落地形态（main `55d68b5`，分支 feat/36-p2-integration 已 ff 合并并删除）

- **queue demo**：config `queue: {driver: db, tableName: queue}`；迁移清单并入 `queue.NewMigration("queue")`；DSN 加 `_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)`——serve 与 worker 两进程共享同一 SQLite 文件不锁死。`jobs.go` 定义 `EmailJob`（Name+Execute，经 mailer log 驱动投递）+ 队列表状态读取（waiting/reserved/failed 直读 `SELECT status, COUNT(*)`）。seed 在 RBAC 后植入 3 个演示邮件 Job（表空才植，幂等）；`/queue` 页（导航可达）显示三行计数 + Enqueue a demo email 按钮（POST /queue/push -> PRG 回 `?pushed=N` 回显）；`goyii queue run`/`listen` 消费——验收中 serve 运行期间独立 worker 进程单轮消费成功。
- **PageCache 演示页**：`/pagecache`（CacheController 控制器级 `web.PageCache{Duration: 3s, VaryBy: 语言+用户id}`）。页面带渲染时间戳：TTL 内二次请求回放缓存页（时间戳逐字节一致）、过期后重渲染、admin 与 guest 变体分离（VaryBy）；与 debug 面板层级=采集外/缓存内：缓存条目无 toolbar、每次回放拿当次新 toolbar。导航新增 Queue / Page cache 入口，两页均 i18n（zh-CN 全量翻译）。
- **列表/表单三件套重构**：`/posts` 换 `view.GridView`（id/title/status/created_at 可排序白名单列头升降切换 + title contains 过滤（LIKE 全转义）+ status 下拉过滤行 + author/actions 列，actions 的 Edit 按 RBAC 显隐），`LinkPager` 翻页与排序链接均保留过滤参数；创建/编辑表单换 `view.ActiveForm`（title maxlength200/required + status select + body textarea，422 回显 validators 消息 + 值回填 + has-error 状态组）。登录页保留手写表单（对照演示：inline zog schema vs 模型 validators 两条路径）。
- **debug 面板**：`debug.New(app).Mount(handler)` 挂在 NewHandler 后、应用自有全局中间件前（App.Debug=true 生效）：页面底部 toolbar、`/debug` 历史 + `/debug/view?id=` 详情、记录落 `runtime/debug/`（启动时建目录）。
- **Vite 前端工程**：`examples/basic/web/`（vite 7.3.7 + package-lock 入库，`node_modules/` 进 .gitignore）。`src/main.ts`（接管 #vite-demo 按钮计数）+ `src/main.css`；**真实构建产物入库**：`dist/.vite/manifest.json` + `dist/assets/main-DRrTpMbW.js` + `main-B8dDBM3K.css`。Go 侧 `mountFrontend`：启动即 `vite.Entry("src/main.ts")`（错名/坏 manifest 直接 fail-fast），layout `<head>` 渲染 script/link；build 模式 `distAssets` 从 `web/dist/assets` 服务 `/assets/*`（200 + 正确 Content-Type），dev 模式（manifest 缺失或 Dev 开关）挂 `DevProxy("/assets")`——**Node 仅构建期需要，Go 侧运行零依赖**。两种模式说明：build 模式为默认与验收路径；dev 模式删 manifest 重启即切（页面指向 localhost:5173 + @vite/client，/assets 经代理含 WS 透传）。
- **嵌套 admin demo 核对（缺则补）**：核对发现 main 上 `/admin/panel/index`（约定路由）对匿名返回的是**首页 200**而非 403——根因是框架级缺陷：`Handle` 把路径恰为 `/` 的绑定注册成 ServeMux 子树 catch-all（"GET /"），吞掉一切未匹配 GET，约定回退（及其模块中间件）对 GET 整体失效（`/site/index` 同样从未真正走到约定解析，此前 p0-check 只断言 200 而被首页蒙混）。修复：`web/handler.go` 将根路径注册改写为 `"/{$}"`（仅精确匹配根），回归测试 `TestRootRouteBindsOnlyTheRoot`（根命中/约定路由不被吞/模块约定路由可达/未知路径 404/POST 根 405+Allow）。修复后 admin demo 完整：匿名 403、admin 登录 200 返回模块 JSON 与反向 URL（`/admin/report/dashboard` 显式路由本就正常）。
- **框架修复 2（debug 面板挂载暴露）**：面板 holdWriter 此前持有**私有 header 副本**，错误路径丢弃持有字节时把动作先设置的响应头一并丢掉（p0-check 的 `Allow: GET, HEAD` 断言抓到）——改为 Header() 直通真实响应头映射（持有期间无字节上线，语义等价；错误路径由统一渲染器经还原后的 writer 作答，Allow/Location 得以保留），回归测试 `TestErrorResponsesKeepHeadersSetBeforeTheFailure`。
- **docs/**：queue/mutex/嵌套模块（base与config）、大件（view与templ）、debug/Gii/Vite/authclient（独立篇）均已随各票落地，本票补缺：`组件-web路由与中间件.md` 增 PageCache 小节（配置形态、参与条件、key 构成、与 debug 面板层级、示例指针）；`组件-base与config.md` queue 节补可跑示例指针；`快速上手.md` 重写为 P0–P2（文件表 + 「P2 生态走一遍」新节：队列/调试面板/整页缓存/嵌套模块/前端两种模式 + 一键验收三脚本）；根 `README.md` 能力一览升 P2、文档索引补 18 篇全量、路线图更新。`docs/README.md` 索引核对完整无缺。
- **one-shot**：新增 `scripts/p2-check.sh`（对齐 15/22 先例形态：gofmt -> go vet -> `go test -race -count=1 ./...` -> 示例编译 -> Vite 产物在库核对 -> migrate/seed/queue run 单轮 -> 起服冒烟）；`p1-check.sh` 适配 GridView/ActiveForm 新标记（搜索框断言换过滤行、q= 搜索链路换 title=/status= 过滤链路，正文关键词断言随过滤语义移除）；`p0-check.sh` 未改（API 面无变化，根路由修复后 /site/index 首次真正走约定解析仍 200、POST 405+Allow 依旧）。

### 验收证据（合并后 main `55d68b5` 上，CGO 经 scoop mingw gcc 启用）

- `bash scripts/p2-check.sh` -> **49 passed, 0 failed, P2 checks green**：全量 `-race` 30 包全绿、示例编译、manifest+hash 资产在库；`migrate up` 含 create_queue_table；`seed` 输出 `queued 3 demo email job(s)`；`queue run` 输出 `processed 3 message(s)` 且含 `Subject: Welcome to the goyii basic queue`（worker 投递）。冒烟覆盖：页面 head 引用 `/assets/main-DRrTpMbW.js`（module script）与 `/assets/main-B8dDBM3K.css`（stylesheet）、hash 资源 GET 200 `text/javascript`；toolbar 注入、`/debug` 历史 200、详情 200 含 Route pattern；GridView（grid-view 表、filters 行、title 输入、status 下拉、sort=title 链接、pagination、Total 25）、过滤收窄（title=Sample&status=draft -> 6 且回显、排序链接保留过滤）、LIKE 转义（`%uns_matchable_%` -> 0 + 空态文案）、白名单外 sort 400、editor 行内 Edit；ActiveForm 422（值回填 + 双字段消息 + has-error）；admin 嵌套（匿名 /admin/panel/index 与 /admin/report/dashboard 均 403、admin 登录 200 模块 JSON）；PageCache（二次请求时间戳逐字节相同、回放仍带新 toolbar、admin 变体时间戳不同、3.5s 后过期重渲染）；队列页（零计数 -> push 302 `?pushed=4` -> 回显 waiting for a worker + `<td>1</td>` -> serve 运行中独立 `queue run` `processed 1 message` 且 `To: smoke@example.com`）；停服后 app.log 含 [db]。
- `bash scripts/p1-check.sh` -> **69 passed, 0 failed, P1 checks green**（浏览器链路在新标记下回归：分页/排序/过滤、422 回显、RBAC 三账号差异、zh 翻译含过滤行空态与列头、邮件、API 共存、登出、日志）。
- `bash scripts/p0-check.sh` -> **31 passed, 0 failed, P0 checks green**（含根路由修复后 `/site/index` 约定路由与 VerbFilter 405+Allow——后者即暴露 debug holdWriter 头丢失的断言）。
- `go test ./...` 30 包全绿；`golangci-lint run ./...`（2.15.0）**0 issues**；`gofmt -l .` 无输出。
- 流程：claimed 单独提交 `dbe039f` -> feat/36-p2-integration 上 `55d68b5` -> main `--ff-only` 合并并删除任务分支。

### 23 号票核对

23（P2 规划拷问）已 resolved：共识 12 项回写 PRD「P2 生态」节、wontfix 四项（httpclient/mongo+es AR/Widget 注册表/Casbin 适配）落盘、拆解 25–36 工单三项均完成（见 23 号票 Comments 的 2026-10-29 拆解落盘记录）——本票无需动作，仅引用确认。

### 集成观察与待人工项

1. **浏览器级自动化已覆盖**：全部验收经 HTTP 断言（状态码/内容/时间戳/命令输出）自动化，见上。
2. **待人工清单（真机浏览器目验）**：a) Vite 按钮交互（bundle 加载后 `Vite bundle loaded — click me` 与计数，需浏览器执行 JS）；b) 真机 HMR 冒烟（`cd web && npm run dev` + 移走 manifest 重启 Go 应用，改 src/main.ts 热更新生效）；c) debug 面板七面板的人眼走查（/debug 详情页布局）。三项均不阻塞本票。
3. **根路由语义修复的影响面**：`/` 绑定从「子树 catch-all」改为「仅根精确匹配」是行为修正——任何依赖「未匹配 GET 落到根动作」的应用其实是依赖了约定回退被饿死的缺陷；web 包既有测试与三个 one-shot 全绿佐证无回归。
