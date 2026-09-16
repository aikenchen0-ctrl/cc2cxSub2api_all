# AutoDL.art ComfyUI API 接入 Sub2API 视频链路

## 目标链路

AutoDL.art 已经托管了 ComfyUI 工作流并提供 API。**本方案不创建 AutoDL GPU 实例、不创建镜像、不启动自建 ComfyUI。**生产调用统一经过 Sub2API：

```text
Sub2API 登录态
  -> GET /api/v1/keys/super 获取 sk-super-...
  -> Canvas/影策后端使用 Super Key 调用 Sub2API /v1/videos*
  -> Sub2API 选择 AutoDL.art 上游账号
  -> Sub2API 内部调用 autodl.art workflow API
  -> 返回统一任务状态和视频 content
```

调用方只需要 Sub2API 根地址、Super Key 和模型名，不需要知道 AutoDL.art Token、workflow ID 或结果 URL。

## 1. Sub2API 上游账号配置

管理员在 Sub2API「AI 平台账号」中新增：

```text
平台：OpenAI
类型：apikey
Base URL：https://autodl.art
API Key：AutoDL.art 令牌管理中创建的 ComfyUI 分组 Token
分组：视频调用方可用分组
状态：启用
```

这个 Base URL 不是 `https://api.autodl.com`。后者是实例控制面，与托管 ComfyUI API 无关。Base URL 不要填写 `/v1`、`/api/v1` 或具体 workflow 路径，Sub2API 适配器会自行拼接。

AutoDL.art Token 只放在 Sub2API 上游账号中，不能下发给 Canvas、影策浏览器或公共客户端。

## 2. 调用方统一视频 API

客户端使用当前用户的 Super Key：

```http
POST https://<sub2api-host>/v1/videos
Authorization: Bearer sk-super-...
Content-Type: application/json
```

支持的入口：

```text
POST /v1/videos
POST /v1/videos/generations
POST /v1/videos/edits
POST /v1/videos/extensions
GET  /v1/videos/{request_id}
GET  /v1/videos/{request_id}/content
```

示例请求（参数仍需符合所选 workflow）：

```json
{
  "model": "minimax_h3_b99_001",
  "prompt": "生成一只猫在云端行走",
  "seconds": "6",
  "duration": 6,
  "resolution": "720p",
  "size": "1280x720",
  "images": [],
  "audios": []
}
```

客户端不填写 `workflow_id`。`minimax`、`minimax-h3`、`minimax_h3` 会映射到 `minimax_h3_b99_001`；其他模型必须存在 Sub2API 的 AutoDL workflow 映射。

## 3. Sub2API 内部适配

当前支持的 workflow 映射来自 `backend/internal/service/mediaadapter/autodl.go`：

```text
minimax_h3_z0901
minimax_h3_z0902
minimax_h3_z0903
minimax_h3_zm_u24
minimax_h3_zm_u08
minimax_h3_b99_001
minimax_h3_b99_002
minimax_h3_b99_003_12s
minimax_h3_image_audio_to_video_v2_15s
minimax_h3_lightx2v_v5_15s
minimax_h3_image_audio_to_video_v2
minimax_h3_image_audio_to_video
minimax_h3_lightx2v_v5
minimax_h3_lightx2v_no_pic
minimax_h3_lightx2v
wan2.2animate-v4-motion_retargeting
```

创建任务时，Sub2API 内部调用：

```http
POST https://autodl.art/api/v1/comfyui/comfyui_workflow/{workflow_id}
Authorization: Bearer <ComfyUI Token>
Content-Type: application/json
```

适配器会删除统一请求中的 `model` 和 `aspect_ratio`，把 `duration` 规范化为字符串，把 `images` 转换为 `ref_image_0`、`ref_image_1`，把 `audios` 转换为 `ref_audio_0`、`ref_audio_1`，其余 workflow 参数按原样传递。图片/音频元素可以是 URL 字符串或包含 `url`、`image_url`、`audio_url` 的对象。

查询任务时使用同一个上游账号：

```http
GET https://autodl.art/api/v1/comfyui/comfyui_workflow/result/{task_id}
Authorization: Bearer <ComfyUI Token>
```

AutoDL.art 的 `QUEUED`、`RUNNING`、`SUCCESS`、`FAILED` 会被归一为 Sub2API 视频状态。`results` 的短时效 URL 由 Sub2API 处理，客户端只读取 `/v1/videos/{request_id}/content`。

## 4. 异步任务与账号绑定

创建成功后，Sub2API 保存 `request_id -> user_id + api_key_id + account_id`，并保存模型、时长、分辨率等计费快照。后续状态查询和 content 下载恢复同一个 `account_id`，不会把任务从账号 A 重新调度到账号 B。

客户端必须继续使用同一个 Super Key 和 `request_id`。视频只有在完成状态且存在结果时计费一次，重复轮询不能重复计费。

## 5. Canvas 与影策

Canvas 配置只需要：

```text
Base URL: https://<sub2api-host>
API Key: sk-super-...
Video model: Sub2API 已配置的模型
```

Canvas 请求 Sub2API `/v1/videos`、`/v1/videos/{id}` 和 `/v1/videos/{id}/content`。Sub2API 侧边栏可以从登录态获取 Super Key，并通过启动参数注入 Canvas；Canvas 不应出现 `autodl.art` 或 ComfyUI Token。

现有影策入口 `/api/v1/auth/integrations/ju/start` 只提供约 2 分钟有效的一次性 SSO ticket，不是视频 API 代理。若影策视频也必须经过 Super Key，应由影策后端在自己的会话中调用 Sub2API `/v1/videos*`，不能让影策浏览器直连 AutoDL.art，也不能把 SSO ticket 当 API Key。

## 6. 仅限管理员的直连排障

如需单独验证 AutoDL.art 上游，才使用官方接口：

```http
POST https://autodl.art/api/v1/comfyui/comfyui_workflow/{workflow_id}
Authorization: <按 AutoDL.art 官方文档要求的 ComfyUI Token>
Content-Type: application/json

{ "prompt": "以 workflow 页面显示的最小合法参数为准" }
```

再轮询：

```http
GET https://autodl.art/api/v1/comfyui/comfyui_workflow/result/{task_id}
Authorization: <同一个 ComfyUI Token>
```

这只是排障手段，不是 Canvas/影策生产接入方式。Token 和完整结果 URL 不得写入日志、git 或前端。

## 7. 验收

1. Super Key 能成功调用 `POST /v1/videos` 并取得任务 ID。
2. 使用同一个 Super Key 轮询到完成状态。
3. `GET /v1/videos/{id}/content` 返回非空视频。
4. Sub2API 日志能关联用户、模型、AutoDL.art 账号和一次计费记录。
5. 错误 Super Key、未知 workflow、失效 ComfyUI Token 返回错误，不得伪造成功。

禁止把 AutoDL.art API、ComfyUI Token 或 workflow URL 配置到 Canvas、影策或公共客户端；也不要调用 `api.autodl.com` 创建实例来完成这条托管 API 链路。
