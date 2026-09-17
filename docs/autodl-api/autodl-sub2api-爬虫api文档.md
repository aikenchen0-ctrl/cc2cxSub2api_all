# AutoDL 视频上游接入 Sub2API 对接文档

## 结论先行

本项目的主路径是 AutoDL.art 托管的 ComfyUI API：**不创建 AutoDL GPU 实例、不创建镜像、不启动自建 ComfyUI**。只配置一个 AutoDL.art 的 `ComfyUI` Token 作为 Sub2API 上游凭据。

```text
Sub2API 登录态
  -> GET /api/v1/keys/super 获取 sk-super-...
  -> Canvas/影策后端使用 Super Key 调用 Sub2API /v1/videos*
  -> Sub2API 选择 AutoDL.art 上游账号
  -> Sub2API 内部调用 autodl.art workflow API
  -> 返回统一视频任务、状态和 content
```

原文中“创建实例、创建镜像、vLLM/SGLang、6006/6008”描述的是另一条自建 GPU 上游路线，不是 AutoDL.art 托管 API。除非明确选择自建路线，否则不要执行那些步骤。

## 1. 凭据边界

| 凭据 | 用途 | 应保存在哪里 |
|---|---|---|
| Sub2API JWT/Cookie | 登录并获取 Super Key | Sub2API 浏览器会话 |
| `sk-super-...` | 调用 Sub2API `/v1/videos*` | Canvas 或影策后端 |
| AutoDL.art ComfyUI Token | 调用托管 workflow | Sub2API 上游账号 |
| AutoDL 控制面 Token | 创建/开关机 `autodl.com` 实例 | 仅运维环境，自建路线才需要 |

Super Key 的获取接口：

```http
GET https://<sub2api-host>/api/v1/keys/super
Authorization: Bearer <Sub2API 登录 JWT>
```

Super Key 名称为 `Sub2API Super Key`，前缀为 `sk-super-`。不要把 AutoDL.art Token 或 AutoDL 控制面 Token 发给 Canvas、影策浏览器或公共客户端。

## 2. Sub2API 配置 AutoDL.art 上游

管理员在「AI 平台账号」中新增：

```text
平台：OpenAI
账号类型：apikey
Base URL：https://autodl.art
API Key：AutoDL.art 令牌管理中创建的 ComfyUI 分组 Token
分组：视频调用方可用分组
状态：启用
```

这里不能填写 `https://api.autodl.com`，后者是实例控制面；也不要填写 `/v1`、`/api/v1` 或具体 workflow 路径，适配器会自行拼接。

Sub2API 当前的 AutoDL workflow 映射包括：

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

`minimax`、`minimax-h3`、`minimax_h3` 会映射到 `minimax_h3_b99_001`。未知模型应返回不支持错误，不得随机选择文本账号。

## 3. 客户端统一视频接口

Canvas、影策后端或其他客户端只调用 Sub2API：

```text
POST /v1/videos
POST /v1/videos/generations
POST /v1/videos/edits
POST /v1/videos/extensions
GET  /v1/videos/{request_id}
GET  /v1/videos/{request_id}/content
```

示例：

```bash
curl -sS https://<sub2api-host>/v1/videos \
  -H "Authorization: Bearer sk-super-..." \
  -H "Content-Type: application/json" \
  -d '{
    "model":"minimax_h3_b99_001",
    "prompt":"一只猫在云端行走",
    "seconds":"6",
    "duration":6,
    "resolution":"720p",
    "size":"1280x720"
  }'
```

取得返回的 `id`、`task_id` 或 `request_id` 后，使用同一个 Super Key 轮询和下载：

```bash
curl -sS https://<sub2api-host>/v1/videos/<request_id> \
  -H "Authorization: Bearer sk-super-..."

curl -L -o result.mp4 https://<sub2api-host>/v1/videos/<request_id>/content \
  -H "Authorization: Bearer sk-super-..."
```

## 4. 适配器内部行为

创建时，Sub2API 根据模型调用：

```http
POST https://autodl.art/api/v1/comfyui/comfyui_workflow/{workflow_id}
Authorization: Bearer <ComfyUI Token>
Content-Type: application/json
```

适配器删除统一请求中的 `model`、`aspect_ratio`，把 `duration` 规范化为字符串，把 `images` 转换为 `ref_image_0`、`ref_image_1`，把 `audios` 转换为 `ref_audio_0`、`ref_audio_1`，其余 workflow 参数按实际工作流传递。

查询时调用：

```http
GET https://autodl.art/api/v1/comfyui/comfyui_workflow/result/{task_id}
Authorization: Bearer <同一个 ComfyUI Token>
```

`QUEUED`、`RUNNING`、`SUCCESS`、`FAILED` 等状态会归一为 Sub2API 视频状态。Sub2API 保存 `request_id -> account_id` 绑定，轮询不会切换上游账号；AutoDL.art 的短时效 `results` URL 由 Sub2API 处理，客户端只读取 `/content`。

## 5. Canvas 与影策

Canvas 只配置：

```text
Base URL: https://<sub2api-host>
API Key: sk-super-...
Video model: Sub2API 已配置的模型名
```

Sub2API 侧边栏会从登录态获取 Super Key，通过启动参数注入 Canvas。Canvas 不应出现 `autodl.art`、workflow ID 或 ComfyUI Token。

影策现有 `/api/v1/auth/integrations/ju/start` 只提供一次性 SSO ticket，不是视频 API 代理。若影策视频必须走 Super Key，应由影策后端调用 Sub2API `/v1/videos*`；不能让影策浏览器直连 AutoDL.art，也不能把 SSO ticket 当视频 API Key。

## 6. 什么时候才需要创建实例/镜像

仅当选择“自己在 `autodl.com` GPU 实例中部署一个 OpenAI-compatible 视频服务”时，才需要实例、镜像、GPU、端口映射和公网数据面。该服务必须真正支持 `/v1/videos`、状态和 content；只有 `/v1/chat/completions` 的 vLLM/SGLang 文本服务不能作为视频上游。

自建路线与本文主路径互斥：

```text
自建路线：autodl.com 实例 -> 自建视频服务 -> Sub2API
托管路线：autodl.art workflow API -> Sub2API
```

## 7. 验收和禁止事项

验收必须包含：Super Key 创建任务成功、同 Key 轮询完成、`/content` 返回非空视频、日志能关联用户/模型/上游账号/计费记录；错误 Super Key、未知 workflow、失效上游 Token 必须明确失败。

- 不要为调用 autodl.art workflow 创建实例或镜像。
- 不要把 `https://api.autodl.com` 填入 Sub2API AI 平台账号。
- 不要把 AutoDL.art Token、workflow URL 或结果 URL 配置到 Canvas/影策。
- 不要把 Base URL 写成 `/v1` 后再重复拼接，避免 `/v1/v1/videos`。
