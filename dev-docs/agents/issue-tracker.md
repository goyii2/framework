# Issue tracker：本地 markdown

本仓库的工单与规格以 markdown 文件形式存放在 `.scratch/` 下。

## 约定

- 每个特性一个目录：`.scratch/<feature-slug>/`
- 规格文档：`.scratch/<feature-slug>/spec.md`
- 实施工单每张一个文件：`.scratch/<feature-slug>/issues/<NN>-<slug>.md`，编号从 `01` 起，禁止把多张工单合并进一个文件
- Triage 状态记录在工单文件顶部的 `Status:` 行（角色字符串见 `triage-labels.md`）
- 评论与讨论历史追加到文件底部的 `## Comments` 标题下

## 当技能说「发布到 issue tracker」

在 `.scratch/<feature-slug>/` 下新建文件（目录不存在则先创建）。

## 当技能说「获取相关工单」

读取所引用路径的文件。用户通常会直接给出路径或工单编号。

## Wayfinder 操作

供 `/wayfinder` 使用。**map** 是一个文件，每个工单对应一个 **child** 文件。

- **Map**：`.scratch/<effort>/map.md`（Notes / Decisions-so-far / Fog 正文）。
- **Child ticket**：`.scratch/<effort>/issues/NN-<slug>.md`，编号从 `01` 起，正文写问题。`Type:` 行记录工单类型（`research`/`prototype`/`grilling`/`task`）；`Status:` 行记录 `claimed`/`resolved`。
- **Blocking**：工单顶部一行 `Blocked by: NN, NN`。当其列出的每个文件都为 `resolved` 时，该工单解除阻塞。
- **Frontier**：扫描 `.scratch/<effort>/issues/` 中开放、未阻塞、未认领的文件；编号最小者优先。
- **Claim**：开始任何工作前先设置 `Status: claimed` 并保存。
- **Resolve**：在 `## Answer` 标题下追加答案，设置 `Status: resolved`，然后把上下文指针（要点 + 链接）追加到 `map.md` 的 Decisions-so-far。
