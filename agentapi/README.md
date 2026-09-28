# AgentAPI 代理站设计文档

本目录记录 AgentAPI 的规划、卫星运行时和部署方式。AgentAPI 是独立部署的代理站应用，拥有自己的域名、页面、品牌配置和运营数据库；Sub2API 仍是用户、模型请求、余额流水和用量的最终业务中心。跨项目的开站控制 API、数据库变更通知、SSO 注册和管理员页面位于 `../sub2api/`，不得由 AgentAPI 直接读写 Sub2API 数据库。

AgentAPI 运行时代码位于 `backend/` 与 `frontend/`，Docker 入口位于 `Dockerfile` 和 `docker-compose.yml`；主站接口与控制台位于 Sub2API 后端/前端。生产启用前仍应完成 [14-待拍板事项](14-待拍板事项.md) 中的业务确认和验收。

单实例部署可复制 `backend/.env.example` 为 `backend/.env` 后执行 `docker compose up -d --build`；同机多实例时复制根目录 `.env.example` 并为每个实例设置不同的 `AGENTAPI_PORT`，再由 Nginx 按域名转发。

平台子域名的一键部署包可以由服务端 CLI 幂等生成：

```bash
cd agentapi/backend
# `agt_...` 必须使用 Sub2API POST /api/v1/agent-provisioning/agents 返回的 agent_id。
go run ./cmd/agentapi-provision \
  --state-dir ../provisioning \
  --idempotency-key request-20260922-agent01 \
  --agent-id agt_0123456789abcdef0123456789abcdef \
  --slug agent01 --domain agent01.cc2.cx \
  --display-name "Agent 01" --owner-main-user-id 42 \
  --main-url https://api.cc2.cx --image registry.example/agentapi:v1
```

发布镜像同时包含 `/app/agentapi-provision`，控制面也可以覆盖镜像 entrypoint 调用同一工具，无需在宿主机安装 Go。

生成包包含独立 Compose、Nginx、非秘密 `.env` 和状态记录；CLI 只为实例生成 Session Secret。公共卫星应用凭据与 SSO Secret 由运维侧分别通过 `SUB2API_APP_CREDENTIAL_FILE`、`SUB2API_SSO_SECRET_FILE` 挂载。独立 `agentapi-provision-worker` 使用开站专用凭据和数据库变更 SSE 驱动 Compose/Nginx 部署，检查 DNS allowlist、TLS、`/healthz`、`/readyz`，并显式激活开站记录。SSE 只推送 Agent ID，worker 随后 GET 权威记录并 claim；主站以 90 秒可续租 lease 保护 progress/activate，worker 默认每 20 秒续租，失租取消部署 context。claim token 只在 no-store 响应返回一次，主站只存 SHA-256 哈希。新流程不生成或登记逐站 runtime control/model 凭据；旧文件和旧接口仅用于迁移兼容。租约不能为 Docker/Nginx 副作用提供 fencing。DNS 记录和通配符证书仍由平台基础设施提供。worker 尚需安装到受控 edge 节点并完成真实环境验收。

当前参考实现已覆盖：主站注册/登录代理、服务端加密 Session、代理钱包和用户子余额、白名单 `/v1` 模型端点、Agent 管理员可配置的本地公开模型 allowlist、JSON/multipart 模型请求校验、显式文本流式转发、视频任务归属与受控轮询、`sk-agent-*` 客户端 Key、实例品牌和支付配置、代理管理员额度分配、事务性 settlement 幂等、Owner usage 对账、高风险操作审计、浏览器模型控制台、健康/就绪检查、单容器 Docker 部署，以及独立开站 worker 源码。新实例使用公共卫星应用凭据，并始终以实例服务端固定的 `AGENT_OWNER_MAIN_USER_ID` 调用模型、余额和 usage；浏览器用户身份只用于 AgentAPI 本地归属和额度控制。旧实例可显式开启逐站 runtime 兼容模式。独立 worker 使用受限的 provisioning-only 凭据访问 Sub2API 开站 API，并以 ID-only SSE + 权威 GET、90 秒可续租 claim、租约保护的进度/激活、Compose/Nginx、DNS/TLS/readyz 检查和失租取消完成自动化。worker 尚未安装配置到实际 edge 运维节点，生产开站闭环仍未验收。

Profile 页面已补齐主站用户名修改和改密：只通过当前 Session 对应的 Sub2API 用户自助 API，SSO identity-only Session 保持只读，改密后撤销本地 Session。

新实例的模型 allowlist 由 AgentAPI 在本地 `/v1` 请求入口和 `/v1/models` 发现接口执行。旧 runtime 兼容模式仍可把范围同步到逐站 model credential。

AgentAPI 托管运行时不依赖全局管理员 Key；普通模式通过公共卫星 `/v1` 接口读取固定 Owner 的余额、usage 并调用模型。开站控制面由主站管理员操作，外部 worker 使用独立 provisioning-only 凭据；逐站 runtime API 只保留给旧实例迁移。

Agent Owner 还可以在控制台启停本代理已映射用户；普通模式只修改当前实例的本地访问状态，不影响该用户在 Sub2API 或其他实例的状态。模型目录支持按 Agent 配置公开模型 allowlist，用量页则按本地 settlement 分页并展示来自主站 usage 的安全字段快照。模型控制台支持同步与异步图片 generation/edit；异步任务归属和结算由 AgentAPI 服务端管理，并按原 request ID 查询主站 usage。当前用户的图片/视频任务索引可分页恢复，刷新或重开控制台后继续轮询原任务，不会重复创建生成请求。

## 当前充值能力

