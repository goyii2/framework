# 09: ADR 0006 + 使用文档 + 表述微调 + 收尾验收

Status: resolved

**What to build:** 决策固化与收尾：**ADR 0006**（`dev-docs/adr/0006-form-binding-layer-not-searchmodel.md`）落盘，承载"引入表单绑定层、不引入 SearchModel"的完整裁决——含三源统一白名单的安全模型、JSON map 中转的理由、`FromRequest` 并存分工、zog 逃生门定位（能力仍在、示例不演示）、与 `yii\base\Model` 动态 API 的取舍。`docs/` 新增表单模型使用文档（三源绑定、白名单、Load 闭环、直绑、过滤模式、gii 产物形态）。PRD"不引入 SearchModel"处表述微调为"filter struct 经表单绑定读取、Apply 显式组装"。工单 31（goyii-framework/issues/31-gridview-listview.md）追加 Comment 指向 ADR 0006。全仓测试 + vet 收尾验收。

**Blocked by:** 04, 07, 08

- [x] ADR 0006 落盘，覆盖 spec.md 13 项共识中的全部裁决点
- [x] `docs/` 使用文档覆盖表单模型全用法（含登录/直绑/过滤三个范例形态）
- [x] PRD 表述微调完成；工单 31 追加指向 ADR 0006 的 Comment
- [x] 全仓 `go test ./...` 与 `go vet ./...` 通过

## Comments

- **2026-10-31 实施验收（落地提交 `db62f9f`，经 `f859b49` 认领；feat/form-binding-09 已 ff-only 合并 main 并删除）**
- 交付物：`dev-docs/adr/0006-form-binding-layer-not-searchmodel.md`（标题即裁决；背景 -> 决策六组（三层 API 不复刻动态 Model / 三源统一白名单与 JSON map 中转 / 类型集与空值语义 / 直绑默认与先查再绑 / filter struct+Apply 显式 / Values 回填与 zog 逃生门）-> 后果（正负含 datetime-local、checkbox、可空清空、fail-fast panic）-> 落地中的再决策（空值语义 JSON 统一、gii 白名单、datetime RFC3339 占位、zog `Required(...)` 陷阱、指针字段与 ActiveForm checkbox 两处机制小修）-> Considered Options 五组（JSON map 中转 vs encoding/json 直解、filter+Apply vs SearchModel 大一统 vs 逐键 Get、直绑 vs FormInput vs JSON DTO、struct tag vs 复刻动态 API、multipart 排除）；含三源分发 Mermaid flowchart）。
- 交付物：`docs/组件-表单模型.md`（使用文档：白名单与类型表、三源绑定（Mermaid 流程图）、Load 闭环与场景、直绑/先查再绑、filter 过滤模式与 keep 派生、form.Values 回填、gii 生成物形态、zog 逃生门；登录/直绑/过滤三范例代码均取自 examples/basic 真实形态（site.go loginForm、models/post.go 标签契约、controllers/post.go create/update/filters/keep/renderPostForm））；`docs/README.md` 组件索引表新增条目（列于 validators 之后）。
- 交付物：`dev-docs/PRD.md` 行 95 表述微调——GridView 过滤分工括注由「读 query 自行 Where，不引入 SearchModel」改为「filter struct 经表单绑定读取、Apply 显式组装，不引入 SearchModel」，仅动该字句；`.scratch/goyii-framework/issues/31-gridview-listview.md` 只追加一条 Comments 补记（过滤读取已由表单绑定机制承接、查询组装仍显式，指向 ADR 0006 与新文档），正文未改。
- 本票零代码改动：form/、view/、gii/、examples/ 与全部机制代码一字未动。
- 全量验收：`rtk go test ./... -count=1` -> **1533 passed in 52 packages**（与 08 票基线持平，文档票零增量）；`rtk go vet ./...` 干净；`golangci-lint run ./...` 全仓 0 issues。
- 遗留/待人工项：无。form-binding 全批工单（01–09）就此收官。
