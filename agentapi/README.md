# AgentAPI 代理站

AgentAPI 是 Sub2API 的共享多租户代理站。所有代理站共用一套站点、代码、后端和部署；Sub2API 用户点击“一键开分站”后成为一个租户的站长，不同站长的数据以可信 `agent_id` 严格隔离。

> AI 或开发者修改本目录前，必须先读 [00-AI开发规范与目标架构.md](00-AI开发规范与目标架构.md)、`../docs/卫星应用接入公共层.md` 和 `../docs/卫星公开模型.md`。不得根据历史代码继续扩展“每站一个容器/数据库/端口”的旧方案。

## 产品边界

Sub2API 负责真实用户、余额、充值、订单、订阅、模型目录、上游调度、计费和用量。AgentAPI 只保留共享代理站所需的核心能力：登录/注册、用户仪表盘、用户自己的 Agent API Key、用量记录、充值与订单、推广返利、个人资料、站长基础仪表盘、本站公告、用量同步、套餐/订单管理以及品牌和基础系统设置。模型兼容接口 `/v1/*` 仍是核心服务能力，但站内不再提供模型浏览或模型操作工作台。

AgentAPI 明确不提供：模型广场、模型工作台、模型权限/模型策略管理、批量图片、代理站开通管理、运营监控、优惠码管理、订阅管理、渠道管理、代理站计费、内容页面管理、备份管理、分组管理、账号管理、代理管理、兑换码管理、插件管理、安全审计（风险控制与提示词审计）和上游核验。禁用模块已从菜单、路由页面、前端 API 客户端、非核心后端 Handler 和新库表结构删除；旧库启动迁移会删除对应退役表，遗留管理路径固定返回 `404`，站长不能代其他用户管理 Agent Key。模型网关只接受仓库定义的卫星公开模型目录；AgentAPI 不提供租户内容页或法律协议 API。

## 目标架构

```text
Sub2API 已登录用户
  → 点击“一键开分站”
  → 主站幂等创建或返回 agent 租户
  → 既有卫星 SSO 启动接口
  → 共享 AgentAPI 建立 3 天 HttpOnly Session
  → 每个请求解析可信 TenantContext
  → 所有数据访问按 agent_id 隔离
```

- 开站是创建租户记录，不是部署新容器。
- 站长是当前租户管理员，不是 Sub2API 管理员。
- 浏览器不接触 SuperKey、主站 JWT、应用凭据或管理员 Key。
- 模型请求按当前真实用户的 `main_user_id` 调用 Sub2API，不由站长代付。
- 新签发的 `sk-*` 只在 AgentAPI 认证，数据库只保存哈希，完整 Key 仅创建时返回一次；历史 `sk-agent-*` 继续兼容，`sk-super-*` 永远不能作为本站 Key。
- 自定义域名和品牌是租户配置，不代表独立运行实例。

## 当前实现状态

截至 2026-09-30，后端已经具备第一批共享多租户核心：同一数据库可保存多个站点，Session 和 Agent Key 固定所属租户，模型转发按凭证租户结算，公开品牌及登录入口可按数据库 Host 映射选择租户，并对跨租户 Session、伪造 query/header 和重复域名建立拒绝与测试。视频、图片任务和结算也已验证可在不同租户复用相同 `task_id` / `request_id`，读取及更新不会串站。主站“一键开分站”代码已改为按已登录用户幂等创建或返回逻辑租户及 owner 成员关系，再通过既有卫星 SSO 票据传递签名的 `agent_id + role + agent_name`；AgentAPI 严格校验这些字段、创建缺失租户并建立 3 天会话，已有租户的 owner 不允许被后续票据或重启改写。上述非核心模块的前端、客户端和专属服务端入口均已删除并建立 `404` 回归测试；AgentAPI 不再注册租户内容页或法律协议 API。

当前共享运行时已经在本地完成一套真实部署与联调：Sub2API 的 `246/247` 迁移已执行并登记，旧 provisioning 表已删除；主站“一键开分站”到 AgentAPI SSO、同一用户重复开站幂等、两个站长获得不同 `agent_id`、共享容器数不变、个人 Key 和列表跨租户隔离、未知 Host 返回 `421` 均已实测通过。迁移另在临时 PostgreSQL 16 中覆盖历史 owner 收敛、已删除用户跳过、旧表/函数删除和失败回滚。AgentAPI 内部及主站的逐站 provision/runtime-control 代码、凭据和管理界面均已退休。

