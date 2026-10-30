# 06: 文档、prod overlay 与收尾验证

Status: resolved
Blocked by: 04（Post 全链路）、05（/backend/system 用户管理）

**What to build:** 收尾：新增 `docs/示例-advanced模板.md` 讲清映射关系（frontend↔根作用域、backend↔Module、console↔命令、common↔共享包、environments↔overlay）及与 Yii2 高级模板的差异（独立登录 → 单一登录 + RBAC 的理由）；README 快速上手补 advanced 一节；web-prod.yaml 以 `${VAR}` 插值演示生产配置；全量检查收口。

- [x] `docs/示例-advanced模板.md` 覆盖映射表 + 差异说明 + 运行步骤，与实际行为一致
- [x] README 快速上手含 advanced 示例运行步骤（migrate up / seed / serve 与访问入口）
- [x] web-prod.yaml 至少一处值经 `${VAR}` 环境变量插值，文档说明验证方式（如 GOYII_ENV=prod + 设变量启动）
- [x] 冒烟：`migrate up && seed && serve` 全流程手动走通（前台浏览、登录、后台 CRUD、system 分页）
- [x] go build ./... && go test ./... 全绿，gofmt/golangci-lint 干净

## Comments

### 验收证据（2026-10-31）

**文档清单**

- 新增 `docs/示例-advanced模板.md`：目录结构与分区装配表、mermaid 拓扑图、Yii2 高级模板映射表、独立登录 -> 单一登录 + RBAC 差异与理由（授权唯一真源为 RBAC 分配表，user 表无 role 列）、/backend 三态表、前后台走一遍、dev/prod overlay 与 `${VAR}` 插值验证、与 basic 的选择对照
- `README.md` 快速开始补 advanced 一节（migrate up / seed / serve + 三个访问入口 + 三态概述），文档列表补链接
- `docs/README.md` 文档索引补条目；`docs/配置参考.md` 的示例对照表补 web-prod.yaml 行
- 运行步骤逐条与实际行为对照过；冒烟中发现并修正三处文档失真：首页状态行实为 `env: dev` + `debug on`（prod 只渲染 `env: prod`）、后台列表链接文案为 "View site"、debug 差异在真实 500 而非 404（HTTP 错误两环境都带自身消息）

**web-prod.yaml + 插值**

- `examples/advanced/config/web-prod.yaml`：`listen: "${ADVANCED_LISTEN:-127.0.0.1:8090}"`、`level: "${ADVANCED_LOG_LEVEL:-warn}"` 两处插值，默认值保证 prod 开箱即跑
- config_test.go：原 no-overlay 用例改用 staging（prod 现在有 overlay），新增两个用例钉住插值默认值回退与变量注入；顺带修正 web-dev.yaml 里"prod overlay 后续切片加入"的过期注释
- 实机验证：`ADVANCED_LISTEN="127.0.0.1:9090" GOYII_ENV=prod go run . serve` -> 9090 应答 200、8090 拒连，首页状态行 `env: prod`（无 debug 行）；dev 下同页为 `env: dev` + `debug on`

**冒烟（examples/advanced，删 runtime 后从零）**

- `go run . migrate up` -> 3 个迁移应用（user / auth 四表 / post）；`go run . seed` -> admin/demo 建号 + `rbac: admin role granted to admin` + 3 篇文章；`user/create` x9 建到 11 个用户
- 前台：`/` 200、`/posts` 仅 2 篇 published（draft 不出现）、`/posts/3`（草稿）与 `/posts/999` 均 404、错密码 401、demo 登录 302 回首页且出现 "Signed in as"
- 后台三态：匿名 `/backend/` 302 -> /login；demo 403；admin 200（深色 frame 命中 "backend partition"）。裸 `/backend` 302 -> `/backend/`
- CRUD：`/backend/posts` 含草稿行 + View site 链接（href=/posts/1、/posts/2）；非法提交 422 双字段消息；创建 302 后前后台列表都出现 "Smoke post"；改 draft 后前台 `/posts/4` 404；删除 302 后行消失
- system：11 用户下 `/backend/system/user/list` 200（user9/user8/admin 在页，分页条含 page=2）；`?page=2` 200 只剩 id=1 行；`?sort=-username` 200、`?sort=password` 400（白名单外）；`?per-page=5` 200
- serve 进程已停（dev 与 prod 各起停一次）

**全量检查**

- `go build ./...` 通过；`go test ./...` 全绿（0 失败）；`gofmt -l` 无输出；`golangci-lint run` 0 issues
- lint 修复：ticket 04/05 遗留的 5 处 ineffassign（advanced 的 post_test.go x3、frontend_test.go x2，未用返回值改 `_`）

