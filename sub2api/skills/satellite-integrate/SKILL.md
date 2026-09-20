---
name: satellite-integrate
description: 把新的 AI 项目接到 Sub2API 公共层（SSO 菜单、隐藏 SuperKey、LINK / _link 环境变量）。用户说对接、接入、左侧菜单、卫星应用、新项目进门户时使用。
---

# 卫星应用接入

先读仓库文档，再改代码：

- 工作区根目录 `docs/卫星应用接入公共层.md`（完整清单）
- `docs/卫星公开模型.md`（文本 / 图片 / 视频公开名，默认 gpt-5.5 / gpt-image-2 / grok-imagine-video-1.5）
- `本地链接.txt`（LINK 与 `{slug}_link`）
- 代码 `sub2api/backend/pkg/satellite/`

## 必做

1. 单体先改多用户，数据按用户隔离。
2. 实现 SSO 回调，用 `sub` 映射本地用户。不要要求票据里的 `rk`。
3. 调模型只走 Sub2API `/v1`，三个头：`Authorization: Bearer <SUB2API_APP_CREDENTIAL>`、`X-Sub2API-On-Behalf-Of`、`X-Sub2API-Satellite`。默认公开名：文本 `gpt-5.5`、图片 `gpt-image-2`、视频 `grok-imagine-video-1.5`。完整目录见 `docs/卫星公开模型.md`。
4. 禁止把 SuperKey 当 API Key 或放进浏览器。
5. 本地 / 云端只切换 `LINK` 和 `{slug}_link`。
6. Sub2API：`pkg/satellite/apps.go` 加一行 + 菜单一项。不要新写 SSO Handler。