尚未完成的是正式生产环境验收，而不是本地共享主链路：当前 AgentAPI Compose 仍是单副本 SQLite，生产还需要并发数据库、多副本 Session/队列、正式域名与 HTTPS Cookie、付费文本/流式/图片/视频计费核对、灰度数据核数、备份恢复和灾备演练。因此可以表述为“共享代理站核心代码、本地部署和双租户隔离已验证”，不能表述为“已经完成生产上线”。准确证据见 [26-部署验收记录.md](26-部署验收记录.md)，生产门槛见 [00-AI开发规范与目标架构.md](00-AI开发规范与目标架构.md) 第 14 节。

## 目录

- `backend/`：Go 服务、数据层、Sub2API 客户端、模型代理与测试。
- `frontend/`：Vue 3 用户界面、站长控制台与组件测试。
- `Dockerfile`、`docker-compose.yml`：当前共享代码的单副本 SQLite 开发/迁移部署，不是最终多副本生产拓扑。
- 逐站 `agentapi-provision*` 命令和 worker 已删除；新租户开通只在共享运行时中创建逻辑租户和 owner 关系。
- `00-AI开发规范与目标架构.md`：最高优先级规范。
- `25-主站事实与代理站管理边界.md`：主站权威数据和本站权限边界。

## 本地开发

后端程序不会自动读取 `.env`。推荐使用 Compose，并确保 `backend/.env` 中的 `SUB2API_SSO_SECRET` 与主站一致：

```bash
cd agentapi
docker compose --env-file backend/.env up -d --build
```

`backend/.env` 是容器内服务端配置，并同时作为 Compose 插值来源；其中的 `SUB2API_SSO_SECRET` 必须与主站一致。不要把任何密钥放入 `frontend/.env`，也不要为不同站长复制 Compose 项目或修改端口：所有站长共用这一个服务，由签名 SSO 中的 `agent_id` 和服务端 Session 隔离。

主站也在本机 Docker 时，可附加主站网络：

```bash
docker compose --env-file backend/.env -f docker-compose.yml -f docker-compose.local.yml config --quiet
docker compose --env-file backend/.env -f docker-compose.yml -f docker-compose.local.yml up -d --build
```

不要公开不带 `--quiet` 的 Compose 配置输出，不要使用 `down -v` 清空数据。该部署只用于当前实现的开发和迁移验证，不得复制为每个新租户的生产部署。

前端开发与构建：

```bash
cd agentapi/frontend
pnpm install
pnpm dev
pnpm build
```

生产构建输出到 `backend/web/`，由 Go 服务同源托管。

## 当前兼容配置

以下是共享运行时及迁移兼容配置，迁移期间必须区分生产必需项与显式旧站兼容项：

- `MAIN_API_URL`：Sub2API 主站 API Origin。
- `LINK`：主站公开 Origin，用于充值跳转。
- `AGENTAPI_SHARED_HOSTS`：共享代理站公共入口的精确 Host 白名单，逗号分隔；不得填写协议、路径或通配符。数据库中已认领的租户自定义域名会单独放行。共享模式缺少该配置时服务拒绝启动。
- `MAIN_MODEL_URL` / `SUB2API_RELAY_BASE_URL`：模型网关地址。
- `SUB2API_APP_CREDENTIAL`：服务端卫星应用凭据。
- `SUB2API_SSO_SECRET`：与 Sub2API 一致的 SSO Secret。
- `SESSION_SECRET`：AgentAPI Session 密钥。
- 共享运行时会在进程启动时强制校验上述三项：应用凭据不能为空，SSO Secret 与 Session Secret 均不得短于 32 个字符；缺失时直接退出，不能以“容器健康但核心业务全部 503”的半配置状态上线。
- 本地 Compose 默认发布到 `18081`（主站为 `18080`）。若在 Shell 中覆盖 `AGENTAPI_PORT`，后续重建必须继续使用同一值；不要让它隐式回退到主站常用的 `8080`。
- `SUB2API_ADMIN_API_KEY` 或 `_FILE`：当前注册普通主站用户的可信服务端凭据，不得用于模型调用或进入浏览器。
- 共享生产默认不设置 `AGENT_ID`、`AGENT_DOMAIN`、`AGENT_OWNER_MAIN_USER_ID`，进程启动不会凭空创建租户；租户由主站签名 SSO 首次进入时创建，并由数据库中的 TenantContext、Session、Agent Key 和 Host 映射解析。
- 共享入口必须设置 `AGENTAPI_SHARED_HOSTS`；反向代理必须保留真实公共 `Host`，不能依赖浏览器可伪造的 `X-Forwarded-Host`。未知 Host 在进入认证、公开设置或模型接口前返回 `421`。
- 上述三个变量只保留给旧单租户数据迁移。只有显式设置 `AGENT_ID` 时，旧的不含 `agent_id` Cookie 才会被绑定到该兼容租户；共享生产会直接拒绝这种旧 Cookie，避免隐式串站。
- 部署预检使用独立的 `AGENTAPI_PREFLIGHT_MAIN_USER_ID` 作为只读探测身份，不再把某个代理站站长当成整个共享进程的 owner。
- `AGENT_BILLING_MODE=user_upstream`：用户主站直结模式。
- `COOKIE_SECURE=true`：生产 HTTPS Cookie。

