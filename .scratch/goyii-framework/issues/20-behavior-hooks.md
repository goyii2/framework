# 20: 常用行为钩子

**Status:** resolved

**What to build:** Yii2 常用 Behavior 四件的替代物（PRD 放弃动态 Behavior 后的落地形态：结构体内嵌 + GORM 钩子）：Timestamp（自动填充创建/更新时间）、Blameable（经当前 Identity 填充操作人）、Sluggable（生成唯一 slug）、OptimisticLock（版本冲突检测）。

**Blocked by:** 08

- [x] Timestamp 按场景填充 created/updated 字段（字段名与时间格式可配）
- [x] Blameable 从请求上下文取当前 Identity 填充操作人
- [x] Sluggable 生成唯一 slug，冲突时追加序号
- [x] OptimisticLock 版本冲突时保存失败并返回可识别的错误类型

## Comments

### 落地形态（main `4432770`，分支 feat/behavior-hooks 已 ff 合入并删除；包 `behaviors`，对应 yii\behaviors 命名空间）

- **位置分叉**：独立顶层 `behaviors` 包，不进 `db/ar`——08 号已 resolved 的包零改动；且 Blameable 需要请求层 Identity（web 包），放进 db/ar 会把请求层耦合进数据层。依赖方向 `behaviors -> web -> base -> db` 单向无环，`db/ar` 与 `behaviors` 可在同一模型自由组合。
- **形态统一：结构体内嵌 + `behaviors.Hooks` 分发器**。模型内嵌 `ar.Model` + `behaviors.Hooks` + 任意组合的行为结构体：
  ```go
  type Article struct {
      ar.Model
      behaviors.Hooks       // 唯一的 GORM 钩子入口
      behaviors.Timestamp   // created_at / updated_at (time.Time)
      behaviors.Sluggable   // slug
  }
  ```
  Hooks 存在的原因：GORM 按方法名调用生命周期钩子，Go 无法从两个内嵌结构体同时提升同名方法（提升歧义会被静默排除，两个钩子都不执行）——所以四个行为自身不定义 GORM 钩子，`Hooks` 是唯一提升实现（BeforeCreate/BeforeUpdate/AfterUpdate），经反射遍历模型的内嵌字段（含嵌套 base、非 nil 内嵌指针，带指针环防护）把事件分发给各行为。模型自定义同名钩子时遮蔽 Hooks 的提升，须经导出的 `DispatchCreate/DispatchUpdate/DispatchUpdated` 手动接线（有测试）。**组合优于继承落地**：一份 `allDoc` 测试同时内嵌四件 + ar.Model，一次 save 四组字段全部就位。
- **Timestamp**：两形态变体承载时间格式——`Timestamp`（time.Time，UTC 归一）与 `TimestampUnix`（int64 Unix 秒），插入填 created+updated、更新只刷 updated，无条件覆盖（Yii2 默认语义；导入历史数据需在 save 后补写）。字段名/列名可配的落地：列名跟随字段映射（标准 created_at/updated_at），非标准列名时模型自带字段 + 自己的钩子填充（结构体内嵌模式下字段声明权本就在模型作者手里，Yii2 的 attributes 数组在 Go 无可变对应物）。事务内填充照常（有测试）。
- **Blameable**：`CreatedBy/UpdatedBy int64`，身份经 `web.IdentityFromContext(stmt ctx)` 解析——请求内走 session 登录或 HttpBasic/Bearer 的瞬态 Identity，非 HTTP 上下文（console/后台/测试）走本票新增的 `web.WithIdentity(ctx, identity)` 接缝（显式绑定优先于请求身份，web 包有独立测试）。**无身份行为（已定义并有测试）**：guest/无 web 上下文/非数值 GetID → 填 0（Yii2 guest 写 null 的对应物，MySQL NOT NULL 列即 0），更新遇 guest 时 updated_by 回 0 而非保留旧值（填充无条件，语义与 Yii2 一致）。int64 数值 ID 为唯一形态（Yii2 默认 user 表自增主键形态）；字符串 ID 应用自带字段，Comments 记录在案。
- **Sluggable**：内嵌 `Sluggable{Slug}` + 模型实现 `SlugOptions() behaviors.SlugOptions` 声明配置（`Source` 必填；`Separator` 默认 `-`、`KeepCase`、`KeepUnicode`）——方法遮蔽是零值安全的配置通道（`ar.New[T].New()` 是 new(T)，结构体字面量配置会丢）。生成规则：保留字母/数字（默认仅 ASCII，非 ASCII 丢弃；拼音转写超范围），分隔符拼接，默认小写。**非 ASCII 策略（已定义）**：默认丢弃，纯非 ASCII 标题产出空 slug（不判重不追加、save 正常）；`KeepUnicode: true` 原样保留（"中文 标题 42" -> `中文-标题-42`，有测试）。**更新策略分叉**：slug 已存在则更新时**不**重生成（URL 稳定，Yii2 `immutable` 的有效半边设为默认），slug 为空（含插入与更新）才生成；插入时显式 slug 被尊重（不覆盖）。**唯一性**：查库判重（探测查询跑在与 save 同一连接上——事务内可见未提交行，有事务内两次同标题测试），冲突追加 `base-2/base-3`（探测循环上限 10000 防病态表死循环）；探测按主键排除自身。并发窗口内两个插入仍可同时通过探测，airtight 需列上 UNIQUE 索引兜底（测试 DDL 已建 UNIQUE，如实记录边界）。
- **OptimisticLock**：`Version int64`，插入初始化为 1（显式预置则保留）；更新前把 version 提到 old+1 并向语句追加 `WHERE version = old`（与主键条件 AND 合并），语句影响 0 行即冲突，返回 `behaviors.ErrStaleVersion`（`errors.Is` 可判，穿透 ar.Save 的错误包装，有测试），记录回滚到加载时的版本供 reload-retry。**实现要点**：GORM 递给钩子的 tx 是 NewDB 会话包装，其 `RowsAffected` 恒 0——真实计数在共享 Statement 的属主 DB 上（`tx.Statement.DB.RowsAffected`，实验验证后采用）；AfterUpdate 冲突错误同时抑制了 gorm.Save「更新 0 行回退 INSERT」的 upsert 路径（删行后的更新报冲突而非撞主键，有测试）。Delete 不带版本条件（删除他人改过的行不属于丢失更新，记 Comments）。

