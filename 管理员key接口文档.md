# 管理员 Key 接口文档

本文档按当前 `sub2api/backend/internal/server/routes/admin.go` 的实现整理，覆盖管理员 Key 可以访问的接口范围，以及开分站自动化时的实际限制。示例中的域名、ID、Key 都是占位符。

- Base URL：`https://sub2api.example.com`
- 管理接口前缀：`/api/v1/admin`
- 管理员 Key 请求头：`x-api-key: <管理员 Key>`
- 管理员 Key 只用于管理接口，不是普通模型 API Key，也不能放进上游账号 `credentials`。
- `/api/v1/admin/*` 同时支持管理员 JWT：`Authorization: Bearer <管理员 access_token>`。需要 step-up 的操作必须使用真人管理员 JWT，会话还要完成 TOTP 二次验证。

管理员 Key 的认证逻辑是：服务端读取配置中的唯一管理员 Key，校验通过后把请求身份映射为数据库中“第一个管理员用户”。因此它不是某个代理的独立身份，也没有租户范围；持有它等同于拥有当前系统管理员接口权限。不要把它下发到浏览器、agentapi 镜像或代理人员手中。

成功响应统一使用如下封装（HTTP 状态通常为 `200`，部分创建/异步接口可能返回 `201/202`）：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

分页接口的 `data` 通常是：

```json
{
  "items": [],
  "total": 0,
  "page": 1,
  "page_size": 20,
  "pages": 1
}
```

失败响应示例：

```json
{
  "code": 403,
  "message": "Admin API key cannot access this endpoint; a two-factor verified admin session is required",
  "reason": "STEP_UP_ADMIN_API_KEY_FORBIDDEN",
  "metadata": {}
}
```

当 `step_up_enabled` 关闭时，step-up 中间件不拦截；开启后，管理员 Key 会被明确拒绝所有经过 step-up 的接口。部分敏感操作即使没有路由级中间件，也会在 Handler 内部要求 step-up，例如创建管理员、把普通用户提升为管理员、清空审计日志和关闭 step-up 开关。

此外，合规确认、面板限流、简单模式和渠道监控 feature guard 仍然会生效；“路由存在”不代表在当前部署模式或配置下必然可用。

## 接口链路：管理员 Key 鉴权与连通性

### 描述

使用 `x-api-key` 访问任一 `/api/v1/admin/*` 路由即可验证 Key。下面使用合规状态接口做无副作用连通性测试。Key 错误返回 `401 INVALID_ADMIN_KEY`；未带认证返回 `401 UNAUTHORIZED`。

### 请求示例

```bash
BASE_URL="https://sub2api.example.com"
ADMIN_API_KEY="<admin-api-key>"

curl -sS "$BASE_URL/api/v1/admin/compliance" \
  -H "x-api-key: $ADMIN_API_KEY"
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "accepted": true,
    "required": true
  }
}
```

## 接口链路：全部管理路由索引

### 描述

以下是当前 `/api/v1/admin/*` 的功能域索引。每个功能域下还包含详情、批量、统计或导入导出子路由；是否需要 step-up 以本文对应链路和服务端当前配置为准。

