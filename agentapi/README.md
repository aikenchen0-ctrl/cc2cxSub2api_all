# AgentAPI 代理站

AgentAPI 是 Sub2API 的独立代理站入口。它提供代理站域名、品牌、用户归属、代理 API Key、模型控制台和站长管理界面；真实用户、余额、用量和模型计费始终由 Sub2API 负责。

业务与安全基线见仓库根目录的 `代理站规划.md`，公共层接入约束见 `../docs/卫星应用接入公共层.md`，公开模型名见 `../docs/卫星公开模型.md`。

## 当前架构

- 用户在代理站注册或登录时，AgentAPI 通过服务端公共卫星协议在 Sub2API 创建或验证真实用户，再保存 `agent_id + main_user_id` 映射。
- 用户创建的 `sk-agent-*` Key 只在当前 AgentAPI 实例生效；数据库只保存哈希，完整 Key 仅在创建时返回一次。
- 模型请求由 AgentAPI 校验用户归属、状态、模型白名单和主站余额后，按该用户的 `main_user_id` 转发到 Sub2API。
- Sub2API 对该真实用户完成一次计费。AgentAPI 不维护可消费子余额，不从站长共享钱包扣款。
- 代理站站长只是当前 `agent_id` 的管理员，只能管理本实例映射用户、品牌、模型策略、结算记录和审计记录。
- 所有表和查询均以 `agent_id` 为隔离边界；生产实例还会按 `AGENT_DOMAIN` 拒绝未知 Host。
- Logo、Title、favicon 和页面品牌来自当前实例配置，不从请求参数选择其他代理站。

`owner_upstream`、本地钱包、旧充值订单和逐站 runtime 凭据仅为旧实例迁移兼容。新实例默认并应保持 `AGENT_BILLING_MODE=user_upstream`；直结模式下本地充值、支付 webhook、额度同步和余额分配写接口均停用。

## 目录

- `backend/`：Go 服务、SQLite 数据层、Sub2API 客户端、模型代理与测试。
- `frontend/`：Vue 3 用户界面、站长控制台与组件测试。
- `Dockerfile`、`docker-compose.yml`：单实例容器部署。
- `backend/cmd/agentapi-provision/`：生成独立实例部署包。
- `backend/cmd/agentapi-provision-worker/`：消费主站开站任务并部署实例。

## 本地开发

后端：

```bash
cd agentapi/backend
cp .env.example .env
go run .
```

前端：

```bash
cd agentapi/frontend
pnpm install
pnpm dev
```

前端生产构建会输出到 `backend/web/`，由 Go 服务同源托管：

```bash
cd agentapi/frontend
pnpm build
```

## 生产配置

至少配置：

- `MAIN_API_URL`：Sub2API 主站 API Origin。
- `MAIN_MODEL_URL`：Sub2API 模型网关 Origin；容器内地址可用 `SUB2API_RELAY_BASE_URL` 覆盖。
- `SUB2API_APP_CREDENTIAL`：服务端公共卫星应用凭据，禁止进入前端环境。
- `SUB2API_SSO_SECRET`：与 Sub2API 一致的公共层 SSO Secret。
- `SESSION_SECRET`：AgentAPI HttpOnly Session 加密密钥。
- `AGENT_ID`、`AGENT_DOMAIN`、`AGENT_OWNER_MAIN_USER_ID`：当前代理站实例身份与站长主站用户 ID。
- `AGENT_BILLING_MODE=user_upstream`：用户主站直结模式。
- `COOKIE_SECURE=true`：生产 HTTPS Cookie。

同机多实例应使用不同数据库、端口、域名、`AGENT_ID` 和站长用户 ID，并由反向代理保留真实 Host。不要把 Sub2API 管理凭据、应用凭据、主站 JWT 或用户主站 API Key 写入浏览器配置、URL、日志或 AgentAPI 数据库。

## 充值

AgentAPI 不创建本地支付订单。充值页读取公开设置中的 `recharge_url`，仅允许 HTTP(S) 地址，并跳转到 Sub2API 主站充值。充值完成后，代理站重新读取该用户的真实主站余额。

## 验证

```bash
cd agentapi/backend
go test ./...

cd ../frontend
pnpm typecheck
pnpm test:run
pnpm build
```

当前回归测试覆盖注册映射、用户直结身份、余额不足前置拒绝、用量归属、API Key 一次性显示与撤销、用户停用后 Session/Key 失效、未知 Host 拒绝、品牌持久化、管理员权限、跨用户数据范围、充值跳转安全和异步图片/视频任务结算。

## 公共层约束

- 从 Sub2API 左侧菜单进入时沿用现有 `GET /api/v1/auth/integrations/:slug/start`。
- 浏览器只持有 AgentAPI HttpOnly Session，不接触任何主站管理凭据。
- 服务端调用公共卫星接口时使用既有应用凭据与代理请求头，不另发明 SSO 或 Key 传递协议。
- 模型入口只公开 `docs/卫星公开模型.md` 中允许的模型名，并可由当前代理站站长进一步收窄 allowlist。
