# 组件：log

`log/slog` 底座上的分级/分类日志：多 Target（console/file）、按类别过滤、慢查询与 profile 计时。对应 `yii\log`。

## 使用

```go
app.Log().Info("user logged in", "user", name)   // base.Log 最小契约接口
app.Logger().Log(slog.LevelDebug, "cache", "post list served", "state", "HIT")
//                              ^^^^ 类别：Target 过滤的键
```

类别是自由字符串，约定前缀：`db`（查询）、`cache`、`web`、`mail`、`application`（无类别即归入）。

## 配置

```yaml
log:
  level: debug            # 根级别：debug | info | warn | error
  targets:
    - type: console       # 人读控制台：info 起步，排除 db 类噪音
      minLevel: info
      except: ["db"]
    - type: file          # 可观测性文件：全量 db 记录 + cache 事件，按大小轮转
      path: runtime/app.log
      minLevel: debug
      categories: ["db*", "cache"]
      maxSizeMB: 10
      maxFiles: 5
  gorm:
    slowThresholdMs: 200  # 达到该耗时的查询以 WARN 落盘
```

类别模式支持尾部 `*` 通配（`db*` 命中 `db`、`db.slow`）；`except` 优先于 `categories`。`log.level` 是整棵树的根闸门，Target 的 `minLevel` 只能更严。

## 轮转与关闭

file target 超过 `maxSizeMB` 轮转为 `app.log.1`、`app.log.2`...保留 `maxFiles` 份。`console.Execute` 在命令返回后统一 `Close()` 全部 Target（serve 优雅停机、migrate、用户命令 alike）。

## profile 计时

```go
prof := app.Logger().BeginProfile("render post/list")
defer prof.End()   // 成对的 begin/end 记录（INFO 级，profile 类别）
```

## 直接构造（测试 / 独立组件）

```go
target, err := log.NewFileTarget("app.log", 10<<20, 5)
logger := log.New(slog.LevelInfo, target)
tailored := logger.WithTarget(console, log.Filter{MinLevel: slog.LevelWarn, Includes: []string{"db*"}})
mem := log.NewMemoryTarget(1000)               // 有界内存 Target，Records() 读回
chatty := logger.WithLevel(slog.LevelDebug)    // 返回降/升级别后的副本，原 logger 不变
```

`MemoryTarget` 保留最近 N 条记录（`debug` 面板按请求挂载的就是它；测试断言分发记录也用它）。

## 相关文档

- [组件：db 与 ActiveRecord](组件-db与ActiveRecord.md)（GORM 查询日志接入）
- [配置参考](配置参考.md)（`log` 段全字段）
