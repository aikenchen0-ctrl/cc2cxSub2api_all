# 影策 / ju.cc2.cx 接入 Sub2API Super Key 实施提示词

你是一名负责端到端交付的高级全栈工程师。请在现有定制项目中完成影策（`ju.cc2.cx`）的视频链路接入，并完成构建、部署和真实视频验收。不要只给方案，必须实施、测试并汇报证据。

## 一、最终目标

实现以下链路：

```text
用户登录 Sub2API
  -> 通过 SSO 登录 ju.cc2.cx / 影策
  -> 影策后端使用服务端配置的 Sub2API Super Key
  -> 所有视频请求发送到 Sub2API
  -> Sub2API /v1/videos*
  -> Sub2API 根据模型调度 AutoDL 上游视频账号
  -> 创建、轮询、下载保持相同任务归属
  -> 视频进入影策资源与项目
```

必须保留当前定制版 Sub2API、Canvas 和用户已有改动，不得用上游仓库或旧镜像覆盖，不得回滚无关变更。

## 二、项目路径和重点文件

项目根目录：

```text
C:\Users\kodesh\Desktop\Workspace\sub2api-pro
```

影策前端重点文件：

```text
ju/web/src/services/api/video.ts
ju/web/src/services/api/video-provider-openai.ts
ju/web/src/services/api/video-provider-newapi.ts
ju/web/src/services/api/video-transport.ts
ju/web/src/services/api/channel-transport.ts
ju/web/src/services/api/custom-channel-relay.ts
ju/web/src/stores/use-config-store.ts
ju/web/src/lib/model-capabilities.ts
```

影策后端重点文件：

```text
ju/backend/internal/app/sub2api_integration.go
ju/backend/internal/auth/sso.go
ju/backend/internal/handler/auth.go
ju/backend/internal/handler/api.go
ju/backend/internal/handler/custom_proxy.go
ju/nginx.conf
ju/docker-compose.yml
ju/docker-compose.server.yml
ju/docker-compose.deploy.yml
```

Sub2API 对照文件：

```text
sub2api/backend/internal/handler/ju_sso.go
sub2api/backend/internal/service/grok_media.go
sub2api/backend/internal/service/mediaadapter/autodl.go
sub2api/backend/internal/server/routes/gateway.go
autodl-api/autodComfyuiApil.md
autodl-api/autodl-art-comfyui-api-对接提示词.md
```

开始修改前逐个阅读相关代码和文档，并检查仓库状态，保护已有修改。

## 三、当前已知阻断点

1. `EnsureSub2APIRelayChannel()` / `prepareSub2APIModel()` 当前把 relay 模型统一登记为 `Capability=text`、`Protocol=chat-completion`，AutoDL 视频模型因此不能正确路由。
2. 影策普通 OpenAI 视频 provider 创建任务时使用 `multipart/form-data`，而 Sub2API AutoDL 适配器要求 JSON。
3. 现有 NewAPI provider 使用 `/v1/video/generations`，但 Sub2API 视频合同是：

```text
POST /v1/videos
POST /v1/videos/generations
GET  /v1/videos/{id}
GET  /v1/videos/{id}/content
```

4. SSO 只负责用户登录身份，不传递 Super Key。Super Key 必须仅由影策后端持有。
5. AutoDL workflow 的字段结构并不统一，不能给每个模型发送同一套通用字段。

## 四、鉴权和安全设计

沿用已有 SSO：

```text
GET /api/v1/auth/integrations/ju/start
GET https://ju.cc2.cx/api/auth/sso/callback?ticket=...
```

两端配置相同且不少于 32 字符的：

```text
SUB2API_SSO_SECRET
```

Sub2API 配置：

```text
JU_SSO_CALLBACK_URL=https://ju.cc2.cx/api/auth/sso/callback
```

Super Key 只允许配置在影策后端环境变量：

```text
SUB2API_RELAY_API_KEY=<通过部署环境注入，不写入仓库>
```

前端系统渠道只保存 `apiKey: "system"`，浏览器只携带影策 session cookie。严禁把 Super Key 放入 URL、query、hash、localStorage、IndexedDB、项目数据、任务正文、日志、截图、测试或提交记录。

## 五、后端实施要求

### 1. 正确注册视频模型

修改 `ju/backend/internal/app/sub2api_integration.go`：

