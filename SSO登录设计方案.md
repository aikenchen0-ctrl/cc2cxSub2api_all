# SSO 登录设计方案

新项目接入请优先遵循 [docs/卫星应用接入公共层.md](docs/卫星应用接入公共层.md)。本文是身份协议的详细设计。

## 1. 文档目的

本文定义一套可供多个独立项目复用的单点登录（Single Sign-On，SSO）方案。方案适用于一个统一身份中心连接多个业务应用的场景，例如控制台、画布、PPT、数据工具和移动端应用。

本文同时说明当前 `sub2api` 中转站与 `canvas` 项目的实际关系。当前实现采用“短期一次性签名票据 + 目标应用本地会话”的轻量协议；新项目优先采用标准 OAuth 2.0/OIDC，只有在受控内网或历史兼容场景下才使用本文的票据协议。

## 2. 核心概念与边界

- **身份中心（IdP）**：负责登录、密码/多因素认证、账号状态、全局登出和身份声明。可以是 sub2api，也可以是 Keycloak、Authentik、Auth0 等 OIDC 服务。
- **业务应用（Relying Party，RP）**：消费身份声明并建立自己的会话。Canvas、PPT、Livart、Ju 等都属于 RP。
- **统一身份**：由稳定的 `subject`（简称 `sub`）标识，不应使用用户名、邮箱或显示名作为主键。
- **本地会话**：业务应用自己的 Cookie 或 Bearer Session。SSO 成功后仍由业务应用签发，业务应用负责权限、资源隔离和会话续期。
- **业务授权**：角色、租户、项目权限、额度和资源访问权由业务应用或明确的授权服务决定。登录成功不等于拥有其他应用的管理员权限。

SSO 解决“用户是谁”和“是否已经登录”，不解决跨应用的数据复制、模型 API Key 共享、余额扣减、套餐同步或管理员授权。应用之间需要共享这些能力时，应设计独立的授权和计费接口。

## 3. 推荐的通用架构

生产环境推荐 OIDC Authorization Code + PKCE：

```text
浏览器 -> 业务应用 -> 身份中心授权端点
                         |
                         v
业务应用 <- 授权码 <- 身份中心
业务应用 -- 后端换 Token --> 身份中心
业务应用 -> 校验 ID Token / UserInfo -> 建立本地会话
```

1. 用户访问业务应用，应用生成随机 `state`、`nonce` 和 PKCE `code_verifier`，保存短期登录事务。
2. 浏览器跳转到身份中心授权端点。请求必须带 `client_id`、固定 `redirect_uri`、`scope=openid profile email`、`state`、`nonce` 和 PKCE 参数。
3. 用户在身份中心完成登录。身份中心仅回跳到预先注册的精确地址，并返回一次性授权码。
4. 业务应用后端使用授权码和 `code_verifier` 调用 Token Endpoint。浏览器不接触客户端密钥。
5. 应用验证 `iss`、`aud`、签名、`exp`、`iat`、`nonce`，读取 `sub` 等声明，并将外部身份映射到本地用户。
6. 应用创建自己的 HttpOnly、Secure、适当 SameSite 的会话 Cookie，清理 URL 中的 `code` 和 `state`。
7. 应用 API 只信任自己的会话，不把身份中心 Access Token 当作本地业务令牌长期保存。

### 3.1 应用注册

每个应用都应单独注册一个 OIDC Client：

- 独立 `client_id`、密钥或 PKCE 配置；
- 独立、精确的回调地址，不使用通配符；
- 独立的登出回调地址；
- 最小化 scope 和 audience；
- 开发、测试、生产使用不同 Client 和密钥；
- 应用标识应进入令牌的 `aud`，服务端必须校验，防止把发给 A 应用的令牌拿给 B 应用使用。

## 4. 当前 sub2api 与 Canvas 的实际实现

### 4.1 职责划分

`sub2api` 是统一入口和中转站：

- 用户在 sub2api 完成账号密码登录及其他认证；
- 通过 `/api/v1/auth/integrations/canvas/start`（实际路由以前缀配置为准）校验门户 JWT；
- 查询统一用户资料；
- 生成 Canvas 专用的短期一次性票据；
- 不向浏览器转发 sub2api JWT、后台 API Key 或 Relay Key；
- 通过 `SUB2API_RELAY_*` 配置向 Canvas 提供服务端模型中转，费用归配置的 API Key 所属账号。

