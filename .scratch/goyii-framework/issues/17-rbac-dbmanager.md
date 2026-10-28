# 17: RBAC DbManager

**Status:** claimed

**What to build:** RBAC（ADR-0003）：AuthManager 接口 + 自建 DbManager——四表（auth_item/auth_item_child/auth_rule 仅存名/auth_assignment，GORM 访问）、item 图内存缓存与失效、规则只在代码定义（启动注册表与库内名字对齐）、`auth.Can/Assign/Revoke` 与 `user.Can` 联动；AccessControl 从 11 的角色回调切换到 AuthManager 后端（接口不变）。

**Blocked by:** 08, 11

- [ ] 角色/权限层级沿 DAG 回溯判定正确（含间接授权与多级继承）
- [ ] Assign/Revoke 持久化；RolesByUser/Children 查询正确
- [ ] 规则（代码定义）在判定链中执行并可否决授权
- [ ] AccessControl 经 AuthManager 判定；11 的角色回调实现仍可选用
- [ ] item 图缓存失效后不再产生陈旧判定（变更后立即可见）
