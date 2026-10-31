# goyii/framework

## Agent skills

### Issue tracker

工单以本地 markdown 文件追踪，位于 `.scratch/<feature-slug>/`。见 `dev-docs/agents/issue-tracker.md`。

### Triage labels

使用五个默认 triage 标签（`needs-triage` / `needs-info` / `ready-for-agent` / `ready-for-human` / `wontfix`）。见 `dev-docs/agents/triage-labels.md`。

### Domain docs

单一上下文（single-context）布局：术语表在仓库根目录 `CONTEXT.md`，ADR 在 `dev-docs/adr/`。见 `dev-docs/agents/domain.md`。

## 分支与验收

- 集成分支 main：工单在 `feat/<slug>-<NN>` 实施，--ff-only 合回后删分支；工单 Status 变更（claimed/resolved）随提交入库，验收证据追加工单 Comments。
- 收尾验收一键命令：`scripts/p0-check.sh`（gofmt -> vet -> 全量 -race -> basic 冒烟）。
