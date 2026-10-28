# 01: 仓库骨架

**Status:** claimed

**What to build:** goyii/framework 的 Go module 骨架：可构建、可 lint 的空包布局，包名对应 Yii2 命名空间（base/web/db/log/cache/rbac/i18n/console/helpers/validators/migrate/dataprovider），后续所有工单在此之上落地。module path 在本票定稿（与开源仓库路径绑定，见 PRD 开放问题）。

**Blocked by:** None (can start immediately)

- [ ] go.mod 声明 go 1.25+，module path 定稿并将理由记入 dev-docs（勾销 PRD 对应开放问题）
- [ ] 各包目录含占位 doc.go，包名与 Yii2 命名空间一一对应
- [ ] golangci-lint 配置落盘，`go build ./...` 与 lint 全绿
- [ ] README 占位：一句话定位 + 指向 docs/ 与 dev-docs/PRD.md