### 验收证据（合并后 main `4432770`，CGO 经 scoop mingw gcc 启用）

- `go build ./...` / `go vet ./...` / `gofmt -l .` 无输出；`CGO_ENABLED=1 go test -race -count=1 ./...` 21 个含测试包全绿（含新增 `behaviors` 包）。
- **Timestamp**（`timestamp_test.go`，SQLite :memory: 显式 DDL）：插入双填 + 载入往返一致；更新 created 不变 / updated 刷新（created_at 精度到亚秒，60ms 隔离）；`TimestampUnix` 秒级变体同路径；事务内填充随提交落库。
- **Blameable**（`blameable_test.go`）：ctx 身份 42 插入双 42；换人 11 更新 updated_by 切换、created_by 保留 7；无身份插入/更新均 0；非数值 GetID 为 0；**全链路 HTTP**（session 中间件 + HttpBasic + 真实 handler）：alice(7) 创建、bob(11) 更新，响应与落库一致；**并发身份切换 -race**：httptest.Server 上 3 身份 × 8 轮各自并发创建+回写自身行（共 24 行），断言每行 created_by/updated_by 恰为其作者、总数精确，-race 干净。
- **Sluggable**（`sluggable_test.go`）：`Hello, Big World!` -> `hello-big-world`；四个同义标题（大小写/标点/空白变体）得 `dup/dup-2/dup-3/dup-4`；显式 slug 保留 + 冲突得 `-2`；改名后 slug 稳定；DB 直清空 slug 后下次更新补填；自定义分隔符 `_` + KeepCase（大小写敏感探测，SQLite BINARY collation 验证）；默认丢非 ASCII（`中文标题 pure` -> `pure`，纯中文 -> 空串）；KeepUnicode 中文 slug 及其 `-2` 冲突；缺 `SlugOptions()` 的模型 save 明确报错；事务内两次同标题探测到未提交行取 `-2`。
- **OptimisticLock**（`optimisticlock_test.go`）：插入 version=1；连续 save 递增并落库；双载入 A 先写成功、B 写入被拒（`errors.Is(err, ErrStaleVersion)`）、B 内存版本回滚为 1 且编辑保留、库行为 A 的 v2、B reload 重试成功升 v3；冲突错误穿透 ar.Save 包装且不误判为 ErrRecordNotFound；删除后的更新读作冲突（不触发 gorm 的 0 行回退 INSERT）。
- **分发器与组合**（`hooks_test.go`）：行为藏身未导出 base 结构体（经内嵌字段间接内嵌）仍被分发到；模型自定义全部三个钩子并经 Dispatch* 委托——填充照常、自定义逻辑照跑、冲突照拒；无 Hooks 内嵌时行为保持静默（以 Sluggable 为证——Timestamp 字段名自带 GORM auto-fill，不作此证）；`DispatchCreate` 传值（不可寻址）报错；四件全内嵌的组合测试一次 save/一次 update 全字段协同。web 包新增 `TestIdentityFromContext`（nil/显式/请求 guest/请求登录/显式优先）。

### 遗留

- Blameable 仅 int64 数值 ID 形态；字符串 ID（GUID 用户表）应用需自带字段（可后续加 `BlameableString` 变体）。
- Sluggable 并发插入的探测-写入窗口依赖列上 UNIQUE 索引兜底（框架测试 DDL 已示范）；拼音转写明确超范围。
- OptimisticLock 仅覆盖 Save 更新链路；Delete 不带版本条件，如需「删除也须基于亲见版本」可后续在 BeforeDelete 加同款 WHERE。
- gorm 句柄若经 `Session` 自定义 `NowFunc`，Timestamp 填充不走它（直接 time.Now()）；如需可注入时钟再议。
