# 08: AR 泛型层

**Status:** ready-for-agent

**What to build:** ActiveRecord 查询与持久化 API（PRD 共识样例）：`ar.New[T](ctx)` 链式查询（where/orderBy/limit/with/join）、`ar.FindByID`、实例 `Save/Delete/Reload`；模型生命周期经 GORM 钩子（BeforeCreate/BeforeUpdate/AfterDelete 等语义）；事务封装。

**Blocked by:** 07

- [ ] 泛型 CRUD 对 SQLite 内存库集成测试通过，含 with 关联预加载
- [ ] 模型实现钩子接口时生命周期函数按保存/删除链路被调用
- [ ] Save/Delete 支持事务原子性，失败可整体回滚
- [ ] 查询构造词汇与 Yii2 对齐（where/orderBy/limit/with/join）
