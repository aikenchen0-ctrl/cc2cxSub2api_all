# 15 客户端 API Key 与接口手册

本文档描述当前 AgentAPI 参考实现已经提供的本地客户端凭据。它解决的是“代理用户如何把 AgentAPI 当作 OpenAI 兼容网关使用”，不是主站管理员 Key、SuperKey 或主站用户 API Key 的转发协议。

## 1. 凭据分层

| 凭据 | 保存位置 | 用途 | 是否出现在浏览器 |
| --- | --- | --- | --- |
| `SUB2API_ADMIN_KEY` | AgentAPI 服务端环境变量 | 创建主站用户、读取主站余额、受控管理动作 | 否 |
| `SUB2API_APP_CREDENTIAL` | AgentAPI 服务端环境变量 | AgentAPI 调用主站 `/v1` | 否 |
| AgentAPI Session | AgentAPI SQLite 加密会话 + HttpOnly Cookie | 控制台登录 | 仅有不可读 Cookie |
| `sk-agent-*` | AgentAPI SQLite 哈希 + 用户自己的客户端 | 调用 AgentAPI `/v1` | 只在创建响应中显示一次 |

主站凭据从不进入 AgentAPI 的前端构建产物、日志、Cookie、localStorage 或 API 响应。

## 2. 创建客户端 Key

### 2.1 页面入口

登录后打开 `/keys`，输入名称并创建。页面显示完整 Key 一次；刷新页面后只能看到前缀，无法恢复明文。

### 2.2 请求

```http
POST /api/v1/api-keys
Cookie: agentapi_session=<HttpOnly session>
Content-Type: application/json
Origin: https://agent.example.com
```

```json
{
  "name": "Desktop client"
}
```

### 2.3 响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "item": {
      "id": 7,
      "name": "Desktop client",
      "prefix": "sk-agent-abc12345",
      "status": "active",
      "created_at": "2026-09-21T12:00:00Z"
    },
    "key": "sk-agent-<只返回这一次的随机值>"
  }
}
```

服务端只保存 SHA-256 哈希和前缀。客户端应立即写入自己的密码管理器，不应把完整 Key 提交到代码仓库、截图、工单或聊天记录。

## 3. 列出和吊销 Key

### 3.1 列出

```http
GET /api/v1/api-keys
Cookie: agentapi_session=<HttpOnly session>
```

响应只包含 `id`、名称、前缀、状态、创建时间和最近使用时间，不包含完整 Key。

### 3.2 吊销

```http
DELETE /api/v1/api-keys/7
Cookie: agentapi_session=<HttpOnly session>
Origin: https://agent.example.com
```

吊销是幂等的业务操作：成功后该 Key 立即不能调用 `/v1`；不能由客户端 Key 自己创建或吊销其他 Key。

## 4. 使用模型网关

```http
POST https://agent.example.com/v1/chat/completions
Authorization: Bearer sk-agent-<客户端保存的完整值>
Content-Type: application/json
```

```json
{
  "model": "gpt-5.5",
  "messages": [
    {"role": "user", "content": "你好"}
  ]
}
```

AgentAPI 会在 JSON 请求进入余额预扣前校验 `model` 是否属于仓库公开目录（例如 `gpt-5.5`、`gpt-image-2`、`grok-imagine-video-1.5`）。未知名称会返回 `400 MODEL_NOT_ALLOWED`，不会调用主站或扣减余额；图片 multipart 请求由主站继续执行协议级解析。

AgentAPI 服务端执行以下顺序：

1. 从 Key 哈希解析 `main_user_id`；
2. 校验用户映射和状态；
3. 以 `AGENT_MAX_REQUEST_COST` 做本地预扣；
4. 按 [17-账务权威与主站扣费桥接](17-账务权威与主站扣费桥接.md) 选择主站计费身份调用主站 `/v1`，服务端添加：

   ```http
   Authorization: Bearer <SUB2API_APP_CREDENTIAL>
   X-Sub2API-On-Behalf-Of: <agent_owner_main_user_id 或 main_user_id>
   X-Sub2API-Satellite: agentapi
   ```

   方案 A 使用代理管理员 `agent_owner_main_user_id`，方案 B 使用当前代理用户 `main_user_id`；客户端不能提交或覆盖该值。
5. 主站失败时冲正预扣；成功时按可验证的 `usage_id`/主站余额差额结算，无法确认时进入 `settlement_pending`，不能直接全额退款；
6. 主站返回的 `Set-Cookie`、`Authorization` 和 `X-Sub2API-*` 不复制给客户端。

GET 类型的 `/v1/models`、视频轮询等请求也支持 AgentAPI Key；只有实际模型提交请求才进入预扣流程。

## 5. 错误和客户端重试

| 响应 | 含义 | 客户端动作 |
| --- | --- | --- |
| `401 INVALID_AGENT_API_KEY` | Key 不存在或已吊销 | 停止重试，换新 Key |
| `403 AGENT_USER_DISABLED` | 代理用户被停用 | 联系代理管理员 |
| `402 AGENT_INSUFFICIENT_BALANCE` | 用户子余额不足 | 先由代理管理员分配额度 |
| `502/503` | 主站或 AgentAPI 上游不可用 | 使用指数退避，避免重复提交非幂等请求 |
| `429` | 上游限流 | 按 `Retry-After` 或指数退避重试 |

模型提交请求的重试应由客户端生成新的 `Idempotency-Key` 策略并结合业务判断；不能因为网络超时就无限重放可能已经到达主站的请求。

## 6. 兼容边界

- AgentAPI Key 只对当前 AgentAPI 实例生效，不可拿到主站或其他代理站使用；
- AgentAPI 不接受浏览器把 `X-Sub2API-On-Behalf-Of`、`X-Sub2API-Satellite` 或 `agent_id` 作为身份凭据；
- 客户端不可调用 `/api/v1/admin/*`、`/api/v1/agent/*` 或支付管理接口；
- 模型名称必须遵守 `docs/卫星公开模型.md` 与主站实际开放列表；
- Key 泄露后的处理顺序是立即吊销、重新创建、检查主站和代理流水，不是把主站管理员 Key 发给客户端。
