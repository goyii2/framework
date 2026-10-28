# 08: AR 泛型层

**Status:** resolved

**What to build:** ActiveRecord 查询与持久化 API（PRD 共识样例）：`ar.New[T](ctx)` 链式查询（where/orderBy/limit/with/join）、`ar.FindByID`、实例 `Save/Delete/Reload`；模型生命周期经 GORM 钩子（BeforeCreate/BeforeUpdate/AfterDelete 等语义）；事务封装。

**Blocked by:** 07

- [x] 泛型 CRUD 对 SQLite 内存库集成测试通过，含 with 关联预加载
- [x] 模型实现钩子接口时生命周期函数按保存/删除链路被调用
- [x] Save/Delete 支持事务原子性，失败可整体回滚
- [x] 查询构造词汇与 Yii2 对齐（where/orderBy/limit/with/join）

## Comments

**落地形态**（main `6813d6f`，分支 feat/active-record 已 ff 合入并删除；包位于 `db/ar`，包名 `ar`）：

- **查询构造 `Query[T]`**：`ar.New[T](ctx)` 返回链式构造器，词汇对齐 Yii2——`Where`（SQL 片段 + `?` 参数，或 struct/map 等值条件）/`OrderBy`/`Limit`/`Offset`/`With`（关联预加载，点号嵌套）/`Join`（完整 JOIN 子句含 ON）。链方法返回**新** Query（不可变分支，共享基查询互不污染）；终端 `All() ([]*T)` / `One() (*T)`（未命中错误包装 `gorm.ErrRecordNotFound`，`errors.Is` 可判）/`Count() (int64, error)`；另有 `New() *T` 产出绑定上下文的空模型（`(new User())->save()` 的对应物）。`ar.FindByID[T](ctx, id)` 按解析出的主键列取行，单列主键任意类型（数值/字符串）均可。
- **实例持久化 `ar.Model`**：模型内嵌 `ar.Model`（仅两个未导出字段，无列），获得 `Save/Delete/Reload`。绑定机制：加载/构造时经鸭子接口 `interface{ bind(context.Context, any) }`（promoted method，无反射扫描）把**创建时的 ctx**与记录指针写回内嵌字段；`Save` 走 GORM Save（主键零值 insert、否则全行 update），`Delete` 按记录主键删除，`Reload` 按主键重取整行（未保存过的零主键明确报错）。
- **设计分叉记录（模型接口 vs 纯结构体约定）**：选**内嵌结构体 `ar.Model` + `T any` 泛型约束**，不引入模型接口约束。理由：① Go 无法给任意 T 添加方法，实例方法 `user.Save()/Delete()/Reload()`（PRD 共识形态）只能经内嵌获得——`class User extends ActiveRecord` 的直接对应；② `T any` 让纯结构体（不内嵌）仍可查询（只读、无实例方法），查询层不强制侵入模型定义；③ 验证层（09 票）的 `user.Validate(scenario)` 同样以模型自身方法表达，与内嵌形态一致。若用接口约束（T 必须实现某 ActiveRecord 接口）则纯结构体无法查询且每个模型被迫声明样板方法，放弃。
- **连接解析**：执行时从 ctx 解析 GORM 句柄，顺序＝事务（`Transaction` 放入）→ `ar.UseConn(ctx, conn)` 注入的连接 → `base.FromContext(ctx).DB()` 应用 db 组件。每次操作经 `Session{NewDB: true}` 取全新 Statement，共享句柄（含事务句柄）间条件不泄漏；ctx 随 Statement 下传（取消 + GORM 钩子上下文）。
- **事务 `ar.Transaction(ctx, fn)`**：函数式封装，fn 收到携带事务的 ctx——闭包内所有查询、新建与加载的实例都参与该事务（**实例绑定事务**：实例保持其创建时的 ctx，事务结束后句柄失效，后续持久化报错而非悄悄落到外层连接）。嵌套调用（ctx 已带事务）经 GORM savepoint 语义：内层回滚只丢弃内层工作，外层可继续提交。`UseConn` 供已持有连接的独立工具绕过 Application 使用本层。
- **生命周期**：直接采用 GORM 钩子接口（模型按需实现 `BeforeSave/BeforeCreate/AfterCreate/AfterSave/BeforeUpdate/AfterUpdate/AfterFind/BeforeDelete/AfterDelete`），本票未引入自有事件机制（与 PRD「模型生命周期事件不走事件调度」一致）。

**验收证据**（合并后 main 上，CGO 经 scoop mingw gcc 启用）：

- `go build ./...` / `go vet ./...` / `gofmt -l .` 全部无输出；`CGO_ENABLED=1 go test -race -count=1 ./...` 全绿（10 个含测试包），其中 `db/ar` 20 个顶层测试全 PASS（无 data race）。
- **SQLite 内存库 CRUD + With 预加载**（`:memory:` + 单连接池，glebarez 纯 Go 驱动；建表用显式 DDL，不用 AutoMigrate）：`TestInsertFindUpdateDeleteReload` 覆盖 insert 回填主键 → FindByID → update 持久化 → 外部变更后 Reload → Delete 后 FindByID 未命中；`TestQueryChainWhereOrderByLimitOffset` 断言 `Where(id>1).OrderBy(id DESC).Limit(2).Offset(1)` 精确取到 [4 3]；`TestQueryCount`/`TestQueryOne`/`TestQueryBranchesDoNotInterfere`（链分支不可变）；`TestWithPreloadsAssociations` 断言 With("Posts") 预加载两层数量与内容、不带 With 时关联不加载；`TestJoinFiltersByRelatedTable` 断言 JOIN 后用户列不被 posts 列污染；`TestDetachedStructQueriesWithoutModel`/`TestUnboundModelRejectsPersistence`/`TestUseConnBypassesApplication` 覆盖纯结构体查询、未绑定拒绝持久化、连接直注。
- **钩子生命周期**（`hookUser` 实现全部九个钩子并记录调用序）：`TestLifecycleHooksOnInsertFindUpdateDelete` 断言 insert 链 `[BeforeSave BeforeCreate AfterCreate AfterSave]`、find 链 `[AfterFind]`、update 链追加 `[BeforeSave BeforeUpdate AfterUpdate AfterSave]`、delete 链追加 `[BeforeDelete AfterDelete]`；`TestHookErrorAbortsInsert` 断言 BeforeCreate 返回错误时插入中止、表空。
- **事务原子性**：`TestTransactionCommitsOnSuccess` / `TestTransactionRollsBackEverythingOnError`（两条 insert + 错误 → 计数 0）/ `TestTransactionRollsBackWhenHookFails`（前一条 insert 随失败钩子整体回滚）；`TestInstanceStaysBoundToItsTransaction`（回滚后行不可见，绑定实例经已结束事务 Reload 报错）；`TestNestedTransactionRollsBackToSavepoint`（内层失败回滚到 savepoint，外层提交后仅剩 outer 行）；`TestInstanceMethodsInsideTransactionParticipate`（闭包内另一实例的未提交写可被 Reload 观察到）。

**遗留**：Save 链路尚未按 Insert/Update 场景触发 validators（PRD 归 09 票，届时在保存链路接入 `Validate(scenario)`）；软删除/悲观锁等 Yii2 高级查询能力未在 P0 范围；gorm logger 与 app.Log 的整合仍属 12 号 log 票。
