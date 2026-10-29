# 组件：cache

Cache 接口 + memory/file/redis 三实现 + 依赖失效（Dependency）。对应 `yii\cache`。

## 使用

```go
ctx := c.Context()                    // 请求 ctx（或 app.WithContext(...)）
cache := c.App().Cache()

// 读 / 写 / 删 / 存在性 / 清空
var posts []Post
hit, err := cache.Get(ctx, "post-list", &posts)
err = cache.Set(ctx, "post-list", posts, time.Minute)
err = cache.Delete(ctx, "post-list")
exists, err := cache.Exists(ctx, "post-list")
err = cache.Flush(ctx)                // 仅清本 keyPrefix 命名空间

// 读穿（推荐路径）：未命中或失效时执行 build 并回填
err := cache.GetOrSet(ctx, "post-list", &posts,
    func(ctx context.Context) (any, error) {
        return ar.New[Post](ctx).OrderBy("id DESC").Limit(50).All()
    },
    time.Minute, dep)
```

`ttl <= 0` 用配置的 `cache.ttl` 默认值。

## 依赖失效

TTL 之外按「指纹」判定过期——数据变了缓存即失效，不必等时间到：

```go
// 值依赖：指纹 = 任意序列化值。max id 变了（新建/删除），列表即重建
dep := cache.NewValueDependency(postMaxID(ctx))

// 时间依赖：到点失效
dep := cache.NewTimeDependency(time.Now().Add(10 * time.Minute))
```

依赖在 `GetOrSet` 写入时捕获指纹，读取时重算比对（需要再查一次库时，把指纹查询做便宜——max(id)、版本号等）。

## 后端与配置

```yaml
cache:
  driver: file          # memory | file | redis
  keyPrefix: myapp      # file/redis 的键命名空间（Flush 只清本前缀）
  ttl: 3600             # 秒；ttl<=0 时的默认寿命
  path: runtime/cache   # file 后端目录
  redis: { addr: "127.0.0.1:6379", password: "", db: 0 }
```

- `memory`：进程内，清扫协程回收过期项，重启即空——单进程开发态。
- `file`：目录落盘，跨进程共享，重启存活。
- `redis`：多进程部署；连接复用 `redis` 段。

## 相关文档

- [快速上手](快速上手.md)（示例的缓存演示与 X-Cache 观察）
- [配置参考](配置参考.md)（`cache` 段全字段）