| 功能域 | 路由前缀 | 主要能力 |
| --- | --- | --- |
| 合规 | `/compliance` | 合规状态、确认 |
| 用户 | `/users`、`/user-attributes` | 用户增删改查、角色、余额、额度、属性、用量 |
| 分组 | `/groups` | 分组、倍率、RPM、模型白名单、复合路由 |
| 上游账号 | `/accounts` | 上游凭证、模型同步、测试、刷新、配额、批量维护 |
| OAuth | `/openai`、`/gemini`、`/antigravity`、`/grok`、`/accounts/generate-*` | OAuth 授权、换码、刷新、创建账号 |
| 国内供应商 | `/cn-providers` | 账号额度、余额 |
| API Key | `/users/:id/api-keys`、`/groups/:id/api-keys`、`/api-keys/:id` | 查看 Key、调整所属分组 |
| 渠道与监控 | `/channels`、`/channel-monitors`、`/channel-monitor-templates` | 渠道、定价、监控计划 |
| 代理 | `/proxies` | 代理池、测试、质量检查、批量维护 |
| 计费 | `/redeem-codes`、`/promo-codes`、`/subscriptions` | 兑换码、优惠码、订阅分配和回收 |
| 统计 | `/dashboard`、`/usage`、`/ops` | 仪表盘、用量、并发、错误和系统日志 |
| 系统设置 | `/settings` | 品牌、邮件、限流、超时、整流器、Beta 策略等 |
| 数据与备份 | `/data-management`、`/backups` | 数据源、S3、备份和恢复 |
| 运维 | `/system`、`/plugins` | 升级、回滚、重启、插件 |
| 安全与审计 | `/risk-control`、`/prompt-audit`、`/audit-logs` | 风控、提示词审计、操作审计 |
| 其他 | `/announcements`、`/affiliates`、`/error-passthrough-rules`、`/tls-fingerprint-profiles`、`/scheduled-test-plans` | 公告、返利、错误透传、TLS 指纹、定时测试 |

### 请求示例

```bash
curl -sS "$BASE_URL/api/v1/admin/groups/all" \
  -H "x-api-key: $ADMIN_API_KEY"
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 123,
      "name": "agent-default",
      "platform": "openai",
      "status": "active"
    }
  ]
}
```

## 接口链路：查询与维护用户

### 描述

管理员 Key 可以调用：

- `GET /api/v1/admin/users`、`GET /api/v1/admin/users/:id`
- `POST /api/v1/admin/users`、`PUT /api/v1/admin/users/:id`、`DELETE /api/v1/admin/users/:id`
- `POST /api/v1/admin/users/:id/balance`
- `GET /api/v1/admin/users/:id/api-keys`
- `GET /api/v1/admin/users/:id/usage`、`GET /api/v1/admin/users/:id/balance-history`
- `POST /api/v1/admin/users/:id/replace-group`
- `GET /api/v1/admin/users/:id/rpm-status`
- `POST /api/v1/admin/users/batch-concurrency`、`POST /api/v1/admin/users/batch-limits`
- `GET/PUT/POST /api/v1/admin/users/:id/platform-quotas...`
- `GET/PUT /api/v1/admin/users/:id/attributes`

`GET /users` 支持 `page`、`page_size`、`status`、`role`、`search`、`group_name`、`api_key_group_id` 和 `attr[id]` 等筛选参数。

### 请求示例

```bash
# 查询代理用户
curl -sS "$BASE_URL/api/v1/admin/users?role=user&search=agent&page=1&page_size=20" \
  -H "x-api-key: $ADMIN_API_KEY"

# 创建普通用户（密码至少 6 位）
curl -sS -X POST "$BASE_URL/api/v1/admin/users" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "agent@example.com",
    "password": "change-me-123",
    "username": "agent",
    "role": "user",
    "balance": 0,
    "concurrency": 5,
    "rpm_limit": 0,
    "allowed_groups": [123],
    "restrict_public_groups": true
  }'
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 456,
    "email": "agent@example.com",
    "username": "agent",
    "role": "user",
    "status": "active",
    "balance": 0,
    "concurrency": 5,
    "rpm_limit": 0,
    "allowed_groups": [123]
  }
}
```

## 接口链路：创建或提升代理管理员

### 描述

`POST /api/v1/admin/users` 的 `role: "admin"`，或者 `PUT /api/v1/admin/users/:id` 把已有用户的 `role` 改为 `admin`，都属于敏感的角色变更。目标原本不是管理员时，Handler 会强制 step-up。

因此：