`canvas` 是独立业务应用：

- 接收回调票据并在本地验证；
- 在本地按 `sub2api subject` 创建或查找普通用户；
- 创建 Canvas 自己的 JWT 会话；
- 之后用 Canvas 会话访问画布、素材和任务 API；
- 不把 sub2api 的角色直接转换为 Canvas 管理员角色。

因此，SSO 成功后是“同一身份、两个应用会话”，不是两个应用共用数据库用户表或共用 JWT 密钥。

### 4.2 当前票据格式

当前实现使用如下形式的签名票据：

```text
base64url(JSON payload) . base64url(HMAC-SHA256(payload, SUB2API_SSO_SECRET))
```

Payload 字段包括：

| 字段 | 含义 |
| --- | --- |
| `sub` | sub2api 的稳定用户标识，必填 |
| `email`、`username`、`displayName`、`avatarUrl` | 可选展示资料，不作为身份主键 |
| `iat` | 签发时间 |
| `exp` | 过期时间，当前约两分钟 |
| `jti` | 随机一次性编号，防重放 |
| `next` | 目标应用内部相对路径 |

sub2api 与 Canvas 必须配置相同且至少 32 个字符的 `SUB2API_SSO_SECRET`。票据应通过 HTTPS 传输，不能记录到日志、分析系统或第三方 Referer。

### 4.3 当前登录时序

```text
1. 用户已登录 sub2api
2. sub2api /integrations/canvas/start 校验门户 JWT
3. sub2api 生成 exp≈2 分钟、随机 jti 的 HMAC 票据
4. 浏览器打开 Canvas /api/auth/sso/callback?ticket=...
5. Canvas 校验签名、字段、时间窗和 next
6. Canvas 对 sha256(jti) 执行数据库原子插入，重复则拒绝
7. Canvas 按 sub2api subject 查找或创建本地普通用户
8. Canvas 设置仅用于交换的 HttpOnly handoff Cookie（约 60 秒）
9. 浏览器同源 POST /api/auth/sso/exchange
10. Canvas 校验请求头、拒绝跨站请求，读取并立即删除 handoff Cookie
11. Canvas 返回自己的 AuthSession，前端保存 Canvas 会话并跳转到安全的相对路径
```

交换 Cookie 与普通会话分离，避免把长期令牌放进回调 URL。路由和日志应使用无查询字符串的日志格式；回调和交换接口都应返回 `Cache-Control: no-store` 与 `Referrer-Policy: no-referrer`。

### 4.4 账号映射与数据隔离

Canvas 使用 `sub2api subject` 的哈希派生本地稳定 ID，首次登录创建本地普通账号；并发首次登录使用唯一约束和 `INSERT ... ON CONFLICT` 收敛。用户禁用、身份冲突或状态异常时拒绝登录。

本地用户表应保存外部身份提供方和 subject 的唯一组合，例如：

```text
identity_provider = sub2api
subject = <sub2api user id>
local_user_id = <canvas user id>
```

不能用邮箱合并账号，因为邮箱可能变更、复用或未验证。外部资料只用于展示和首次建档；昵称、头像等后续是否同步必须有明确策略。

## 5. 安全要求

- 所有票据和授权码只能一次使用，服务端持久化 `jti` 摘要并设置过期清理。
- 强制校验签名算法、签发方、受众、时间偏差、最大寿命和字段长度；拒绝控制字符。
- `next` 只能是本站允许的相对路径，拒绝 `//`、反斜杠、控制字符和外部 URL，避免开放重定向。
- 回调 URL 必须 allowlist，禁止动态拼接任意域名；生产统一 HTTPS。
- 共享密钥只放服务端密钥管理系统或受限环境变量，定期轮换。轮换期间可短暂支持双密钥并记录版本号。
- 登录、签发、交换接口限流并记录审计事件，但不得记录票据原文、JWT、Cookie 或 API Key。
- 会话 Cookie 使用 `HttpOnly`、`Secure`、`SameSite=Lax/Strict`，设置合理过期时间；高风险操作要求重新认证。
- 对 OIDC 必须校验 `state`、`nonce`、PKCE、`iss`、`aud`、签名和 `exp`。不接受只解码 JWT 不验签的实现。
- 全局登出不能只删除当前应用 Cookie。需要 OIDC Front-Channel/Back-Channel Logout，或提供每个应用撤销本地会话的机制。
- 生产禁止通过 URL 传递长期 Token、密码、sub2api JWT 或模型 API Key。

