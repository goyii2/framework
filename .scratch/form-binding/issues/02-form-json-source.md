# 02: form JSON 源 + Content-Type 分发

Status: ready-for-agent

**What to build:** `form.Bind` 按 Content-Type 分发：`application/json` 走 **map 中转赋值**（解码到 `map[string]any` 后按 `form` tag 反射赋值，不经 `encoding/json` 直解 struct），`application/x-www-form-urlencoded` 维持既有路径。三源白名单语义统一：JSON body 中的多余字段——包括与未打 tag 导出字段大小写相近的键——赋不进任何 struct，从机制上防住直绑模型时的 over-posting；JSON 类型不匹配同样逐字段归并进 `validators.Errors.Fields`（非整包失败）。body 为空或 Content-Type 缺省/不支持时的行为做出裁决并测试固化。**走 TDD**。

**Blocked by:** 01

- [ ] Content-Type 分发：JSON → map 中转赋值；form-urlencoded → 既有路径；分发行为有单测
- [ ] 多余 JSON 字段（含大小写变体）不产生任何赋值（白名单语义与另两源一致）
- [ ] JSON 类型不匹配逐字段报错（如字符串给 int 字段），同 body 内合法字段正常绑定
- [ ] JSON 数组值可绑定 `[]string`；嵌套对象对非 `TextUnmarshaler` 字段报类型错误
- [ ] 空 body / 缺省 Content-Type / 不支持类型的行为已裁决并有测试
- [ ] TDD：测试套件先红后绿