- step-up 关闭时，当前管理员 Key 可以执行这两类请求；
- step-up 开启时，管理员 Key 会返回 `403 STEP_UP_ADMIN_API_KEY_FORBIDDEN`，必须改用已启用 TOTP 且刚完成 step-up 的真人管理员 JWT；
- 不能让普通管理员把自己降级为普通用户；
- 仅给前端隐藏“管理员”按钮不能实现代理模式权限隔离，后端仍需增加代理标记/角色和余额策略。

### 请求示例

```bash
# 需要 step-up 关闭，或改用真人管理员 JWT
curl -sS -X PUT "$BASE_URL/api/v1/admin/users/456" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"role":"admin","status":"active"}'
```

### 响应示例

成功时：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 456,
    "email": "agent@example.com",
    "role": "admin",
    "status": "active"
  }
}
```

step-up 开启时：

```json
{
  "code": 403,
  "message": "Admin API key cannot access this endpoint; a two-factor verified admin session is required",
  "reason": "STEP_UP_ADMIN_API_KEY_FORBIDDEN"
}
```

## 接口链路：创建分组、设置倍率和路由

### 描述

分组接口用于决定平台、模型、计费倍率、RPM、可用账号和复合路由。常用接口包括：

- `GET /api/v1/admin/groups`、`GET /api/v1/admin/groups/all`、`GET /api/v1/admin/groups/:id`
- `POST/PUT/DELETE /api/v1/admin/groups` 及 `/:id`
- `POST /api/v1/admin/groups/:id/duplicate`
- `GET /api/v1/admin/groups/:id/stats`、`GET /api/v1/admin/groups/:id/api-keys`
- `GET/PUT/DELETE /api/v1/admin/groups/:id/rate-multipliers`
- `PUT/DELETE /api/v1/admin/groups/:id/rpm-overrides`
- `GET/POST/PUT/DELETE /api/v1/admin/groups/:id/composite-routes...`

支持的常见 `platform` 有 `openai`、`anthropic`、`gemini`、`antigravity`、`grok`、`kimi`、`zhipu`、`deepseek`、`minimax`、`opencode_go` 和 `composite`。

### 请求示例

```bash
curl -sS -X POST "$BASE_URL/api/v1/admin/groups" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "agent-egoi-openai",
    "description": "代理默认分组",
    "platform": "openai",
    "rate_multiplier": 1.0,
    "is_exclusive": false,
    "rpm_limit": 0
  }'
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 123,
    "name": "agent-egoi-openai",
    "platform": "openai",
    "rate_multiplier": 1,
    "is_exclusive": false,
    "status": "active"
  }
}
```

## 接口链路：创建主站上游账号与同步模型

### 描述

管理员 Key 可以创建、修改、删除和测试上游账号，也可以刷新账号、重置配额、清理错误、设置可调度状态、同步模型以及查看账号统计：

- `GET/POST/PUT/DELETE /api/v1/admin/accounts` 及 `/:id`
- `POST /api/v1/admin/accounts/:id/test`、`/refresh`、`/clear-error`、`/reset-quota`、`/schedulable`
- `GET /api/v1/admin/accounts/:id/models`、`POST /api/v1/admin/accounts/:id/models/sync-upstream`
- `GET /api/v1/admin/accounts/:id/usage`、`/stats`、`/today-stats`
- `POST /api/v1/admin/accounts/batch`、`/bulk-update`、`/batch-refresh`、`/batch-delete` 等批量接口
- 各平台 OAuth、上游计费探测和 CRS 同步接口

给 agentapi 配置主站上游时，`credentials` 中应放专门创建的、权限受限的普通上游凭证和主站 URL；严禁放入主站管理员 Key、SuperKey 或数据库凭证。账号详情和列表会对敏感凭证脱敏，但导出接口可能包含完整凭证。

### 请求示例

```bash
curl -sS -X POST "$BASE_URL/api/v1/admin/accounts" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "agent-egoi-main-upstream",
    "platform": "openai",
    "type": "apikey",
    "credentials": {
      "api_key": "<专用受限上游凭证>",
      "base_url": "https://sub2api.example.com/v1"
    },
    "group_ids": [123],
    "concurrency": 5,
    "priority": 50
  }'
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 789,
    "name": "agent-egoi-main-upstream",
    "platform": "openai",
    "type": "apikey",
    "status": "active",
    "group_ids": [123],
    "credentials": {
      "api_key": "********",
      "base_url": "https://sub2api.example.com/v1"
    }
  }
}
```

## 接口链路：查看 API Key 并绑定分组

### 描述

管理员 Key 可以查看用户或分组下的 API Key，并修改已有 Key 的分组：

- `GET /api/v1/admin/users/:id/api-keys`
- `GET /api/v1/admin/groups/:id/api-keys`
- `PUT /api/v1/admin/api-keys/:id`

管理员 Key **不能**直接调用普通用户 JWT 路由 `POST /api/v1/keys` 创建、`PUT /api/v1/keys/:id` 修改或 `DELETE /api/v1/keys/:id` 删除用户 Key。开分站脚本如果需要自动生成代理用户 Key，当前必须让用户登录后自助创建，或者另行增加受保护的 provisioning 接口；不能假设管理员 Key 已经具备该能力。

### 请求示例

```bash
# 先读取目标用户的 Key
curl -sS "$BASE_URL/api/v1/admin/users/456/api-keys?page=1&page_size=20" \
  -H "x-api-key: $ADMIN_API_KEY"