## 6. 会话、权限与计费

SSO 登录完成后，每个应用独立维护：

- 本地会话生命周期、刷新和撤销；
- 应用内角色和资源权限；
- 审计日志；
- 用户数据、项目和文件的隔离。

sub2api 的统一账号、Canvas 的本地账号和 Relay API Key 是三种不同对象：

1. 统一账号证明用户身份。
2. Canvas 本地账号承载画布和素材所有权。
3. Relay API Key 决定模型网关调用者和计费归属。

除非另有明确的服务端授权协议，Canvas 用户不会因为 SSO 自动获得 sub2api 管理员权限、渠道管理权限、余额或 API Key。服务端 Relay Key 不应下发到浏览器，也不应由用户输入覆盖。

## 7. 通用接入清单

业务应用接入时至少完成以下工作：

1. 注册独立 OIDC Client 或明确票据协议的 audience。
2. 建立 `identity_provider + subject` 唯一索引和本地用户映射表。
3. 实现回调、票据/授权码校验、一次性消费和本地会话创建。
4. 实现安全的 `next` 归一化、错误跳转和 URL 清理。
5. 设计首次登录建档、禁用账号、身份冲突、改名和删除账号策略。
6. 为登录、交换、登出、会话撤销和权限变更加入审计与限流。
7. 明确 API Key、模型调用、余额、租户和数据权限的归属。
8. 用独立测试用户覆盖成功、过期、篡改、重放、跨站、开放重定向、跨应用 audience 和跨用户资源访问。

## 8. 配置示例

通用 OIDC 配置：

```env
OIDC_ISSUER_URL=https://id.example.com
OIDC_CLIENT_ID=canvas-production
OIDC_CLIENT_SECRET=<server-side-secret>
OIDC_REDIRECT_URI=https://canvas.example.com/auth/oidc/callback
OIDC_POST_LOGOUT_REDIRECT_URI=https://canvas.example.com/login
OIDC_SCOPES=openid profile email
```

当前 sub2api/Canvas 兼容配置：

```env
# sub2api
SUB2API_SSO_SECRET=<same-random-secret-at-least-32-chars>
CANVAS_SSO_CALLBACK_URL=https://canvas.example.com/api/auth/sso/callback

# canvas
SUB2API_SSO_SECRET=<same-random-secret-at-least-32-chars>
SUB2API_RELAY_BASE_URL=https://api.example.com/v1
SUB2API_APP_CREDENTIAL=<server-side-satellite-credential>
```

回调地址、端口和反向代理协议必须按实际部署填写。开发、测试和生产不得复用密钥或用户数据卷。

## 9. 验收标准

- 已登录用户可从 sub2api 打开所有已注册应用并自动进入目标页。
- 未登录、禁用、过期、篡改、错误 audience、错误签发方和重放票据均被拒绝。
- 最终地址不含票据、授权码、JWT 或 API Key；一次性 Cookie 被清除。
- 两个 sub2api 用户在 Canvas 的项目、素材、任务和会话完全隔离。
- 刷新、重新打开和服务重启后的行为符合本地会话策略。
- 全局登出后，各应用本地会话按约定失效。
- Canvas 的模型请求只通过服务端 Relay，客户端无法读取 Relay Key，计费归属可追溯。
- 日志、监控、Referer 和错误消息中不出现敏感凭据。

## 10. 演进建议

当前 HMAC 票据适合 sub2api 与少量受控服务之间的快速集成，但共享密钥会增加密钥分发和轮换成本，也缺少标准的 audience、发现和撤销能力。新增项目或对外部署时，应迁移到 OIDC Authorization Code + PKCE；现有 Canvas 可保留兼容回调，在完成迁移后关闭 HMAC 入口。

迁移期间应同时验证两套流程，但不能让同一票据在两套入口重复消费。迁移完成后，删除旧共享密钥、旧回调和兼容代码，并保留迁移审计记录。
