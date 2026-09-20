# AGENTS.md

本仓库以 `sub2api/` 为中枢，其余目录是独立卫星 AI 应用。

对接或修改任何卫星项目（含新目录）之前，必须先读：

**[docs/卫星应用接入公共层.md](docs/卫星应用接入公共层.md)**

文本 / 图片 / 视频公开名：**[docs/卫星公开模型.md](docs/卫星公开模型.md)**

环境对照：**[本地链接.txt](本地链接.txt)**（只改 `LINK` 和 `{slug}_link` 的值，不要改变量含义）。

不要另发明 SSO、API Key 传递或菜单跳转协议。用户点左侧菜单进入项目后应直接可用；SuperKey 不得当作普通 API Key，不得出现在浏览器。

新项目在 Sub2API 侧只做两件事：`sub2api/backend/pkg/satellite/apps.go` 加一行，左侧菜单加一项。SSO 启动接口已是 `GET /api/v1/auth/integrations/:slug/start`。