# 将已有 Key 绑定到分组 123
curl -sS -X PUT "$BASE_URL/api/v1/admin/api-keys/987" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"group_id":123,"reset_rate_limit_usage":true}'
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 987,
    "group_id": 123,
    "reset_rate_limit_usage": true
  }
}
```

## 接口链路：渠道、代理和模型路由

### 描述

管理员 Key 可以维护渠道及其模型映射、维护代理池，并配置错误透传规则和 TLS 指纹模板：

- 渠道：`GET/POST/PUT/DELETE /api/v1/admin/channels`，以及 `/model-pricing`、`/pricing/sync-models`
- 代理：`GET/POST/PUT/DELETE /api/v1/admin/proxies`，以及 `/test`、`/quality-check`、`/stats`、`/accounts` 和批量接口
- 错误透传：`GET/POST/PUT/DELETE /api/v1/admin/error-passthrough-rules...`
- TLS 指纹：`GET/POST/PUT/DELETE /api/v1/admin/tls-fingerprint-profiles...`

这些接口影响上游选择、模型映射和错误处理；真正的上游密钥仍在账号接口中配置。

### 请求示例

```bash
curl -sS -X POST "$BASE_URL/api/v1/admin/channels" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "agent-egoi-channel",
    "description": "代理上游渠道",
    "group_ids": [123],
    "restrict_models": false,
    "features": "",
    "features_config": {},
    "model_mapping": {}
  }'
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 321,
    "name": "agent-egoi-channel",
    "group_ids": [123],
    "restrict_models": false,
    "model_mapping": {}
  }
}
```

## 接口链路：余额、兑换码和订阅

### 描述

余额和计费相关接口包括：

- `POST /api/v1/admin/users/:id/balance`：`set`、`add`、`subtract`
- `/api/v1/admin/redeem-codes`：列表、统计、导出、生成、创建并兑换、批量修改/删除、失效
- `/api/v1/admin/promo-codes`：优惠码增删改查和使用记录
- `/api/v1/admin/subscriptions`：分配、批量分配、延长、重置配额、撤销、恢复；另有按用户/分组查询

写入型计费操作应使用稳定的 `Idempotency-Key`，避免重试造成重复充值。管理员 Key 能调整任何用户余额；如果代理管理员不应自行充值或改价，必须在后端增加 agent 模式限制，不能只依赖界面隐藏。

### 请求示例

```bash
# 给用户增加余额
curl -sS -X POST "$BASE_URL/api/v1/admin/users/456/balance" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: agent-456-recharge-20260921-001" \
  -d '{"balance":100,"operation":"add","notes":"代理充值"}'

