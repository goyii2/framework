# 绝对网址主机信息仅取 boot 配置（url 绝对生成）

## 背景

反向生成已覆盖相对网址（`url.Manager.URL` 与包级 `url.To`），但邮件渲染、队列任务、OAuth 回调等场景需要完整网址（`scheme://host[:port]` + 路径 + 查询串）。Yii2 的 `Url::to()` 第二参传 `true` 时从**当前请求**推导 hostInfo（Request 组件持有请求域信息），也允许组件级显式配置。goyii 要决定同一件事：生成绝对网址的主机信息（hostInfo，见 CONTEXT.md 术语「主机信息」）从哪来——请求推断、boot 配置，还是两者兼有。

## 决策

**主机信息只在 boot 期由装配代码经 `Manager.SetHostInfo` 注入，值来自项目配置（ADR-0005 的「项目自有配置 + 代码装配」形态）；不从当前请求推导。** 写读走 Manager 既有 RWMutex 模式：boot 期写、请求期读。

### 1. 来源：仅 boot 配置

绝对网址的真实消费者全部运行在**无请求上下文**里，配置来源恰好覆盖：

- **邮件渲染**：队列 worker 或 console 发信，手里没有 HTTP 请求；
- **队列任务**：延迟执行时原请求早已返回，request 不可用；
- **OAuth 回调**：redirect_uri 在装配期注册给第三方，与运行期生成的回调网址必须一致——这本来就是一份配置。

反过来，唯一握着请求的场景（渲染页面）恰恰不需要绝对网址：页面里的链接跟随当前访问域名才是期望行为，相对 URL 天然满足。

### 2. 不做请求推断

不从当前请求的 `Host` 头 / `X-Forwarded-Proto` 推导 hostInfo：

- **主力消费者无请求可推**：邮件与队列里推断来源是空集，推断版 API 对它们毫无用处；
- **正确推断本身依赖配置**：反代后面取哪个头、信哪个代理是信任边界配置——绕一圈还是回到 boot 配置，却多了一条与显式值打架的来源；
- **Host 头是攻击面**：密码重置类链接若按请求 Host 生成，Host 头注入可把收件人引向钓鱼域；
- **需要跟随当前访问域名的场景用相对 URL 即可**——相对 URL 在任何域下都正确，是更简单的答案。

### 3. API 两层分层，复刻 URL / To

| | 显性 error | 吞错 |
|---|---|---|
| 相对 | `Manager.URL(name, params) (string, error)` | `url.To(name, params) string` |
| 绝对 | `Manager.AbsoluteURL(name, params) (string, error)` | `url.ToAbsolute(name, params) string` |

- `AbsoluteURL` 的路由解析与 `URL` **完全一致**（注册名走 pattern、未注册名走约定回退 `/<name>` + 查询串），仅在结果前拼 HostInfo；HostInfo 未配置返回显性 error（装配缺失应该被看见，而不是拿到半截网址）；注册路由缺路径参数时 error 原样透传。
- 包级 `ToAbsolute` 经 Default manager 吞错：任何失败（未配置 HostInfo、缺路径参数）返回 `"#"`——与 `To` 的 cannot-create 标记一致，模板渲染处不必开 err 分支。
- `To` / `URL` 的签名与行为一律不动，绝对能力是纯增量。

装配形态（示例 main.go 即此形态）：`url.SetDefault(handler.URLs())` 之后紧接调用 `handler.URLs().SetHostInfo(cfg.Web.HostInfo)`——值来自项目配置（yaml / overlay / env），行为在装配代码。

### 4. SetHostInfo 非法值 fail-fast

`SetHostInfo(s)` 接收一体字符串 `scheme://host[:port]`（如 `https://example.com`），尾斜杠容错（`https://example.com/` 与不带斜杠产出一致）。缺 scheme、缺 host，或带 userinfo / 多余路径 / query / fragment 的值是装配错误，**boot 期 panic**（消息指明 boot-time wiring mistake），与 `Add` 重复注册即 panic 同哲学：这些值一旦被接受，产出的半截绝对网址会流向无请求上下文的消费者（邮件、回调地址），错得悄无声息——首用时错误已经进了用户邮箱。这也是 ADR-0005「错误一律 boot 期暴露，请求路径没有首用炸雷」在绝对生成上的延续。

## 后果

正面：

- 「我们的对外主机名是什么」只有一个答案（一处配置），邮件、队列、回调不会各说各话；
- 密码重置等安全敏感链接不受 Host 头注入影响；
- 装配错误（漏配 scheme、误填路径）boot 期即崩，不流入请求路径；
- 两层 API 与既有 URL / To 心智模型完全对齐，零新概念。

负面 / 代价：

- 单主机假设：一个进程只对外服务一个主机名；多域部署需按域拆进程或前置网关分流——与「配置值只有一个」的既有事实一致，不为假想需求加复杂度；
- 换域名要改配置并重启，不能请求期自适应——用显式换可预测与安全；
- `ToAbsolute` 吞错意味着漏配 HostInfo 在页面上表现为 `"#"` 死链而非报错——装配侧须靠 `AbsoluteURL` 的单测或 fail-fast 值校验兜住（示例的 boot 期调用即让非法值无处可藏）。

## Considered Options

- **来源三选一**：**仅 boot 配置**（选定：恰好覆盖全部真实消费者，且是唯一在邮件/队列场景可用的来源）；请求推断（rejected：主力消费者无请求可推，Host 头即攻击面，正确推断仍需信任配置）；boot 配置为主 + 请求推断回退（rejected：同一问题两个答案——邮件里一个域、页面上另一个域，还把 Host 头注入引进密码重置类链接）。
- **API 形态**：**两层分层复刻 URL / To**（选定：装配缺失走显性 error、模板渲染走吞错 `"#"`，两层各有其位）；仅显性 error 版（rejected：视图渲染处到处 err 分支）；仅吞错版（rejected：漏配 HostInfo 被静默吞成 `"#"`，邮件照发死链）；同一函数加选项开关错误行为（rejected：调用点行为不可读，非 Go 惯用法）。
- **非法值处理**：**boot 期 panic**（选定，与 `Add` 重复注册同哲学，ADR-0005 的 boot 期暴露原则）；容忍存储、首用报错（rejected：failingMutex 形态，boot 绿灯请求期炸雷，且炸在无请求上下文里更隐蔽）；静默忽略（rejected：半配置状态直达消费者）。
- **值形状**：**一体字符串 `scheme://host[:port]`**（选定：就是配置文件里一行 URL，无字段间一致性可错）；`Scheme` + `Host` 两字段结构体（rejected：两字段要防互相矛盾，拆开写多一层无收益）。
