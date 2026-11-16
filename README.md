# Sub2API 工作区

本目录以 `sub2api/` 为主站，挂 **11 个卫星应用** 和 **1 个 AgentAPI 代理站**。业务全量是这 13 个项目，不是目录数量。

部署范围以 [docs/部署清单.md](docs/部署清单.md) 和 [卫星应用.txt](卫星应用.txt) 为准。`项目.txt` 里的旧路径不再作为项目名单。

## 口径

| 分类 | 数量 | 目录 | 是否部署 |
| --- | ---: | --- | --- |
| 主站 | 1 | `sub2api/` | 是 |
| 卫星应用 | 11 | 见下表 | 是 |
| 代理站 | 1 | `agentapi/` | 是，单独计数，不是第 12 个卫星 |
| 主站内部模块 | 0 个额外项目 | 归因探测在 `sub2api/` 内 | 随主站部署，不新建容器 |
| 参考项目 | 不计数 | `sub2api-cost-console/` | 否，只读对照 |
| 文档、脚本、补丁 | 不计数 | `docs/`、`操作文档/`、`scripts/`、`patches/`、`audit-persistence/` | 否 |

菜单项和目录不是一一对应：

- 「AI商品图」打开的是 `livart/`，入口参数为商品海报，不是独立项目。
- 「上游审计」是主站页面 `/admin/upstream-audit`，不是卫星。
- `sub2api/backend/pkg/satellite/apps.go` 有 12 个 SSO 注册项（11 个卫星 + AgentAPI）。这是接入表，不改变上面的业务分类。

## 主站

| 目录 | 菜单 / 入口 | 做什么 | 本地入口 |
| --- | --- | --- | --- |
| `sub2api/` | 主站 | Go + Vue 的 AI API 网关。负责登录、余额、充值、模型目录、上游调度、计费和左侧菜单。 | `http://localhost:18080` |

Compose 项目名是 `deploy`，应用容器 `sub2api`。依赖容器是 `sub2api-postgres`、`sub2api-redis`。`canvas-app` 也在这个 Compose 项目里，局部部署必须指定服务，不能只按项目名把主站和 Canvas 一起重建。

主站文档：[sub2api/README_CN.md](sub2api/README_CN.md)、[sub2api/DEV_GUIDE.md](sub2api/DEV_GUIDE.md)。

## 11 个卫星

用户从主站左侧菜单进入后应直接可用。模型调用走主站，SuperKey 不进浏览器。

| 目录 | 左侧菜单 | 产品 | 本地端口 | 应用容器 |
| --- | --- | --- | --- | --- |
| `canvas/` | AI生图生视频 | 无限画布。图片、视频、音频和素材编排。 | 3522 | `canvas-app` |
| `ju/` | AI导演台 | 影策。从文字走到分镜、角色和影视镜头资产。 | 3000 | `ju-web-1`、`ju-backend-1` |
| `livart/` | 超级改图 | 基于画布的 AI 图像创作；「AI商品图」也进这里。 | 8080 | `livart-livart-1` |
| `ppt/` | PPT生成 | Presenton。AI 演示文稿生成。 | 8341 | `ppt-production-1` |
| `aicut/` | AI剪辑 | OpenChatCut。Agent 可继续编辑的 AI 视频剪辑。 | 5199 | `aicut` |
| `screen2code/` | AI一键复刻网站 | 截图、设计稿或录屏生成前端代码。 | 5173 | `screen2code-frontend-1`、`screen2code-backend-1` |
| `aiexcel/` | AI表格 | 格知。浏览器内解析表格，AI 只生成操作计划。 | 4173 | `aiexcel` |
| `qrcode/` | 艺术二维码 | 墨莲。可扫的艺术二维码。目录冻结，只构建、部署和只读验收。 | 5221 | `qrcode` |
| `yibiao/` | AI找标投标 | BidMonitor。招投标信息监控和筛选。 | 8081 | `yibiao` |
| `ai3d/` | 图生3D | 图片、草图或 GLB 的 3D 生成和查看。 | 5174 | `ai3d` |
| `aihuoke/` | AI获客 | 营销线索情报。可运行代码在 `aihuoke/ai-hunter-root/`，`refs/` 只读。 | 3001 | `aihuoke-frontend`、`aihuoke-backend` |

11 个卫星对应 14 个应用容器。AI 获客、JU、Screen2Code 各有前后端两个容器，容器数不能当成卫星数。

各卫星依赖：JU 使用 `ju-sso-redis-1`；Livart 使用 `livart-postgres`、`livart-rabbitmq-1`、`livart-minio-1`。其余卫星不另算工作区依赖容器。

操作步骤在 [操作文档](操作文档/)，按菜单名编号，不是另一套应用。