# 创建兑换码并直接兑换到指定用户
curl -sS -X POST "$BASE_URL/api/v1/admin/redeem-codes/create-and-redeem" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: order-agent-001" \
  -d '{"code":"order_agent_001","type":"balance","value":100,"user_id":456,"notes":"代理充值"}'
```

### 响应示例

余额接口返回更新后的用户对象（字段可能随 DTO 版本增加）：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 456,
    "balance": 100,
    "email": "agent@example.com",
    "status": "active"
  }
}
```

`create-and-redeem` 返回的 `data` 通常包在 `redeem_code` 字段中：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "redeem_code": {
      "id": 1001,
      "code": "order_agent_001",
      "type": "balance",
      "value": 100,
      "status": "used",
      "used_by": 456
    }
  }
}
```

## 接口链路：并发、RPM、平台额度和倍率

### 描述

可以通过用户、分组和账号接口设置资源限制：

- 用户：`POST /api/v1/admin/users/batch-concurrency`、`POST /api/v1/admin/users/batch-limits`
- 用户平台额度：`GET/PUT /api/v1/admin/users/:id/platform-quotas`、`POST /reset`
- 用户分组替换和专属倍率：`POST /api/v1/admin/users/:id/replace-group`、用户更新中的 `group_rates`
- 分组倍率与 RPM：`/api/v1/admin/groups/:id/rate-multipliers`、`/rpm-overrides`
- 账号并发、优先级、倍率和调度状态：`PUT /api/v1/admin/accounts/:id`、`POST /schedulable`

这些限制可用于给代理设置额度上限，但现有管理员接口默认是全局管理员权限，并不会自动把资源限制绑定到某个代理租户。

### 请求示例

```bash
curl -sS -X PUT "$BASE_URL/api/v1/admin/users/456/platform-quotas" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "quotas": [
      {
        "platform": "openai",
        "daily_limit_usd": 20,
        "weekly_limit_usd": 100,
        "monthly_limit_usd": 300
      }
    ]
  }'
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "platform_quotas": [
      {
        "platform": "openai",
        "daily_limit_usd": 20,
        "weekly_limit_usd": 100,
        "monthly_limit_usd": 300,
        "daily_used_usd": 0
      }
    ]
  }
}
```

## 接口链路：用量、仪表盘、运维监控和审计

### 描述

管理员 Key 可以读取全局用量和仪表盘数据，也可以读取运维监控信息：

- 用量：`GET /api/v1/admin/usage`、`/stats`、`/search-users`、`/search-api-keys`、清理任务接口
- 仪表盘：`/api/v1/admin/dashboard/stats`、`/realtime`、`/trend`、`/models`、`/groups`、`/users-ranking`、批量用量接口
- 运维：`/api/v1/admin/ops/concurrency`、`/user-concurrency`、`/account-availability`、流量、错误、请求详情、系统日志和告警接口
- 审计：`GET /api/v1/admin/audit-logs`、`GET /api/v1/admin/audit-logs/:id`

清空审计日志 `POST /api/v1/admin/audit-logs/clear` 由 Handler 强制要求真人 TOTP，管理员 Key 不能直接执行。

### 请求示例

```bash
curl -sS "$BASE_URL/api/v1/admin/dashboard/stats?days=7" \
  -H "x-api-key: $ADMIN_API_KEY"