- 文本模型继续使用 `Capability=text`、`Protocol=chat-completion`。
- AutoDL 视频模型使用正确的视频 capability 和能走 `/v1/videos` 的协议。
- 优先增加显式的 `SUB2API_RELAY_VIDEO_MODELS` 配置或共享模型注册表，不要只用模型名是否包含 `video` 来猜测。
- 检查 `ChannelAPIURLForProtocol()` 的路径拼接，禁止产生 `/v1/v1/videos` 或错误的 `/v1/video/generations`。

### 2. 建立专用 Sub2API JSON 视频 provider

不要直接复用普通 OpenAI multipart 创建逻辑。影策浏览器调用自身后端系统代理：

```text
POST /api/sub2api-relay/videos
GET  /api/sub2api-relay/videos/{id}
GET  /api/sub2api-relay/videos/{id}/content
```

影策后端使用 `SUB2API_RELAY_API_KEY` 转发到：

```text
POST {SUB2API_RELAY_BASE_URL}/videos
GET  {SUB2API_RELAY_BASE_URL}/videos/{id}
GET  {SUB2API_RELAY_BASE_URL}/videos/{id}/content
```

创建请求必须为 `application/json`。基础字段可包括：

```json
{
  "model": "minimax_h3_b99_001",
  "prompt": "...",
  "seconds": "6",
  "duration": 6,
  "resolution": "768p",
  "aspect_ratio": "16:9",
  "images": [],
  "audios": [],
  "videos": []
}
```

但必须按 workflow 裁剪字段，禁止无条件发送所有字段。

### 3. workflow 参数规则

以 AutoDL 文档和 Sub2API 当前适配器实现为准，至少覆盖：

- `minimax_h3_b99_001`：文生视频。
- `minimax_h3_b99_002`：`images[0] -> first_frame`，`images[1] -> last_frame`。
- `minimax_h3_z0903`：正确映射多图和多音频字段，如 `ref_image_0`、`ref_audio_0`。
- `wan2.2animate-v4-motion_retargeting`：仅发送其接受的 `seed`、`ref_image`、`ref_video`、`resolution`；不得附加通用 `prompt`、`duration` 或编号引用字段。

逐一核对其余 workflow。不能凭经验假设字段；以 `autodComfyuiApil.md` 和实际接口响应为准。

### 4. 异步任务合同

创建结果兼容提取：

```json
{"id":"...","status":"queued"}
{"task_id":"..."}
{"request_id":"..."}
```

任务对象必须固定保存：

```text
id
英文 model ID
provider
channelId
创建时的 endpoint 配置
```

创建、轮询、content 必须使用同一个逻辑渠道和 Super Key。状态至少兼容：

```text
queued, pending, running, processing,
success, succeeded, completed, done,
failed, failure, cancelled
```

完成后从 Sub2API `/content` 获取结果，不得将 AutoDL 短时效 URL写入项目。

### 5. 错误处理

- 400：模型参数或 workflow 字段错误。
- 401/403：Super Key 无效、禁用或无视频权限。
- 404：任务不存在、归属丢失或旧任务。
- 429：额度、并发或频率限制。
- 502/504：AutoDL 上游或下载失败。

400/401/403/404 应终止当前任务轮询并显示失败原因，禁止无限重试。不能把 HTTP 200 中的业务失败误判成成功。

## 六、中文模型显示与英文 ID

前端只把中文作为 `displayName`，option value、任务存储和请求 payload 必须保留英文模型 ID。中文名称存在重复，不能作为唯一主键，建议使用英文 ID 或 `channelId::英文ID`。

至少配置以下映射：

| 中文显示名 | 英文模型 ID |
| --- | --- |
| H3六图三音频生视频（高质量音画融合） | `minimax_h3_z0903` |
| H3六图生视频（多图一致性创作） | `minimax_h3_z0902` |
| H3文生视频（高质量创意直出） | `minimax_h3_z0901` |
| H3多图多音频生视频（升级画质） | `minimax_h3_zm_u24` |
| H3多图多音频生视频（高速版） | `minimax_h3_zm_u08` |
| H3首尾帧生成视频 | `minimax_h3_b99_002` |
| H3文生视频 | `minimax_h3_b99_001` |
| H3多图生视频12秒 | `minimax_h3_b99_003_12s` |
| 动作迁移 | `wan2.2animate-v4-motion_retargeting` |
| H3多图多音频生视频15秒 | `minimax_h3_image_audio_to_video_v2_15s` |
| H3多图生视频15秒 | `minimax_h3_lightx2v_v5_15s` |
| H3多图多音频生视频 | `minimax_h3_image_audio_to_video_v2` |
| H3图生视频-音频同步（自动对口型） | `minimax_h3_image_audio_to_video` |
| H3多图参考生视频 | `minimax_h3_lightx2v_v5` |
| H3文生视频 | `minimax_h3_lightx2v_no_pic` |
| H3首尾帧生成视频 | `minimax_h3_lightx2v` |

