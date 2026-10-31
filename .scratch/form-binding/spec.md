# form-binding 规格共识

来源：grill-with-docs 会话（2026-10-31）确认的 13 项共识。实施代理开工前先读本文件。

## 总裁决

1. **SearchModel 维持不引入**（PRD 行 95 原裁决成立）："读 query"的样板由表单绑定机制接管；理由固化进 ADR 0006。
2. **FormModel 以「表单模型」补齐**：不复刻 `yii\base\Model` 动态 API（attributes()/scenarios()/load()），以 struct tag 绑定 + 场景校验闭环落地。词条已入 CONTEXT.md「表单模型（Form Model）」。

## form 包（新增）

3. 三层 API：`form.Bind(r, &f)`（body，按 Content-Type 分发 JSON / x-www-form-urlencoded）、`form.BindQuery(r, &f)`（query）、`form.Load(ctx, r, &f, scenario)`（绑定+校验一步，返回 `*validators.Errors`，nil=通过）；`web.Context` 薄委托便捷方法；无新接口/基类，校验走已有 `validators.Ruled`。
4. v1 范围：JSON body + form-urlencoded + query 三源；标量全集 + `[]string` + `time.Time`(RFC3339) + `encoding.TextUnmarshaler` 扩展点；**multipart 文件上传排除**。
5. 安全模型：`form` tag 即白名单，未打 tag 字段绝不绑定；未知字段忽略（`_csrf` 天然兼容）；**三源白名单语义统一**——JSON 解码到 map 再按 tag 赋值（不经 `encoding/json` 直解 struct），防直绑时的 over-posting；类型错误每字段归并进 `validators.Errors.Fields`，消息走 i18n。
6. ActiveForm 回填：`form.Values(f)` 反射导出 `map[string]string`。
7. form 包属编解码类高风险逻辑 → **TDD**。

## 绑定目标形态

8. **直绑为默认**：gii 模型模板给可编辑列生成 `form` tag；create 直绑零值记录；**update 先查再绑**（fetch → Load → Save，防未提交字段被零值覆盖）；`postPayload` / `postFormInput` / gii `FormInput`+`apply`+`FormValues` 三件套淘汰；登录表单、filter struct 这类"形态本就不同"的保留独立 struct。

## SearchModel 侧落地

9. filter struct 打 `form` tag 经 `BindQuery` 读取，`Apply` 留在项目代码显式组装；无 `search()` 大一统概念。
10. `dataprovider.FromRequest` 维持现状（排序白名单属取数层语义，与字段绑定不同层，并存不合并）。
11. gii 列表骨架生成过滤：filter struct + `BindQuery` + `Apply` 骨架（字符串列 LIKE+转义、其余等值，转义辅助随生成物携带）+ `keep` 闭包。

## 迁移与文档

12. examples 全量迁移：登录表单改表单模型闭环，淘汰裸 zog schema + `translateIssues` 旁路（框架不禁止直接用 zog，逃生门靠能力仍在、不靠示例演示）；gii 模板同步；gii 生成测试断言更新。
13. 单个 **ADR 0006**（`dev-docs/adr/0006-form-binding-layer-not-searchmodel.md`）承载完整裁决；PRD 行 95 表述微调；工单 31（goyii-framework/issues/31-gridview-listview.md）追加 Comment 指向 ADR；`docs/` 新增表单模型使用文档。

## 工单索引

| # | 工单 | Blocked by |
|---|---|---|
| 01 | form 核心绑定（form-urlencoded + query） | 无 |
| 02 | JSON 源 + Content-Type 分发 | 01 |
| 03 | Load 闭环 + Context 便捷 + Values 回填 | 02 |
| 04 | 登录表单迁移（basic + advanced） | 03 |
| 05 | basic 文章 create/update 直绑 | 03 |
| 06 | basic 列表过滤迁移 | 01（可与 02–05 并行） |
| 07 | gii 模板迁移：直绑 + 过滤骨架 | 03, 05, 06 |
| 08 | advanced 其余触点迁移 | 03 |
| 09 | ADR 0006 + 使用文档 + 收尾验收 | 04, 07, 08 |