curl -sS "$BASE_URL/api/v1/admin/usage?user_id=456&page=1&page_size=20" \
  -H "x-api-key: $ADMIN_API_KEY"
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "requests": 1280,
    "tokens": 845000,
    "cost": 12.34,
    "active_accounts": 3,
    "active_users": 18
  }
}
```

## 接口链路：系统设置、Logo 与站点品牌

### 描述

系统设置接口包括：

- `GET/PUT /api/v1/admin/settings`：注册、登录、邮件、品牌、首页内容、默认额度、返利等
- `GET/PUT /api/v1/admin/settings/panel-rate-limit`、`/stream-timeout`、`/rectifier`、`/beta-policy`
- `GET/PUT /api/v1/admin/settings/overload-cooldown`、`/rate-limit-429-cooldown`
- `GET/PUT /api/v1/admin/settings/web-search-emulation` 以及测试、额度重置
- 邮件模板列表、预览、修改和恢复

Logo、站点名、副标题、首页内容等 OEM 字段位于 `/settings` 的 `site_logo`、`site_name`、`site_subtitle`、`home_content` 等字段中。该接口的请求体字段很多，推荐先 GET 当前完整设置，在服务端合并目标字段后再 PUT；不要用只含一个字段的脚本 payload 覆盖未知设置。开启或关闭 step-up 本身有额外 TOTP 条件。

### 请求示例

```bash
# 先读取当前配置
curl -sS "$BASE_URL/api/v1/admin/settings" \
  -H "x-api-key: $ADMIN_API_KEY"

# 下面展示需要合并到完整设置对象中的 OEM 字段
curl -sS -X PUT "$BASE_URL/api/v1/admin/settings" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "site_name": "Egoi AI",
    "site_logo": "https://cdn.example.com/egoi/logo.svg",
    "site_subtitle": "Agent API",
    "home_content": "欢迎使用"
  }'
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "site_name": "Egoi AI",
    "site_logo": "https://cdn.example.com/egoi/logo.svg",
    "site_subtitle": "Agent API",
    "home_content": "欢迎使用"
  }
}
```

## 接口链路：OAuth 和供应商账号

### 描述

管理员 Key 可以驱动上游 OAuth 和供应商管理流程：

- OpenAI：`/api/v1/admin/openai/generate-auth-url`、换码、刷新、从 OAuth/Codex PAT 创建账号、查询/刷新/重置配额
- Gemini：`/api/v1/admin/gemini/oauth/auth-url`、`exchange-code`、`capabilities`
- Antigravity：`/api/v1/admin/antigravity/oauth/auth-url`、换码、刷新
- Grok：能力查询、授权、刷新、SSO token、密码、从 OAuth/SSO 创建和 reconcile
- 通用账号 OAuth：`/api/v1/admin/accounts/generate-auth-url`、`/generate-setup-token-url`、`/exchange-code`、`/exchange-setup-token-code`、`/cookie-auth` 等
- 国内供应商：`/api/v1/admin/cn-providers/accounts/:id/quota` 和 `/balance`

授权码、refresh token、cookie、PAT 等都是敏感数据；只应在服务端短期保存，响应和日志不得直接转发给代理用户。

### 请求示例

```bash
curl -sS -X POST "$BASE_URL/api/v1/admin/openai/generate-auth-url" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"redirect_uri":"https://sub2api.example.com/admin/oauth/callback"}'
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "auth_url": "https://provider.example.com/oauth/authorize?...",
    "state": "oauth_state_001"
  }
}
```

## 接口链路：公告、风控、提示词审计和返利

### 描述

管理员 Key 可以管理公告，读取和处理风控记录，配置提示词审计，以及查看/调整邀请返利：

- 公告：`GET/POST/PUT/DELETE /api/v1/admin/announcements...`
- 风控：`/api/v1/admin/risk-control/config`、`status`、`logs`、用户解封和 hash 清理
- 提示词审计：`/api/v1/admin/prompt-audit/config`、运行状态、事件查询、删除和探测
- 返利：`/api/v1/admin/affiliates/invites`、`rebates`、`transfers` 及用户设置接口

这些接口会影响全站用户或合规数据，代理管理员不应直接持有调用权限。

### 请求示例

```bash
curl -sS -X POST "$BASE_URL/api/v1/admin/announcements" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "系统维护通知",
    "content": "今晚 23:00 进行维护",
    "enabled": true
  }'
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 12,
    "title": "系统维护通知",
    "enabled": true
  }
}
```

## 接口链路：数据管理、备份、插件与系统运维

### 描述

管理员 Key 可以读取和执行大量运维接口：

- 数据管理：`/api/v1/admin/data-management` 的配置、数据源 profile、S3 profile、备份任务
- 备份：`/api/v1/admin/backups` 的配置、创建、列表、删除、下载链接和恢复
- 插件：`/api/v1/admin/plugins` 的查询、UI session、上传、启停、删除、配置和测试
- 系统：`/api/v1/admin/system/version`、`check-updates`、`rollback-versions`、`update`、`rollback`、`restart`
- 定时测试：`/api/v1/admin/scheduled-test-plans`

以下接口需要 step-up，因此不能只用管理员 Key（step-up 关闭时中间件可能放行，但仍应按敏感操作处理）：

- `GET /api/v1/admin/accounts/data`、`GET /api/v1/admin/proxies/data` 导出
- S3 profile 创建/修改/激活、备份创建、备份下载链接和恢复
- `/api/v1/admin/backups` 中修改 S3/图片存储配置
- 插件上传、启停、删除、保存配置、测试

账号和代理导出、备份下载/恢复可能包含凭证或完整数据库数据，绝不能交给代理脚本调用。

### 请求示例

```bash
# 读取版本（普通管理员 Key 可调用）
curl -sS "$BASE_URL/api/v1/admin/system/version" \
  -H "x-api-key: $ADMIN_API_KEY"

