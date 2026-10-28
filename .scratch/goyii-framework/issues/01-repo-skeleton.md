# 01: 仓库骨架

**Status:** resolved

**What to build:** goyii/framework 的 Go module 骨架：可构建、可 lint 的空包布局，包名对应 Yii2 命名空间（base/web/db/log/cache/rbac/i18n/console/helpers/validators/migrate/dataprovider），后续所有工单在此之上落地。module path 在本票定稿（与开源仓库路径绑定，见 PRD 开放问题）。

**Blocked by:** None (can start immediately)

- [x] go.mod 声明 go 1.25+，module path 定稿并将理由记入 dev-docs（勾销 PRD 对应开放问题）
- [x] 各包目录含占位 doc.go，包名与 Yii2 命名空间一一对应
- [x] golangci-lint 配置落盘，`go build ./...` 与 lint 全绿
- [x] README 占位：一句话定位 + 指向 docs/ 与 dev-docs/PRD.md

## Comments

- 2026-10-28 实施记录（Windows + Git Bash，Go 1.27.2 编译，go.mod 声明 go 1.25）：
  - module path 定稿理由已记录于 `dev-docs/PRD.md`「工程约定」节（`goyii` 已被 2016 年注册的空置个人账号占用，`goyii2` 为实测可用的最短对应名）；PRD「开放问题」已无该项，无需再改动 PRD。
  - 包目录共 12 个：base/web/db/log/cache/rbac/i18n/console/helpers/validators/migrate/dataprovider，与 PRD「工程约定」所列包名一一对应；各含一句话占位 doc.go，说明对应的 Yii2 子系统。
  - golangci-lint 本机已有 v2.13.1，无需安装；`.golangci.yml` 采用 v2 `default: standard` 温和默认 linter 集 + gofmt 格式检查。
  - 验收证据：
    - `go build ./...` -> 无输出（成功）
    - `go vet ./...` -> 无输出（成功）
    - `golangci-lint run` -> `No issues found`
  - 流程：claimed 单独提交 `8c011b4` -> `feat/repo-skeleton` 上实现提交 `b4a81de` -> main `--ff-only` 合并并删除任务分支。
