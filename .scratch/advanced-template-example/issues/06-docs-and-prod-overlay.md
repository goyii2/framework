# 06: 文档、prod overlay 与收尾验证

Status: ready-for-agent
Blocked by: 04（Post 全链路）、05（/backend/system 用户管理）

**What to build:** 收尾：新增 `docs/示例-advanced模板.md` 讲清映射关系（frontend↔根作用域、backend↔Module、console↔命令、common↔共享包、environments↔overlay）及与 Yii2 高级模板的差异（独立登录 → 单一登录 + RBAC 的理由）；README 快速上手补 advanced 一节；web-prod.yaml 以 `${VAR}` 插值演示生产配置；全量检查收口。

- [ ] `docs/示例-advanced模板.md` 覆盖映射表 + 差异说明 + 运行步骤，与实际行为一致
- [ ] README 快速上手含 advanced 示例运行步骤（migrate up / seed / serve 与访问入口）
- [ ] web-prod.yaml 至少一处值经 `${VAR}` 环境变量插值，文档说明验证方式（如 GOYII_ENV=prod + 设变量启动）
- [ ] 冒烟：`migrate up && seed && serve` 全流程手动走通（前台浏览、登录、后台 CRUD、system 分页）
- [ ] go build ./... && go test ./... 全绿，gofmt/golangci-lint 干净

## Comments
