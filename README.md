# goyii/framework

用 Go 重写的 Yii2 风格 Web 框架：保留 Yii2 的架构概念与心智模型（组件化 Application、ActiveRecord、约定式路由、Filter 族中间件、RBAC、i18n），以 Go 惯用法落地——接口、泛型与显式注册，替代 PHP 的动态挂接与数组配置实例化。享受 Go 的类型安全、并发模型与单二进制部署。

```go
cfg, _ := config.Load("config")                 // 值走 yaml + env，行为走代码组装
app := base.New(cfg)
base.SetDefault(app)

h := web.NewHandler(app)
h.Get("/post/{id:[0-9]+}", postCtrl.ActionView).Name("post/view")
url.SetDefault(h.URLs())
url.To("post/view", url.P{"id": 1})              // -> /post/1（命名路由 + 约定拼装兜底）

type PostController struct{ web.Controller }
func (c *PostController) ActionView(ctx *web.Context) error {
    p, err := ar.FindByID[Post](ctx, ctx.ParamInt("id"))
    if err != nil { return web.NotFound("post not found") }
    return ctx.Render("post/view", PostViewData{Post: p})   // templ 视图注册表
}
```

## 能力一览（P0 - P2）

- **web**：显式路由 + `/{controller}/{action}` 约定回退、AccessControl / VerbFilter / Csrf / ContentNegotiator / Cors / RateLimiter / HttpCache / **PageCache** / HttpBasic / HttpBearer 中间件、统一错误处理（JSON/HTML 协商）、会话与登录态、**嵌套 Module**（前缀拼接 + 模块级中间件）
- **db/ar**：GORM 之上的泛型 ActiveRecord（查询链、Save 自动按场景验证、事务、钩子）
- **migrate**：yiimigrate 并入的版本化迁移（八命令、四方言、dry-run、历史表）
- **validators**：场景 -> zog schema 映射，错误形态 `字段 -> []消息`
- **rbac**：DbManager 四表实现（角色/权限 DAG、规则代码定义、item 图缓存）与 AccessControl/导航联动
- **i18n**：JSON 消息源 go:embed 单二进制、语言协商链（参数 > 会话 > Accept-Language > 默认）、x/text 格式化
- **view**：templ 视图注册表 + Layout 组合 + Widget（LinkPager / Breadcrumbs / Menu / DetailView / **ActiveForm / GridView / ListView**）
- **dataprovider**：Active/Array DataProvider，`page` / `per-page` / `sort=-created_at,id` 参数与白名单防注入
- **behaviors**：Timestamp / Blameable / Sluggable / OptimisticLock（结构体内嵌 + Hooks 分发）
- **queue / mutex**：异步队列（Job 注册表 + JSON 载荷、sync/db/redis 驱动、listen/run/retry worker、退避重试与 TTR 回炉）、按名互斥锁（process/redis，TTL 自释放）
- **debug / gii / vite / authclient**：开发期请求内省面板（七面板 + toolbar 注入）、model/controller/crud 代码生成器、Vite manifest 解析与 dev 反向代理（HMR）、GitHub/Google OAuth2 登录
- **log / cache / mail / console / helpers**：slog 多 Target、memory/file/redis + 依赖失效、log/SMTP 邮件器、cobra 命令树、惯用帮助函数

## 快速开始

```bash
git clone https://github.com/goyii2/framework.git
cd framework/examples/basic

go run . migrate up   # SQLite 零依赖建表（含 RBAC 四表）
go run . seed         # admin/editor/demo 三账号 + RBAC 层级 + 25 篇文章
go run . serve        # http://127.0.0.1:8090
```

浏览器完整走通：登录 -> GridView 过滤/排序/分页列表 -> 详情 -> ActiveForm 编辑（验证 + CSRF + 422 回显）-> 按账号差异化的导航与按钮 -> 中英文切换（界面与验证消息）-> 测试邮件 -> 队列入队与 worker 消费 -> 整页缓存命中回放 -> 页底调试面板。全部细节见 [docs/快速上手](docs/快速上手.md)。

advanced 模板示例（Yii2 高级模板形态：单进程单端口，frontend=根作用域、backend=`/backend` 模块、console=同一二进制命令、common=共享包、environments=overlay）：

```bash
cd framework/examples/advanced

go run . migrate up   # user + post + RBAC 四表
go run . seed         # admin/admin123（admin 角色）+ demo/demo123（无角色）+ 3 篇文章
go run . serve        # http://127.0.0.1:8090（前台根作用域 + /backend 后台模块）
```

前台匿名浏览 `/posts`（仅 published）、`/login` 登录；后台 `/backend` 守卫链演示三态（匿名 302 -> 登录页、demo 403、admin 200 进深色后台做 post CRUD）；嵌套子模块 `/backend/system/user/list` 约定路由 + DataProvider 分页；`GOYII_ENV=prod` + `web-prod.yaml` 的 `${VAR}` 插值演示生产配置。全部细节见 [docs/示例-advanced模板](docs/示例-advanced模板.md)。

一键验收（含 `-race` 全量测试与组件面冒烟）：

```bash
./scripts/p2-check.sh   # P2 组件面（Vite/debug/GridView/ActiveForm/嵌套模块/PageCache/queue）
./scripts/p1-check.sh   # P1 页面链路
./scripts/p0-check.sh   # P0 API 面
```

## 文档

- [安装](docs/安装.md) —— 环境与包对照表
- [快速上手](docs/快速上手.md) —— 从零跑通 examples/basic
- [示例：advanced 模板](docs/示例-advanced模板.md) —— 应用分区与 Module：examples/advanced
- [配置参考](docs/配置参考.md) —— 全部配置段落与环境变量
- 组件使用说明：[base 与 config](docs/组件-base与config.md) | [web 路由与中间件](docs/组件-web路由与中间件.md) | [session 与认证](docs/组件-session与认证.md) | [db 与 ActiveRecord](docs/组件-db与ActiveRecord.md) | [validators](docs/组件-validators.md) | [migrate](docs/组件-migrate.md) | [log](docs/组件-log.md) | [cache](docs/组件-cache.md) | [rbac](docs/组件-rbac.md) | [i18n](docs/组件-i18n.md) | [dataprovider](docs/组件-dataprovider.md) | [behaviors](docs/组件-behaviors.md) | [mail](docs/组件-mail.md) | [view 与 templ](docs/组件-view与templ.md) | [debug 面板](docs/组件-debug面板.md) | [gii 代码生成器](docs/组件-gii代码生成器.md) | [vite 前端构建](docs/组件-vite.md) | [authclient 第三方登录](docs/组件-authclient第三方登录.md)

## 路线图

P0 核心骨架、P1 完整框架体验与 P2 生态（queue、mutex、调试面板、Gii 代码生成、Vite 集成、GridView/ListView/ActiveForm 大件、PageCache、嵌套 Module、authclient）已交付；后续分期见 [dev-docs/PRD.md](dev-docs/PRD.md)。

## 更多

- [dev-docs/PRD.md](dev-docs/PRD.md) —— 需求与工程决策
- [dev-docs/adr/](dev-docs/adr/) —— 决策记录
- [CONTEXT.md](CONTEXT.md) —— 术语表
