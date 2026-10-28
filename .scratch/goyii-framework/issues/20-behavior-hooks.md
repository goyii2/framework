# 20: 常用行为钩子

**Status:** ready-for-agent

**What to build:** Yii2 常用 Behavior 四件的替代物（PRD 放弃动态 Behavior 后的落地形态：结构体内嵌 + GORM 钩子）：Timestamp（自动填充创建/更新时间）、Blameable（经当前 Identity 填充操作人）、Sluggable（生成唯一 slug）、OptimisticLock（版本冲突检测）。

**Blocked by:** 08

- [ ] Timestamp 按场景填充 created/updated 字段（字段名与时间格式可配）
- [ ] Blameable 从请求上下文取当前 Identity 填充操作人
- [ ] Sluggable 生成唯一 slug，冲突时追加序号
- [ ] OptimisticLock 版本冲突时保存失败并返回可识别的错误类型
