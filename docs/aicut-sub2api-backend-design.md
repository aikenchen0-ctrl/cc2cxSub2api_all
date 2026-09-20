# aicut + Sub2API 后端设计

## 目标

- Sub2API 负责账号、登录、模型权限、余额和服务端 SuperKey。
- aicut 负责本地会话、租户隔离、项目/媒体归属和任务状态。
- 浏览器只持有 aicut 的 HttpOnly 会话 Cookie，不接触 Sub2API JWT、SuperKey 或用户 API Key。
- 长任务统一进入任务层，避免同一用户的请求被无限并发调用。

## SSO 链路

1. 用户在 Sub2API 登录后点击「AI剪辑」。
2. `GET /api/v1/auth/integrations/aicut/start` 校验 Sub2API 会话。
3. Sub2API 在服务端解析该用户的 SuperKey。
4. Sub2API 生成两分钟有效的 HMAC-SHA256 票据：`iss/aud/sub/email/iat/exp/jti/next`。
6. 浏览器跳转到 `OPENCHATCUT_SSO_CALLBACK_URL`。
7. aicut 校验签名、issuer、audience、时间窗和一次性 `jti`，解密 `rk`。
8. aicut 将 `sub` 保存到当前用户映射，并签发自己的 `occ_session` Cookie。
9. 后续模型请求由 aicut 服务端携带应用凭据和 `X-Sub2API-On-Behalf-Of` 转发到 Sub2API `/v1`。

共享密钥必须只配置在两个服务端：

```env
SUB2API_SSO_SECRET=<至少 32 字节随机值>
OPENCHATCUT_SSO_CALLBACK_URL=https://editor.example.com/api/auth/sso/callback
```

## aicut 用户和租户边界

当前 Gateway 的 `TenantContext` 是请求级身份边界，生产部署应将下面的逻辑数据持久化到数据库：

```text
users(id, identity_provider, external_subject, email, status, created_at, updated_at)
sessions(id_hash, user_id, expires_at, revoked_at, last_seen_at)
credentials(user_id, relay_key_ciphertext, key_id, updated_at)
projects(id, owner_user_id, name, storage_prefix, created_at, updated_at)
tasks(id, user_id, project_id, type, provider, model, status, idempotency_key, error, timestamps)
usage_ledger(id, user_id, task_id, provider, model, units, estimated_cost, created_at)
```

`external_subject` 使用 Sub2API 的稳定用户 ID，不能使用 email 作为主键。项目、媒体、导出物和生成任务必须带 `owner_user_id`，R2 路径使用 `uploads/tenants/<user-id>/...`。

## 任务和限流

短请求可以同步执行；图片、视频、音乐、语音、转写和导出应统一创建任务：

```text
POST /api/tasks       -> 202 { taskId }
GET  /api/tasks/:id   -> 状态和结果
POST /api/tasks/:id/cancel
GET  /api/tasks/:id/events
```

生产环境采用 Redis + BullMQ（或 Redis Streams）：

```text
aicut:llm
aicut:image
aicut:video
aicut:music
aicut:voice
aicut:transcription
aicut:export
```

## API Key 调用点与 MQ 边界

网关模式下，LLM 请求由 `server/plugins/llm-proxy.ts` 统一携带 `SUB2API_APP_CREDENTIAL`、`X-Sub2API-On-Behalf-Of` 和 `X-Sub2API-Satellite` 转发到 `/v1`；它需要保持流式同步，只增加用户级并发闸门、RPM/TPM 和 429 退避。

其余 Provider Key 的实际调用分散在以下插件：

- 图片：`server/plugins/image.ts`、`image-provider-clients.ts`。
- 视频：`video.ts`、`ofox-video-provider.ts`、`grok-video-provider.ts`。
- 音乐/音效：`music-*.ts`、`music-media.ts`、`sound.ts`。
- 云转写：`transcription-providers.ts`、`assemblyai-upload.ts`。
- 语音：`voice-ai-sdk.ts`、`voice-providers.ts`。
- Web/素材：`firecrawl*.ts`、`stock*.ts`；沙箱：`e2b.ts`。

图片、视频、音乐、云转写和导出属于长耗时或可重试任务，应该写入 `tasks` 表后投递 Redis + BullMQ；Worker 按 `tenant_id` 和全局额度执行，队列消息只保存任务引用，不能保存明文 Key。素材搜索和普通 TTS 可以先用用户级 QPS/日配额，超时或批量任务再进入队列。

单实例低并发时，现有进程内 `TaskLimiter` 可以先运行；多实例、多个用户共享额度、需要重启恢复任务时，Redis 队列就属于必需基础设施。数据库保存任务最终状态和用量账本，Redis 只保存排队/租约/并发状态。

Worker 执行任务时依次取得：全局并发许可、供应商并发许可、用户并发许可和 RPM/TPM 许可，然后读取当前用户 subject，使用应用凭据调用 Sub2API。任务必须使用 `(user_id, idempotency_key)` 去重，429/502/503 使用指数退避，401/403/额度不足不自动重试。

当前 aicut 的导出和转写已经有进程内 `TaskLimiter`；迁移到多实例时应把许可和队列状态移到 Redis，数据库保留任务最终状态，Redis 不作为唯一事实来源。

## 配置和部署

- 本地单机可以继续使用文件租户目录和进程内队列。
- 在线单实例至少需要数据库保存会话、任务和用量。
- 多实例需要共享数据库、Redis 和对象存储，并为 Worker 设置独立进程。
- `OCC_SESSION_SECRET` 用于 aicut 本地会话签名；`SUB2API_SSO_SECRET` 只用于跨服务 SSO，两个密钥不能混用。
