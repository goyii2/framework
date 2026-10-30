# 17: RBAC DbManager

Status: resolved

**What to build:** RBAC（ADR-0003）：AuthManager 接口 + 自建 DbManager——四表（auth_item/auth_item_child/auth_rule 仅存名/auth_assignment，GORM 访问）、item 图内存缓存与失效、规则只在代码定义（启动注册表与库内名字对齐）、`auth.Can/Assign/Revoke` 与 `user.Can` 联动；AccessControl 从 11 的角色回调切换到 AuthManager 后端（接口不变）。

**Blocked by:** 08, 11

- [x] 角色/权限层级沿 DAG 回溯判定正确（含间接授权与多级继承）
- [x] Assign/Revoke 持久化；RolesByUser/Children 查询正确
- [x] 规则（代码定义）在判定链中执行并可否决授权
- [x] AccessControl 经 AuthManager 判定；11 的角色回调实现仍可选用
- [x] item 图缓存失效后不再产生陈旧判定（变更后立即可见）

## Comments

### 验收证据（feat: a0edb7b，分支 feat/rbac-dbmanager 已 ff 合并 main）

- **DAG 回溯判定**：`rbac/manager_test.go` TestCheckAccessDirectAssignment / ...InheritsThroughHierarchy（admin→editor→author→post/create 多级继承、持有向上不泄漏）、...DiamondInheritance（菱形两路径）。回溯方向与 Yii2 DbManager.checkAccessRecursive 一致（沿 parents 上溯），已对照上游源码核实。
- **环检测**：TestAddChildRejectsCycles（自环、直接回边、传递回边、permission→role 拒绝、缺项报 ErrItemNotFound）；环检测基于实时 auth_item_child 边表 BFS（能看见其它进程写入的边）。TestCheckAccessSurvivesTamperedCycle 验证绕过守卫直插环边后判定带 visited 保护不悬挂。
- **Assign/Revoke 持久化**：TestAssignRevokePersistence（冷缓存管理器逐项核对落库、重复 Assign 报 ErrAlreadyAssigned、Revoke 缺失静默、RevokeAll）、TestRolesByUserDirectOnly（只列直接角色）、TestChildrenMaintenance / TestItemLifecycle（RemoveItem 级联清边与授权）。
- **规则链执行与否决**：TestRuleExecutesAndCanVeto（挂在权限上）、TestRuleOnAncestorVetoesDownstream（挂在链中角色上，否决下游权限）、TestRuleReceivesUserAndParams。规则仅代码定义：`Rule` 接口（Name + Execute(userID, params)）；加载图时注册表对齐——auth_rule 有名无实现或 item 引用未注册规则名均在加载时报错并点名（TestRuleRegistryAlignment / TestRuleStoredNameWithoutItemAlignment）；`AddRule` 注册实现并把名字持久化进 auth_rule。auth_rule 表只存 name + created_at（依 ADR，不存序列化数据）。
- **缓存失效（硬标准）**：TestCacheInvalidationMakesMutationsImmediatelyVisible——缓存预热后，AddItem/AddChild/Assign/Revoke/RemoveChild/RemoveItem 每一步在**同一管理器实例**上的下一次判定立即反映；结构性变更整体失效图+分配缓存，Assign/Revoke 精确驱逐该用户条目。管理器所有 DB 语句都在写锁内执行；已加载图为不可变快照，遍历无锁，规则可安全回调管理器。
- **并发**：TestConcurrentChecksAndMutations（8 读 goroutine 判定 + 2 写 goroutine Assign/Revoke/AddChild/RemoveChild 交替 50 轮），`go test -race` 通过。
- **web 联动**：`web.AccessChecker` 消费端接口（CheckAccess(userID, itemName, params map[string]any) (bool, error)）；`rbac.Params` 为 `map[string]any` 别名，`*rbac.DbManager` 结构化直插（rbac/web_compat_test.go 编译期钉死）。`Handler.SetAuthManager` + `User.Can/CanWithParams` + `AccessControl{Auth: ...}`：具体角色名走后端（后端错误按 500 报，不冒充 403），"?"、"@" 语义不变；Auth 为空时 11 的角色回调/RoleResolver 原样可用（既有 TestAccessControlCustomRoleResolver 等全部保持绿）。
- **测试汇总**：`go test -race -count=1 ./...` 20 包全绿（rbac 19 用例、web 134 用例、全仓 1075 用例）；`scripts/p0-check.sh` 31/31 通过（含 examples/basic migrate→seed→serve 冒烟：demo 删除 403、admin 经 admin→post/delete 间接授权删除 204）。

### 建表方式记录（工单要求的 Comments 备注）

**examples 采用 yiimigrate 迁移路径**：`rbac.NewMigration()`（名 `m20260101_000003_create_auth_tables`）经 migrate 引擎四方言生成 DDL，examples/basic 在 `migrations.All()` 注册。DbManager 自身不建表（缺表时判定报错而非静默拒绝，TestGraphLoadFailsBeforeTablesExist）。

方言可移植性导致的两处 DDL 分叉（相对 Yii2 官方 schema）：1) auth_item_child / auth_assignment 的复合主键落为 UNIQUE 索引（SQLite 方言不支持 ADD PRIMARY KEY，且 .PrimaryKey() 标志在 SQLite 强制 INTEGER 类型，不适合字符串键；单列字符串主键用内联 `Append("PRIMARY KEY")` 表达）；2) 不建外键约束（SQLite 方言无 ADD FOREIGN KEY），RemoveItem 由管理器显式级联删边与授权，正确性不依赖引擎级联。

### API 形态备注

- `Assign(itemName, userID)` 均以字符串按名操作（Yii2 传 Item 对象）；重复 Assign 报 `ErrAlreadyAssigned`，对齐 Yii2 InvalidCallException 语义。
- 缓存为进程内（图+按用户分配）：跨进程部署下其它进程的结构变更在本进程自身下次失效/重启后可见——ADR 选定"item 图内存缓存"的既定代价，已在 DbManager 文档注释说明。
