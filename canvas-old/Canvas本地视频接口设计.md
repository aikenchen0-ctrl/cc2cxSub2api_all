# Canvas 本地视频接口设计

## 目标

Canvas 在本地开发时通过 `http://localhost:18080` 调用 Sub2API，线上访问时通过 `https://api.cc2.cx` 调用同一网关。浏览器根据当前页面主机自动选择 Base URL，URL 查询参数仍可临时覆盖配置。

## 请求链路

1. Canvas 从当前模型渠道解析 `baseUrl`、`apiKey` 和模型名。
2. 创建视频：`POST {baseUrl}/v1/videos`。
3. 请求头：`Authorization: Bearer {apiKey}`，JSON 请求必须包含 `Content-Type: application/json`。
4. 请求体按请求类型选择字段。JSON 文生视频示例：

```json
{
  "model": "seedance2.0-900-720p",
  "prompt": "...",
  "seconds": "10",
  "duration": 10,
  "size": "1280x720",
  "resolution": "720p",
  "ratio": "16:9",
  "aspect_ratio": "16:9",
  "input_reference": ["data:image/png;base64,..."],
  "input_audio": ["data:audio/mpeg;base64,..."],
  "reference_mode": "asset"
}
```

字段说明：

- `model` 或 `model_id`：使用 `GET /v1/models` 返回的公开模型 ID，不要传内部 `upstream_model`。
- `seconds` 或 `duration`：视频时长，可传数字或带 `s` 的字符串，必须在模型定价和能力范围内。
- `size`：如 `1280x720`，用于推导比例和分辨率；显式 `ratio`/`aspect_ratio` 与 `resolution` 优先。
- `input_reference`、`reference_images` 或 `image`：参考图片；JSON 传 base64/data URI，multipart 传文件，不要传公网 URL。
- `input_audio`、`reference_audios` 或 `audio`：参考音频，支持 JSON base64/data URI 或 multipart 文件。
- `reference_mode`：`frame` 表示首尾帧（最多 2 张），`asset` 表示普通参考图；仅在模型支持时发送。

multipart 图生视频使用 `input_reference` 重复上传文件字段；同样的字段可用于 `input_audio`。Sub2API 也兼容 `/v1/videos/generations`，但 Canvas 默认使用 `/v1/videos`。

## 任务状态

- 查询：`GET {baseUrl}/v1/videos/{taskId}`
- 完成但未返回 URL：`GET {baseUrl}/v1/videos/{taskId}/content`
- 上游状态通常为 `queued`、`in_progress`、`completed`、`failed`；Canvas 映射为 `pending`、`completed`、`failed`。
- 创建响应至少读取 `id`（或嵌套任务中的 `request_id`）；轮询使用实际任务 ID。
- 结果 URL 兼容读取 `video.url`、`video_url`、`result_url`、`content.video_url`、`data[0].url`。
- 没有结果 URL 时，`GET /v1/videos/{taskId}/content` 返回 `video/mp4` 原始二进制，不是 base64 或 JSON。

## 错误契约

- `401/403`：鉴权失败，检查 Canvas 渠道 API Key 和 Sub2API 下游权限。
- `404`：模型或接口不存在，检查模型映射与 Base URL。
- `422`：请求字段不符合上游模型要求，保留服务端错误正文供 UI 展示。
- `503`：网关没有可用账号或账号被利润阈值过滤；Canvas 不伪造成功，直接显示服务繁忙，并在 Sub2API 管理端调整分组利润控制/账号权限。
- `400`：参数缺失、模型不支持或未定价；保留错误正文展示具体字段。
- `402`：积分不足；不要继续轮询已拒绝的任务。
- `409`：视频尚未完成，content 尚未就绪；继续轮询任务状态。
- `429`：账号并发已满；按服务端提示重试并避免高频创建。
- 网络错误：显示请求失败，并保留可重试操作。

## 验收标准

- 本地 Canvas 页面请求目标为 `http://localhost:18080/v1/videos`。
- 线上 Canvas 页面请求目标为 `https://api.cc2.cx/v1/videos`，不再使用访问者自己的 localhost。
- 创建、轮询、content 下载均携带同一 API Key。
- 创建前先调用 `GET /v1/models` 获取公开模型名、时长、分辨率、比例、参考图和音频能力，Canvas 不猜测模型参数。
- 时长、分辨率、比例和参考素材数量必须落在模型声明的能力及定价范围内。
- 视频结果保存到 Canvas 本地资产后可预览、下载和再次引用。
