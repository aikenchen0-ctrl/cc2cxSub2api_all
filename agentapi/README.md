# AgentAPI 代理站

AgentAPI 是 Sub2API 的共享多租户代理站。所有代理站共用一套站点、代码、后端和部署；Sub2API 用户点击“一键开分站”后成为一个租户的站长，不同站长的数据以可信 `agent_id` 严格隔离。

> AI 或开发者修改本目录前，必须先读 `../docs/卫星应用接入公共层.md` 和 `../docs/卫星公开模型.md`。收费行为以当前代码和 [收费模块.md](收费模块.md) 为准。不得根据历史代码继续扩展“每站一个容器/数据库/端口”的旧方案。

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

- 共享入口支持两种登录方式：主站 SSO，以及主站邮箱密码。密码登录由 AgentAPI 服务端向主站验证，再调用既有卫星 SSO 启动接口取得签名 ticket，最后建立本站 3 天 HttpOnly 租户 Session；密码、主站 JWT、SuperKey 和运行时凭据都不进入浏览器。
- 开站是创建租户记录，不是部署新容器。
- 站长是当前租户管理员，不是 Sub2API 管理员。
- 浏览器不接触 SuperKey、主站 JWT、应用凭据或管理员 Key。
- 模型请求的收费方式见 [收费模块.md](收费模块.md)。默认按当前用户的主站账户扣费；站长开通本站支付后，主站从站长账户结算实际费用，用户本站余额按“实际费用 × 模型计费倍率”扣减。
- 新签发的 `sk-*` 只在 AgentAPI 认证，数据库只保存哈希，完整 Key 仅创建时返回一次；历史 `sk-agent-*` 继续兼容，`sk-super-*` 永远不能作为本站 Key。
- 自定义域名和品牌是租户配置，不代表独立运行实例。

## 当前实现状态

截至 2026-11-16，后端已经具备共享多租户核心：同一数据库可保存多个站点，Session 和 Agent Key 固定所属租户，模型转发按凭证租户结算，公开品牌及登录入口可按数据库 Host 映射选择租户，并对跨租户 Session、伪造 query/header 和重复域名建立拒绝与测试。视频、图片任务和结算也已验证可在不同租户复用相同 `task_id` / `request_id`，读取及更新不会串站。主站“一键开分站”代码已改为按已登录用户幂等创建或返回逻辑租户及 owner 成员关系，再通过既有卫星 SSO 票据传递签名的 `agent_id + role + agent_name`；AgentAPI 严格校验这些字段、创建缺失租户并建立 3 天会话，已有租户的 owner 不允许被后续票据或重启改写。上述非核心模块的前端、客户端和专属服务端入口均已删除并建立 `404` 回归测试；AgentAPI 不再注册租户内容页或法律协议 API。

当前共享运行时已经在本地完成一套真实部署与联调：Sub2API 的 `246/247` 迁移已执行并登记，旧 provisioning 表已删除；主站“一键开分站”到 AgentAPI SSO、同一用户重复开站幂等、两个站长获得不同 `agent_id`、共享容器数不变、个人 Key 和列表跨租户隔离、未知 Host 返回 `421` 均已实测通过。迁移另在临时 PostgreSQL 16 中覆盖历史 owner 收敛、已删除用户跳过、旧表/函数删除和失败回滚。AgentAPI 内部及主站的逐站 provision/runtime-control 代码、凭据和管理界面均已退休。

2026-10-27 已补齐共享入口邮箱密码登录：服务端先验证主站密码，再以主站 access token 调用 `GET /api/v1/auth/integrations/agentapi/start` 换取签名 ticket，校验 ticket 的用户与登录用户一致后建立租户 Session。主站 SSO 继续保留；登录页按 `registration_enabled` 隐藏注册入口，关闭注册时提示用户先在主站注册。同日 17:27 已将包含该链路和白金主题的 `cc2cx/agentapi:20261027-1727` 发布到 `https://agentapi.cc2.cx`；公网 `/api/v1/auth/login-options` 返回 `password_login_enabled=true`，并使用一次性主站账号完成真实邮箱密码登录 200 验证。

2026-10-28 已支持站长域名直接注册：`AGENTAPI_ENTRY_TENANTS` 将精确 Host 绑定到逻辑租户，未登录访问该域名时解析为该站长站点；注册先在主站创建普通用户，再通过带卫星应用凭据的 `/api/v1/auth/satellite/agentapi/agent-membership` 写入 `agent_shared_tenant_members`，本站写入 `agent_users` 后签发 `member` 会话。注册用户不会成为 owner，主站 SSO 也会优先进入已加入的租户，而不是为成员新建一个站。

当前 AgentAPI Compose 仍是单副本 SQLite。收费行为以 [收费模块.md](收费模块.md) 为准。

2026-11-16 已完成本轮前端收口：控制台页面统一使用共享页面壳，卡片网格统一间距，统计卡等高，支付设置与站长设置的内嵌面板改用统一卡片头/内边距；用户管理从无内边距的大表格改为带筛选区、分隔线和响应式表头的卡片。当前功能与主站关系详见 [功能说明.md](功能说明.md)。

