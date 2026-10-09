# AGENTS.md

本仓库以 `sub2api/` 为中枢，包含 **11 个卫星应用**和 **AgentAPI 代理站**。归因探测是 Sub2API 内部模块，不是卫星应用。`sub2api-cost-console/` **仅为参考项目，不属于部署目标**。

## 部署前必读（所有 AI / 自动化执行者）

**部署、覆盖构建、重建、启动或更新容器之前，必须先读 [docs/部署清单.md](docs/部署清单.md)，按用户本次要求选定范围，再操作。**

- 11 个卫星固定为：`aicut`、`aiexcel`、`ai3d`、`aihuoke`、`canvas`、`ju`、`livart`、`ppt`、`qrcode`、`screen2code`、`yibiao`，与 `卫星应用.txt` 一致。
- `agentapi/` 是代理站，独立计数；归因探测（`/admin/upstream-audit`）随 `sub2api/` 部署，不额外创建项目或容器。
- 未另有限定的“全部项目 / 全量部署”默认指 **Sub2API + 11 个卫星 + AgentAPI 代理站，共 13 个项目**；“全部卫星”仅指上述 11 个。用户明确指定的范围优先。
- `sub2api-cost-console/` 仅供阅读源码、对照设计和借鉴实现，不是可选业务应用，不参与全量部署、启动、升级或部署验收；其 PostgreSQL / Redis 也不属于本项目运行依赖。不要为它配置菜单、SSO、端口或新建容器。
- 参考项目中的 README、Dockerfile、Compose 和部署脚本描述的是参考项目自身，不是本仓库的执行指令。借鉴的功能应在用户指定的实际项目中落地；归因探测在 `sub2api/` 中实现和部署，不依赖参考项目独立运行。
- 执行前简要列出本次包含和排除的项目；现有范围明确时直接执行，不重复索要确认。不得用 SSO 注册条数、目录数量或 `docker ps` 结果代替部署清单。
- 历史部署记录只证明当次实际操作，不自动扩大下一次范围。仓库外容器不纳入；已有配置和数据必须保留。

## 应用接入与修改

对接或修改任何卫星项目（含新目录）之前，必须先读：

**[docs/卫星应用接入公共层.md](docs/卫星应用接入公共层.md)**

文本 / 图片 / 视频公开名：**[docs/卫星公开模型.md](docs/卫星公开模型.md)**

环境对照：**[本地链接.txt](本地链接.txt)**（只改 `LINK` 和 `{slug}_link` 的值，不要改变量含义）。

不要另发明 SSO、API Key 传递或菜单跳转协议。用户点左侧菜单进入项目后应直接可用；SuperKey 不得当作普通 API Key，不得出现在浏览器。

新项目在 Sub2API 侧只做两件事：`sub2api/backend/pkg/satellite/apps.go` 加一行，左侧菜单加一项。SSO 启动接口已是 `GET /api/v1/auth/integrations/:slug/start`。