# 下面的备份创建在 step-up 开启时必须改用真人管理员 JWT + TOTP
curl -sS -X POST "$BASE_URL/api/v1/admin/backups" \
  -H "Authorization: Bearer <admin-jwt-with-step-up>" \
  -H "Content-Type: application/json" \
  -d '{"type":"full"}'
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "version": "2.0.0",
    "commit": "abc1234",
    "update_available": false
  }
}
```

## 接口链路：读取、轮换和删除管理员 Key

### 描述

管理员 Key 自身也由管理设置接口维护：

- `GET /api/v1/admin/settings/admin-api-key`：只返回是否存在和掩码
- `POST /api/v1/admin/settings/admin-api-key/regenerate`：生成新 Key，完整值只在本次响应返回
- `DELETE /api/v1/admin/settings/admin-api-key`：删除 Key，删除后所有 `x-api-key` 请求立即失效

轮换后旧 Key 不可继续使用；生产脚本必须安全保存新 Key，不能写入浏览器、日志或 Git。

### 请求示例

```bash
curl -sS "$BASE_URL/api/v1/admin/settings/admin-api-key" \
  -H "x-api-key: $ADMIN_API_KEY"

curl -sS -X POST "$BASE_URL/api/v1/admin/settings/admin-api-key/regenerate" \
  -H "x-api-key: $ADMIN_API_KEY"
```

### 响应示例

查询状态：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "exists": true,
    "masked_key": "sk-admin-********abcd"
  }
}
```

