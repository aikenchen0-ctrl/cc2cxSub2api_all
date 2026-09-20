# Sub2API 接入

## 现有门户直连入口

保留 Sub2API 门户的“无限画布”入口：新版同时接收 fragment 中的 baseUrl/apiKey 和旧查询参数。fragment 优先；导入前清除 URL 中的凭据，并将配置写入实际的本地渠道列表，不只更新旧的顶层字段。同地址渠道更新密钥，其他渠道保留。

此模式的 API Key 仍保存在用户浏览器中。未登录时由浏览器直连网关，需要网关允许该站点跨域；登录后部分任务使用 Canvas 后端代理。不要把管理员共享 SuperKey 作为普通公开链接发送。

## 后端 Relay

在 .env 中同时设置 SUB2API_RELAY_BASE_URL 和 SUB2API_RELAY_API_KEY。地址只能是 HTTP(S) 根地址或 /v1，不能使用 /api/v1。首次配置自动建立 sub2api-relay 渠道；后续启动同步密钥和模型；两项都清空后停用并删除该渠道存储的密钥。

- SUB2API_RELAY_MODELS：文本模型，默认 gpt-5.5。
- SUB2API_RELAY_IMAGE_MODELS：管理员验证过的图片模型，默认不启用。
- SUB2API_RELAY_VIDEO_MODELS：管理员验证过的视频模型，默认不启用。
- 模型列表使用英文逗号分隔，填写网关公开模型 ID，不填写内部上游模型名。
- 未配置普通用户云端权限时首次启用该权限；已有明确的允许/禁止配置保持不变。管理员可在后台调整。
- Relay 使用现有 Canvas 云端渠道与算力点机制。默认未配置模型价格时 Canvas 不额外扣点；Sub2API 消耗记在共享 Key 的拥有者账户，不是自动记在每位 SSO 用户账户。
- 网关请求跳过 Canvas 按模型名推断厂商原生协议的二次转换。重定向不跟随，避免把服务端 Key 发往其他地址。
- 密钥保存在后端私有设置中，公开设置和管理员设置响应均不返回密钥。数据库仍需按含密钥数据保护。

Docker 使用已有网关网络，不要把容器内 localhost 当成宿主机：

```bash
docker compose -f docker-compose.local.yml -f docker-compose.sub2api.yml up -d --build
```

SUB2API_DOCKER_NETWORK 填现有网络名，网关地址例如 http://sub2api:8080/v1。宿主机开发地址例如 http://localhost:18080/v1。此叠加配置不改动已有 Sub2API、ju 或其他应用。

## SSO 接收端

可选配置 SUB2API_SSO_SECRET，至少 32 字符，Sub2API 门户和 Canvas 必须使用同一个。接收地址为 /api/auth/sso/callback，票据格式兼容 ju：base64url(JSON) 加 HMAC-SHA256 签名，字段为 sub、iat、exp、jti，以及可选的 username、displayName、email、avatarUrl、next。

有效期最多两分钟，校验签名、时间、字段长度和控制字符。已用 jti 的 SHA-256 摘要写入 sso_tickets 表，重启和共享数据库多副本下仍防重放。票据不携带网关密钥。

外部 subject 映射为独立普通账号，不按同名 username/email 绑定已有账号，不提升权限，封禁状态仍生效。回调通过 60 秒 HttpOnly Cookie 交接，前端用同源 POST /api/auth/sso/exchange 获取 Canvas 会话。跳转 URL 不包含会话 Token；Go 请求日志省略查询参数。

Sub2API 门户的“无限画布”现在默认使用 /api/v1/auth/integrations/canvas/start?next=%2F，允许 VITE_CANVAS_SSO_URL 覆盖签发地址。门户后端 CANVAS_SSO_CALLBACK_URL 设置为 Canvas 的 /api/auth/sso/callback 完整地址，默认 http://localhost:3522/api/auth/sso/callback。两端 SUB2API_SSO_SECRET 必须相同；不修改 JU_SSO_CALLBACK_URL。部署前必须同时启用 Canvas Relay 并填写模型列表，否则 SSO 登录成功后仍没有可用云端模型。旧 fragment 链接继续可导入，但不作为默认门户入口。

## 验证边界

自动测试覆盖 Relay 配置、轮换、停用、错误地址、公开设置无密钥、文本/图片/视频创建的网关路径、SSO 票据和路由级会话/流式文本链路。测试使用隔离数据库和模拟上游，不产生真实模型费用。

已补上 Relay 视频完成但无公开地址时的带鉴权 content 下载，并用 HTTP 测试验证任务归属及跨用户拒绝。真实模型目录的能力约束、视频参考素材/时长/分辨率及完整轮询仍需按当前网关实现补齐端到端验收。不能仅凭模拟上游测试承诺全部媒体能力可用。
