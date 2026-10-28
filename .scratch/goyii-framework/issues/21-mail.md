# 21: mail 组件

**Status:** ready-for-agent

**What to build:** Mailer 抽象 + SMTP 实现：消息构造（收发件人/主题/正文/附件）与发送接口，经 Application 取用；开发态提供 log Mailer（不外发，完整消息落日志），生产态 SMTP。

**Blocked by:** 02

- [ ] SMTP 实现对本地桩服务器完成发送验证
- [ ] log Mailer 在开发态记录完整消息内容
- [ ] Mailer 为接口，单测可注入 fake