## 充值与计费

AgentAPI 不创建可消费本地余额。充值入口只使用公开 `LINK` 跳转 Sub2API；未配置公开地址时禁用，不得暴露 Docker 内部地址。充值后重新读取用户的主站真实余额。

主站负责实际计费和用量入库。AgentAPI 只保存明确标记来源和完整性的只读副本；未取得主站明细时显示待确认或不可用，不得使用余额差值、预留值或标准价估算冒充真实费用。

## 验证

部署前只读预检：

```bash
cd agentapi/backend
go run . --preflight
```

部署后只读验收（正式环境默认强制 HTTPS）：

```powershell
.\scripts\VerifyDeployment.ps1 `
  -AgentBaseUrl 'https://agent.example.com' `
  -ExpectedHost 'agent.example.com'
```

该脚本检查首页、`/healthz`、`/readyz`、未知 Host=`421`、21 条永久删除路径=`404`，并扫描入口 HTML 与 JavaScript 产物，确认没有服务端密钥赋值或 SuperKey 值。仅在本地 HTTP 联调时可显式增加 `-AllowHttp`；正式验收不得使用该参数。可选的 `-SessionCookieFile` 与 `-AgentKeyFile` 只从本地文件读取现有会话 Cookie 和个人 `sk-*`，不会在输出中打印文件内容，也不会创建用户、Key、订单、模型请求或计费记录。`-Json` 可供流水线读取结构化结果。

该脚本是无副作用的边界核验，不替代正式浏览器登录、两个租户并发隔离、真实付费文本/图片/视频、支付回调、多副本或恢复演练。

运行中 SQLite 的一致性备份（无管理页面，不会暂停服务）：

```bash
docker exec agentapi-agentapi-1 /app/agentapi --backup /app/data/backups/agentapi-20260930.db
docker cp agentapi-agentapi-1:/app/data/backups/agentapi-20260930.db ../.local/agentapi-backups/agentapi-20260930.db
```

`--backup` 使用 SQLite `VACUUM INTO`，会纳入已提交 WAL 并在成功前执行 `integrity_check`；目标文件必须不存在，命令不会覆盖旧备份。复制到数据卷之外并加密保存后，才算完成灾备备份。该命令是部署运维能力，不恢复已经删除的“备份管理”菜单或站长权限。

自动化测试：

```bash
cd agentapi/backend
go test ./...

cd ../frontend
pnpm typecheck
pnpm test:run
pnpm build
```

测试与本地联调已经证明共享开站主链路、双租户隔离和迁移行为；它们仍不能替代正式域名浏览器流程、真实付费模型请求、多副本与灾备等生产验收。最终验收按 [00-AI开发规范与目标架构.md](00-AI开发规范与目标架构.md) 第 14 节执行。

## 文档导航

- [00-AI开发规范与目标架构.md](00-AI开发规范与目标架构.md)：必读规范和目标架构。
- [代理站规划.md](代理站规划.md)：共享多租户迁移计划。
- [25-主站事实与代理站管理边界.md](25-主站事实与代理站管理边界.md)：主站事实和权限边界。
- [24-成本控制台参考与用量分析.md](24-成本控制台参考与用量分析.md)：用量统计专题。
- [26-部署验收记录.md](26-部署验收记录.md)：当前本地共享部署验收与历史联调证据。
- [27-主站前端迁移计划.md](27-主站前端迁移计划.md)：历史前端迁移流水账，不是当前产品清单。