## 目录

- `backend/`：Go 服务、数据层、Sub2API 客户端、模型代理与测试。
- `frontend/`：Vue 3 用户界面、站长控制台与组件测试。
- `Dockerfile`、`docker-compose.yml`：当前共享代码的单副本 SQLite 开发/迁移部署，不是最终多副本生产拓扑。
- 逐站 `agentapi-provision*` 命令和 worker 已删除；新租户开通只在共享运行时中创建逻辑租户和 owner 关系。
- `收费模块.md`：按当前代码整理的收费说明。
- `功能说明.md`：当前功能清单、主站联系矩阵、计费边界和已知缺口。

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
- `AGENTAPI_ENTRY_TENANTS`：可选的站长域名绑定，格式为 `host=agt_...`，多个用逗号分隔。显式绑定优先于历史单租户 `agent_config.domain`，但已登录 Session 仍以自身租户为准。未登录访问绑定域名时用于品牌、登录和注册归属。
- `AGENT_EMAIL_VERIFY_ENABLED`：必须与主站 `email_verify_enabled` 一致。主站未配置 SMTP 或关闭邮箱验证时保持 `false`，否则注册入口会出现但提交会被 `EMAIL_VERIFY_REQUIRED` 拦截。
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

计费有两种模式，以租户的 `agent_config.billing_mode` 为准：

- 默认 `user_upstream`：主站直接扣当前用户的主站余额，本站钱包不参与。AgentAPI 在转发前读取余额用于拒绝明显不足的请求，不创建本地预留。
- 站长开启本站收费后：模型请求携带 `X-Sub2API-Payer`，主站从站长账户结算实际费用，AgentAPI 再按“实际费用 × 模型计费倍率”扣减用户本站钱包。充值进入的是本站钱包，不是主站余额。

主站仍是模型计费与权威用量的来源。AgentAPI 只保存明确标记来源和完整性的快照；未取得主站明细时显示待确认或不可用，不得使用余额差值、预留值或标准价估算冒充真实费用。

套餐目前在创建订单和支付履约上仍按普通充值处理：`order_type=subscription` 不会写入主站 `user_subscriptions`，也没有本地权益表。上线前不得把套餐宣传为已生效订阅，详见 [功能说明.md](功能说明.md) 第 7 节。

## 已知风险与上线门槛

以下问题按当前源码确认，不能以页面已存在视为已闭环：

- **P0 凭据**：`backend/admin_key_config.go` 留有主站管理员 Key，`backend/admin_credential.go` 在环境变量缺失或为占位符时会回退到内置值。必须移除回退并改为缺失即失败。
- **P0 部署**：根 `docker-compose.online.yml` 的 AgentAPI 服务只通过可选 `env_file` 取得运行配置，未显式注入 `SUB2API_APP_CREDENTIAL`。干净部署缺少该文件时，服务启动校验会失败或核心链路不可用。
- **P1 套餐**：套餐订单没有订阅权益，支付成功仍进入本站钱包；不得作为订阅售卖。
- **P1 取消**：用户/站长取消订单只改本站状态，没有调用渠道 `CancelPayment`，渠道侧仍可能完成支付。
- **P1 结算**：`FinalizeSettlement` 对 `released` 仍可能计算扣费；`debitUserUsageTx` 余额不足时把扣款截断为 0 却仍确认结算，缺少欠款/短款记录。
- **P1 退款**：本地退款未校验原始消费锚点，存在重复或超额退款风险。
- **P1 模型目录**：代码中的公开模型目录与 `../docs/卫星公开模型.md` 不一致；文档多出的模型会被 `MODEL_NOT_ALLOWED` 拒绝。
- **P1 凭据模型**：主站调用仍是“共享应用凭据 + On-Behalf-Of”，尚未迁移到每卫星独立凭据与不可伪造用户断言。

## 前端布局约定

- `AppLayout` 已提供页面外边距。控制台页面根节点使用 `.agent-page` 或 `.agent-page-narrow`，不要再次叠加 `p-6`、`mx-auto max-w-*`。
- 卡片网格使用 `gap-5`；同排统计卡加 `h-full`，避免高度参差。
- 新卡片优先使用 `.card`、`.card-header`、`.card-body`、`.card-footer`、`.stat-card`，不要复制一套灰色边框和内边距。
- 不要再使用 `gap-px` 把卡片拼成一块；独立卡片必须保留圆角、边框和间距。
- 公开首页、认证页和支付回调页是独立全屏布局，不套 `.agent-page`。

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

2026-11-16 最近一次结果：`go test ./...` 通过；`pnpm typecheck` 通过；`pnpm test:run` 为 67 个测试文件、343 项全部通过；`pnpm build` 通过。收费行为见 [收费模块.md](收费模块.md)，功能与主站联系见 [功能说明.md](功能说明.md)。

## 文档导航

- [接手文档.md](接手文档.md)：界面体系、主题规范、验证方式与交接约束。
- [功能说明.md](功能说明.md)：当前功能、主站联系矩阵、计费边界、完成度与已知缺口。
- [收费模块.md](收费模块.md)：按当前代码整理的收费说明。