必须继续对照文档确认第 17 个模型，并纳入同一注册表，不能遗漏。

## 七、Docker 与 Nginx

生产环境至少配置：

```text
SUB2API_SSO_SECRET=<两端一致的随机密钥>
SUB2API_RELAY_BASE_URL=http://sub2api:8080/api/v1
SUB2API_RELAY_API_KEY=<仅部署环境注入>
SUB2API_RELAY_MODELS=<英文模型 ID 列表>
SUB2API_RELAY_VIDEO_MODELS=<英文视频模型 ID 列表>
SUB2API_RELAY_ALLOW_LOCAL=false
CANVAS_CORS_ORIGINS=https://ju.cc2.cx
```

若不在同一 Docker 网络，通过实际 HTTPS API 域名访问。检查 Nginx：

- `/api/` 代理不得破坏完整路径。
- 支持较大的图片、音频和视频上传。
- 视频 content 支持二进制和 Range 响应。
- 任务状态和 content 不得缓存。
- 适当增加 `proxy_read_timeout`。
- 跨域 origin 必须精确，不要在 credentials 场景使用 `*`。
- session cookie 使用合适的 Secure、HttpOnly、SameSite。
- Sub2API 代理保留必要请求头，并按部署要求启用 `underscores_in_headers on;`。

构建镜像时必须从当前本地定制源码构建。完成后强制重建相关容器，并确认健康检查和容器实际镜像 ID。

## 八、测试与验收

### 自动化测试

至少补充：

- 视频模型 capability / protocol 注册测试。
- 中文显示名到英文 ID 的测试，特别覆盖重复中文名称。
- JSON payload 和各 workflow 字段裁剪测试。
- 创建响应任务 ID 兼容测试。
- 状态归一化测试。
- 404 终止轮询测试。
- Super Key 不进入前端持久化数据的安全测试。
- SSO ticket 过期、防重放测试。

### 真实 API 验收

真实 Key 只能通过环境变量注入，不得写入命令历史记录之外的项目文件或最终报告。完成：

1. 使用 `minimax_h3_b99_001` 创建文生视频任务。
2. 持续轮询到终态。
3. 调用 `/content`，使用 `Range: bytes=0-99`。
4. 期望 HTTP 200 或 206、`Content-Type: video/mp4`，Range 正常时应为 206。
5. 至少再验证一个图片 workflow；测试素材可用安全的临时资源。
6. 若具备音频和视频素材，再验证 z0903 和 Wan；若缺素材，明确报告未覆盖项，不可伪造通过。

### 浏览器验收

在 `https://ju.cc2.cx` 实际登录并生成视频：

- 模型选择器显示中文名称。
- Network 创建 payload 使用英文模型 ID。
- 请求只走 `/api/sub2api-relay/videos...` 或等价的影策后端系统代理路径。
- 浏览器不直连 AutoDL 或其他视频供应商。
- 创建、轮询、content 使用同一任务 ID。
- 404 不会无限轮询。
- 视频完成后可播放并进入影策资源。
- Request URL、Referer、Storage、页面数据和前端日志中均没有 Super Key。

## 九、禁止事项

- 不得覆盖或回滚用户定制版 Sub2API、Canvas 或无关修改。
- 不得把 Super Key 或 AutoDL Token 写入代码、文档、测试、日志或浏览器存储。
- 不得让浏览器直连 AutoDL。
- 不得把所有 relay 模型继续注册为文本模型。
- 不得让普通 multipart provider 直接调用 AutoDL JSON workflow。
- 不得给所有 workflow 强行附加统一字段。
- 不得把中文显示名发送给 Sub2API。
- 不得对 400/401/403/404 无限重试。
- 不得仅凭页面能打开就声称验收成功。

## 十、最终汇报格式

完成后按以下顺序汇报：

1. 修改文件及核心行为。
2. 自动化测试命令和通过数量。
3. Docker 镜像、容器、端口和健康状态。
4. 真实任务 ID、模型英文 ID、最终状态和 content HTTP 结果；隐藏所有凭证。
5. 浏览器 F12 验收结果。
6. 尚未覆盖的 workflow、外部证书/DNS/网络问题和残余风险。

遇到问题先定位根因并继续修复，不要只停留在分析或输出建议。