轮换响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "key": "sk-admin-<仅此一次返回的完整值>"
  }
}
```

## 接口链路：开分站自动化的可行链路与缺口

### 描述

按照“主站统一计费、代理使用 agentapi”的方案，主站后端可以用管理员 Key 在服务端编排以下步骤：

1. `POST /admin/users` 创建代理用户（建议先创建 `role: user`）。
2. `POST /admin/groups` 创建或复用代理分组，设置平台、倍率、RPM 和模型范围。
3. `POST /admin/accounts` 创建指向主站 `/v1` 的专用上游账号，并绑定代理分组。
4. 通过 `PUT /admin/users/:id`、`/platform-quotas`、`/batch-limits` 设置代理额度和并发。
5. 通过普通用户登录流程创建 API Key，再由 `PUT /admin/api-keys/:id` 绑定分组；当前管理员 Key 没有“直接为用户生成 API Key”的接口。
6. 如确实要让该用户登录管理控制台，再把角色提升为 `admin`；这一步在 step-up 开启时必须由真人管理员 JWT + TOTP 完成。

当前接口模型存在三个不能忽略的缺口：

- **管理员 Key 是全局 Key**：它映射到首个管理员，没有代理/租户范围。不能把它发给代理管理员，否则代理可读取、修改或删除全站用户、账号、余额、设置和运维数据。
- **代理管理员仍是普通 admin**：现有后端没有“只能设置销售价、不能改余额、不能看其他代理数据”的 agent 角色。余额限制必须新增后端权限判断，不能只改前端。
- **缺少管理员代用户创建 Key 的接口**：自动开站脚本不能仅凭管理员 Key 完成“创建用户 + 创建其 API Key”的闭环，需要用户自助创建、主站新增受保护 provisioning 接口，或由独立 agentapi 初始化服务使用一次性注册凭证完成。

建议的安全边界是：主站管理员 Key 只保存在主站编排服务；agentapi 只保存专用的、可撤销的主站普通上游凭证；代理端不接触管理员 Key、SuperKey、OAuth refresh token、账号导出和备份接口。

### 请求示例

```bash
# 主站编排服务：创建代理用户
curl -sS -X POST "$BASE_URL/api/v1/admin/users" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "email":"agent@example.com",
    "password":"temporary-password-123",
    "username":"agent-egoi",
    "role":"user",
    "balance":0,
    "concurrency":5,
    "allowed_groups":[123]
  }'

# 用户完成登录后，再由主站编排服务绑定其已创建的 Key
curl -sS -X PUT "$BASE_URL/api/v1/admin/api-keys/987" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"group_id":123}'
```

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "provisioned": true,
    "user_id": 456,
    "group_id": 123,
    "upstream_account_id": 789,
    "api_key_created_by_admin_key": false,
    "next_step": "user_login_and_create_api_key"
  }
}
```

上面的最后一个响应是编排服务应返回的业务结果示例，不是当前 Sub2API 原生接口的固定响应；当前系统原生接口会分别返回用户、分组、账号或 API Key 对象。

## 接口链路：管理员 Key 与其他凭证的边界

### 描述

三类凭证不能混用：

| 凭证 | 用途 | 是否可访问 `/api/v1/admin/*` | 是否可作为 `/v1` 模型 Key |
| --- | --- | ---: | ---: |
| 管理员 Key（`x-api-key`） | 主站后台自动化 | 是 | 否 |
| 管理员 JWT（`Authorization: Bearer`） | 管理后台和 step-up | 是（需 admin 角色） | 否 |
| 普通用户 API Key / SuperKey | 模型调用或用户自助接口 | 否 | 按其自身协议使用；SuperKey 不是普通 API Key |

`sk-super-*` 不能当普通 API Key 放入 `/v1` 的 Bearer 或 `x-api-key` 请求中，也不能放进上游账号配置。对外提供模型调用时，应为对应用户或 agentapi 创建专用普通 API Key，并按分组和额度管理。

### 请求示例

```bash
# 正确：管理员 Key 调用后台接口
curl -sS "$BASE_URL/api/v1/admin/users/456" \
  -H "x-api-key: $ADMIN_API_KEY"

# 错误用法：不要把管理员 Key 当模型 Key
curl -sS "$BASE_URL/v1/models" \
  -H "Authorization: Bearer $ADMIN_API_KEY"
```

### 响应示例

以下是模型网关拒绝管理员 Key 的示意，实际错误封装取决于调用的 `/v1` 网关路由：

```json
{
  "code": 401,
  "message": "Invalid API key"
}
```