AgentAPI 已包含一个默认关闭的 provider-neutral 充值桥接：用户可以创建充值订单，支付供应商通过 HMAC webhook 报告结果，系统在同步到足够的主站 Owner 额度后把额度原子分配到用户子余额。支付回调不会直接增加主站余额，也不产生佣金；主站暂时不可用或额度不足时订单保持 `paid_pending_allocation`，可由管理员或后台 reconciler 重试。

当前尚未接入真实支付供应商 SDK、退款/拒付、渠道对账和主站自动入账。启用前请阅读 [09-支付与充值](09-支付与充值.md)、[16-实现状态与边界](16-实现状态与边界.md) 和 [18-实现与接口手册](18-实现与接口手册.md)。

## 已确定的总体方案

1. **代理站独立部署**：AgentAPI 可以作为一个可复用的 Docker 镜像运行，每个代理使用独立配置；不复制一套 Sub2API 后端。
2. **主站保存真实用户**：用户从代理站注册时，由 AgentAPI 调用主站受控接口创建真实用户，并保存 `main_user_id` 映射。
3. **代理数据库只保存映射和运营数据**：不复制主站密码、SuperKey 或完整用户账务。
4. **采用 A 方案**：代理先向主站预存可消费额度，代理用户消费时扣减代理额度。
5. **不使用佣金策略**：没有佣金、分成、差价或佣金提现流程；主站只记录实际成本和用量。
6. **管理员凭据只在服务端使用**：浏览器永远不接触管理员 Key、SuperKey、主站 JWT 或用户 API Key。
7. **主站接口采用公共卫星协议**：运行实例使用公共 `SUB2API_APP_CREDENTIAL` 和服务端固定 Owner；全局管理员 Key、SuperKey 和旧逐站 runtime 凭据都不进入浏览器，新实例不依赖 runtime 表。
8. **不另发明 SSO/API Key 协议**：如果 AgentAPI 接入 Sub2API 左侧菜单，沿用仓库现有 HMAC SSO、3 天 Session、`SUB2API_APP_CREDENTIAL` 与三个代理请求头。

## 文档阅读顺序

| 顺序 | 文档 | 解决的问题 |
| --- | --- | --- |
| 1 | [00-设计基线与决策](00-设计基线与决策.md) | 术语、已确定事项、未决事项 |
| 2 | [01-业务边界与目标](01-业务边界与目标.md) | 谁负责什么、哪些内容不做 |
| 3 | [02-总体架构与请求链路](02-总体架构与请求链路.md) | 从开站到请求的完整链路 |
| 4 | [03-领域模型与数据结构](03-领域模型与数据结构.md) | 实体、字段、约束和数据归属 |
| 5 | [04-一键开分站流程](04-一键开分站流程.md) | 开站编排、状态和回滚 |
| 6 | [05-主站接口契约](05-主站接口契约.md) | AgentAPI 与 Sub2API 的接口边界 |
| 7 | [06-认证与安全](06-认证与安全.md) | 凭据、Session、权限和威胁防护 |
| 8 | [07-预充值与账务](07-预充值与账务.md) | A 方案的余额、消费和流水规则 |
| 9 | [08-用户注册与模型请求](08-用户注册与模型请求.md) | 用户如何注册、登录和调用模型 |
| 10 | [09-支付与充值](09-支付与充值.md) | 代理支付配置、订单和回调 |
| 11 | [10-部署域名与配置](10-部署域名与配置.md) | Docker、DNS、TLS、Nginx 和环境变量 |
| 12 | [11-状态机幂等与故障恢复](11-状态机幂等与故障恢复.md) | 重试、失败、冲正和对账 |
| 13 | [12-测试验收清单](12-测试验收清单.md) | 如何证明闭环可用 |
| 14 | [13-实施顺序与风险](13-实施顺序与风险.md) | 分阶段落地和风险控制 |
| 15 | [14-待拍板事项](14-待拍板事项.md) | 开发前必须确认的选择 |
| 16 | [15-客户端API Key与接口手册](15-客户端API Key与接口手册.md) | 客户端凭据、模型调用和错误处理 |
| 17 | [16-实现状态与边界](16-实现状态与边界.md) | 设计项与当前代码能力的对应关系 |
| 18 | [17-账务权威与主站扣费桥接](17-账务权威与主站扣费桥接.md) | 代理用户、代理管理员与主站余额的唯一扣费关系 |
| 19 | [18-实现与接口手册](18-实现与接口手册.md) | 当前代码的路由、请求、错误码和验收行为 |
| 20 | [19-上线与运维手册](19-上线与运维手册.md) | Docker、Nginx、密钥、对账、备份、撤站 |
| 21 | [20-版本变更记录](20-版本变更记录.md) | 冻结决策、已实现能力和后续变更规则 |
| 22 | [21-闭环追踪矩阵](21-闭环追踪矩阵.md) | 把开站、注册、充值、消费、对账和撤站串成可验收闭环 |

## 与仓库公共层的关系

AgentAPI 如果作为独立代理站直接部署，可以使用自己的入口域名；如果以后挂入 Sub2API 左侧菜单，必须遵循：

- 启动接口使用现有 `GET /api/v1/auth/integrations/:slug/start`；
- 回调使用公共 HMAC 票据，不携带明文 Key；
- 浏览器只保存 AgentAPI 的 HttpOnly Session；
- 服务端调用 `/v1` 时发送 `SUB2API_APP_CREDENTIAL`、`X-Sub2API-On-Behalf-Of`、`X-Sub2API-Satellite`；
- Session/JWT/Cookie 生命周期使用 3 天；
- 模型出站只使用 `docs/卫星公开模型.md` 中的公开模型名。

这些规则来自：

- `docs/卫星应用接入公共层.md`
- `docs/卫星公开模型.md`
- `本地链接.txt`
