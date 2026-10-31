# 09: ADR 0006 + 使用文档 + 表述微调 + 收尾验收

Status: ready-for-agent

**What to build:** 决策固化与收尾：**ADR 0006**（`dev-docs/adr/0006-form-binding-layer-not-searchmodel.md`）落盘，承载"引入表单绑定层、不引入 SearchModel"的完整裁决——含三源统一白名单的安全模型、JSON map 中转的理由、`FromRequest` 并存分工、zog 逃生门定位（能力仍在、示例不演示）、与 `yii\base\Model` 动态 API 的取舍。`docs/` 新增表单模型使用文档（三源绑定、白名单、Load 闭环、直绑、过滤模式、gii 产物形态）。PRD"不引入 SearchModel"处表述微调为"filter struct 经表单绑定读取、Apply 显式组装"。工单 31（goyii-framework/issues/31-gridview-listview.md）追加 Comment 指向 ADR 0006。全仓测试 + vet 收尾验收。

**Blocked by:** 04, 07, 08

- [ ] ADR 0006 落盘，覆盖 spec.md 13 项共识中的全部裁决点
- [ ] `docs/` 使用文档覆盖表单模型全用法（含登录/直绑/过滤三个范例形态）
- [ ] PRD 表述微调完成；工单 31 追加指向 ADR 0006 的 Comment
- [ ] 全仓 `go test ./...` 与 `go vet ./...` 通过