## 代理站

| 目录 | 左侧菜单 | 做什么 | 本地入口 |
| --- | --- | --- | --- |
| `agentapi/` | 一键开分站 | 共享多租户代理站。所有站长共用一套站点、代码和容器，按 `agent_id` 隔离。不按租户新建容器、数据库或端口。 | `http://localhost:18081` |

容器是 `agentapi-agentapi-1`。收费行为以 [agentapi/收费模块.md](agentapi/收费模块.md) 和当前代码为准，部署时不改收费模式。

## 不部署的目录

| 目录 | 处理方式 |
| --- | --- |
| `sub2api-cost-console/` | 仅供对照源码和界面。不配置菜单、SSO、端口，不加入全量部署。归因探测的落地位置是 `sub2api/`，不是这里。 |
| `docs/` | 工作区文档。部署前先读 [docs/部署清单.md](docs/部署清单.md)。 |
| `操作文档/` | 11 个卫星的测试操作手册。 |
| `scripts/` | [scripts/Start-LocalDocker.ps1](scripts/Start-LocalDocker.ps1) 只启动上述 13 个项目的既有容器，不重建镜像，也不启动参考项目。 |
| `patches/` | 调度连续性补丁和事故记录，不是运行服务。 |
| `audit-persistence/` | 审计保留脚本和说明，不是卫星，也不替代主站归因探测。 |
| `.local/` | Git 忽略的本机快照、凭据和回滚材料。不提交，不写入公开文档。 |

仓库外容器不纳入。已有的 `cost-console`、`cost-console-postgres`、`cost-console-redis` 只是历史参考实例，不因此变成部署目标，也不要为了整理名单去停掉它们。

## 本地入口

已有本机部署时，在仓库根目录启动，不删除数据：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/Start-LocalDocker.ps1
```

不要用 `docker compose down -v`。不要把 `sub2api/deploy` 的单独启动当成 13 个项目都已部署。各项目源码级启动方式看各自 README。

地址只改 [本地链接.txt](本地链接.txt) 里的 `LINK` 和 `{slug}_link`，不改变量含义。本地主站模型地址是 `LINK=localhost:18080`。

## 接入约定

改任何卫星或代理站前先读：

- [docs/卫星应用接入公共层.md](docs/卫星应用接入公共层.md)
- [docs/卫星公开模型.md](docs/卫星公开模型.md)
- [AGENTS.md](AGENTS.md)

SSO 启动接口固定为 `GET /api/v1/auth/integrations/:slug/start`。新卫星在主站只加 `apps.go` 一行和左侧菜单一项。卫星本地 Session 保持 3 天。不要另写一套 SSO、API Key 传递或菜单跳转协议。

## 根目录文件

这些文件不是独立项目。

| 文件 | 用途 |
| --- | --- |
| [AGENTS.md](AGENTS.md) | 给自动化执行者的范围和接入规则 |
| [卫星应用.txt](卫星应用.txt) | 11 个卫星目录名单 |
| [本地链接.txt](本地链接.txt) | 本地和线上的 `LINK`、菜单跳转地址 |
| [docker-compose.online.yml](docker-compose.online.yml) | 线上编排，不是本地全量启动入口 |
| [nginx-additional-servers.conf](nginx-additional-servers.conf) | 额外 Nginx 站点片段 |
| [SSO登录设计方案.md](SSO登录设计方案.md) | SSO 方案记录，执行时仍以公共层和代码为准 |
| [部署分站.md](部署分站.md)、[代理部署流程.md](代理部署流程.md) | AgentAPI 分站部署记录和流程 |
| [卫星计费配置.md](卫星计费配置.md) | 卫星计费配置说明 |
| [管理员key接口文档.md](管理员key接口文档.md) | 管理员 Key 接口 |
| [账号切换策略-260926.md](账号切换策略-260926.md) | 上游账号切换策略 |
| [预设大模型.md](预设大模型.md) | 预设模型清单 |
| [AutoDL视频模型文档.txt](AutoDL视频模型文档.txt) | AutoDL 视频工作流原料 |
| [图标设定.md](图标设定.md) | 卫星 favicon 读取主站 `site_logo` |
| [home.html](home.html)、[logo.jpg](logo.jpg) | 页面和品牌素材，不是应用 |
| [工作汇报.md](工作汇报.md)、[审查报告-20261023.md](审查报告-20261023.md)、[提示词.txt](提示词.txt)、[项目.txt](项目.txt) | 历史记录或旧清单，不定义当前部署范围 |

## 许可证

主站遵循 [sub2api/LICENSE](sub2api/LICENSE)。其他目录如有单独许可证，以该目录内的文件为准。不要提交真实密钥、密码、生产数据或 `.local/` 中的配置。
