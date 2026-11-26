# Sub2API 接入

## 现有门户直连入口

Sub2API 门户的“无限画布”入口统一使用 `GET /api/v1/auth/integrations/canvas/start`。浏览器只建立 Canvas 自己的 HttpOnly Session，不保存 Sub2API JWT、用户 API Key 或 SuperKey。

## 后端 Relay

在服务端设置 `SUB2API_RELAY_BASE_URL` 和 `SUB2API_APP_CREDENTIAL`。地址只能是 HTTP(S) 根地址或 `/v1`，不能使用 `/api/v1`。`SUB2API_APP_CREDENTIAL` 仅用于卫星服务端调用 Sub2API，用户身份通过 `X-Sub2API-On-Behalf-Of` 传递；不得用 SuperKey 或共享 API Key 代替。

- SUB2API_RELAY_MODELS：文本模型，默认 `gpt-6.1-sol,gpt-6-astra,gpt-6-sol,gpt-6.1-terra,gpt-6.2-astra,gpt-6.2-sol`，选中首项 `gpt-6.1-sol`。
- SUB2API_RELAY_IMAGE_MODELS：管理员验证过的图片模型，默认 `gpt-image-2.6-flare,gpt-image-2,gpt-image-2.5,grok-imagine-image-1.5`，选中首项 `gpt-image-2.6-flare`。
- SUB2API_RELAY_VIDEO_MODELS：管理员验证过的视频模型，默认不启用。
- 模型列表使用英文逗号分隔，填写网关公开模型 ID，不填写内部上游模型名。
- 未配置普通用户云端权限时首次启用该权限；已有明确的允许/禁止配置保持不变。管理员可在后台调整。
- Relay 使用现有 Canvas 云端渠道与算力点机制。Sub2API 消耗按当前 SSO 用户记录计费，并由服务端 SuperKey 在启用分组范围内选取上游。
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

## 上游返回登录页 HTML（invalid character '<'）

现象：生成图片或文本长时间没有结果，任务最终报 `invalid character '<' looking for beginning of value`，日志里出现面板登录页（例如 `登录·xxx 面板`）。

原因：Canvas 把任意 2xx 响应当成接口 JSON 解析，而上游返回的其实是面板/网关的 HTML 登录页。常见来源：

- `SUB2API_RELAY_BASE_URL` 指向面板首页或 `/api/v1`，拼出 `/api/v1/v1/images/generations` 这类不存在的路径，面板用单页应用兜底返回 200 HTML。
- 前置反代把 `/v1` 请求交给面板，或应用凭据失效后由登录页接管。

当前行为：

- 请求地址不是网关 `/v1`（包含 `/api/v1` 或重复 `/v1`）时直接返回 502「模型网关地址配置错误」，不发出真实请求，也不扣点。
- 上游返回 2xx 但 `Content-Type` 为 HTML 或响应体以 `<` 开头时立即返回 502，并在错误信息里带上页面标题，例如「上游返回的是网页「登录·MoMo 面板」而不是接口数据」；上游 URL、状态码和页面片段写入 AI 调用日志，任务立刻失败并退点，不再等到 600 秒超时。
- Sub2API 嵌入式前端只对 GET/HEAD 返回 SPA，非 GET 的未知路径返回 404，不再用 200 HTML 兜底。

排查顺序：在 Canvas 容器内 `curl -i -X POST "$SUB2API_RELAY_BASE_URL/images/generations"`（带服务端凭据）应得到 JSON 401 或 200；拿到 HTML 说明地址或凭据不对。线上正确地址形如 `https://api.cc2.cx` 或 `http://sub2api:8080/v1`，不要填面板域名首页。

## 验证边界

自动测试覆盖 Relay 配置、轮换、停用、错误地址、公开设置无密钥、文本/图片/视频创建的网关路径、SSO 票据和路由级会话/流式文本链路。测试使用隔离数据库和模拟上游，不产生真实模型费用。

已补上 Relay 视频完成但无公开地址时的带鉴权 content 下载，并用 HTTP 测试验证任务归属及跨用户拒绝。真实模型目录的能力约束、视频参考素材/时长/分辨率及完整轮询仍需按当前网关实现补齐端到端验收。不能仅凭模拟上游测试承诺全部媒体能力可用。
