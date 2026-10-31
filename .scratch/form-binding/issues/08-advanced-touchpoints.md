# 08: advanced 其余绑定点迁移

Status: claimed

**What to build:** examples/advanced 收尾迁移：backend 文章表单的 `readPostForm` 改直绑（create 零值记录、update 先查再绑，同 basic 形态）；system 用户列表触点核查——`FromRequest` 分页排序**维持不动**，仅确认是否存在应顺手迁移的字段读取（如 per-page 判断），核查结论写进工单 Comments。advanced 全量回归绿。

**Blocked by:** 03

- [ ] backend 文章 create/update 直绑，`readPostForm` 及调用点删除
- [ ] system 用户列表核查结论（保持现状的项及理由）记录在工单 Comments
- [ ] `FromRequest` 分页排序调用不变
- [ ] advanced 全量测试通过
