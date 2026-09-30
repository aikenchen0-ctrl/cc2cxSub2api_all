# AgentAPI 代理站

AgentAPI 是 Sub2API 的独立代理站入口。它提供代理站域名、品牌、用户归属、代理 API Key、模型控制台和站长管理界面；真实用户、余额、用量和模型计费始终由 Sub2API 负责。

代理站管理端明确不提供主站全局资源模块：分组管理、账号管理、代理管理、兑换码管理、插件管理、安全审计（风险控制与提示词审计）和上游核验均已从前端路由、菜单、客户端 API、服务端 API 与新库初始化中移除。普通用户自己的 `/redeem` 兑换入口不属于“兑换码管理”，继续按主站权威兑换协议使用。

业务与安全基线见 [代理站规划.md](代理站规划.md)，公共层接入约束见 `../docs/卫星应用接入公共层.md`，公开模型名见 `../docs/卫星公开模型.md`。

## 当前架构

- 用户在代理站注册时，AgentAPI 使用服务端 `SUB2API_ADMIN_API_KEY` 调用主站现有管理员创建用户接口（固定普通用户、零初始余额），成功后保存 `agent_id + main_user_id` 归属；登录仍由主站验证。该管理员密钥不进入浏览器，也不用于模型请求。未配置时关闭代理站注册。

主站负责所有实际计费和用量入库；代理站只跟踪请求并同步主站记录，不以余额差值重建账单。站长可在用户列表中管理本站下属用户的代理 API Key。部署权限、升级幂等键注意事项及剩余验收见 [主站事实与代理站管理边界](25-主站事实与代理站管理边界.md)。
- 用户创建的 `sk-agent-*` Key 只在当前 AgentAPI 实例生效；数据库只保存哈希，完整 Key 仅在创建时返回一次。
- 模型请求由 AgentAPI 校验用户归属、状态、模型白名单和主站余额后，按该用户的 `main_user_id` 转发到 Sub2API。
- Sub2API 对该真实用户完成一次计费。AgentAPI 不维护可消费子余额，不从站长共享钱包扣款。
- 代理站站长只是当前 `agent_id` 的管理员，只能管理本实例映射用户、品牌、模型策略、结算记录和本站操作日志；不能管理主站分组、账号池、代理池、插件、兑换码发行、安全策略或上游核验。
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
# 编辑配置后，将变量导入当前进程环境再执行 go run .。
# Go 程序不会自动加载 .env；推荐使用下述 Compose 启动方式。
```

前端：

容器启动（先在 `backend/.env` 配置与主站一致的 `SUB2API_SSO_SECRET`，通过同一文件提供 Compose 变量与服务端环境，避免两份 Secret 不一致）：

```bash
cd agentapi
docker compose --env-file backend/.env up -d --build
```

如果主站也在本机 Docker 中，使用附加网络配置，确保重建后仍可解析 `sub2api`，不依赖手动 `docker network connect`：

```bash
docker compose --env-file backend/.env -f docker-compose.yml -f docker-compose.local.yml config --quiet
docker compose --env-file backend/.env -f docker-compose.yml -f docker-compose.local.yml up -d --build
```

该配置要求主站已有 `deploy_sub2api-network`（可通过 `SUB2API_DOCKER_NETWORK` 指定实际名称），不会创建或修改主站网络。远程主站不需要附加此文件。不要公开不带 `--quiet` 的 Compose 配置输出，其中可能包含服务端密钥。启动前检查 `AGENTAPI_PORT`，避免占用已有实例端口；保留原项目名及数据卷，禁止使用 `down -v` 清空数据。

前端开发：

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
- `LINK`：主站公开 Origin，用于充值跳转；不配置则禁用充值跳转，不回退内部 API 地址。
- `SUB2API_ADMIN_API_KEY` 或 `_FILE`：主站普通用户创建权限，只有可信部署者可持有。兼容旧名称 `SUB2API_ADMIN_KEY`；新名称优先，显式配置的文件不可读时关闭注册，不回退旧密钥。
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

### 部署前只读预检

使用本次版本的二进制，在已安全注入部署环境变量的服务端执行：

```bash
./agentapi --preflight
# 开发环境可在 backend 目录执行：go run . --preflight
```

Go 不自动读取 `.env`。不要把密钥放在命令行参数中。已部署本次版本的容器可执行 `docker compose --env-file backend/.env exec -T agentapi /app/agentapi --preflight`；检查的是容器当前环境，修改文件后不会自动更新已有容器环境。

预检在打开数据库、初始化站点和启动 HTTP 服务之前退出。仅 GET 查询配置的站长主站资料及其卫星余额，不注册用户、不创建 Key、不调用模型。输出 JSON 检查状态和 HTTP 状态码，不输出地址、用户资料、余额、响应正文或密钥；全部通过退出 0，否则退出 1，网络检查总超时 20 秒。

`admin_user_read` 和 `satellite_balance_read` 独立检查，前者失败不会跳过后者。401/403 表示相应认证或授权检查失败；`configuration_missing_or_invalid` 表示缺少有效配置；`transport_error` 需排查网络、TLS 或超时。`sso_secret` 仅检查本地长度，不证明与主站一致；管理员 GET 成功也不证明注册 POST 权限、模型可调用、真实计费或跨站隔离通过。预检不是完整上线验收。

### Windows 本地运行配置快照

若部署时使用了内存中的 Compose override，旧 `backend/.env` 不一定代表正在运行的配置，不能直接据此重建。可在仓库根目录执行：

```powershell
./agentapi/scripts/RuntimeConfig.ps1 -Action Save
./agentapi/scripts/RuntimeConfig.ps1 -Action Check
```

`Save` 将当前容器业务环境、镜像标识、端口和数据卷名称保存到 `.local/agentapi-runtime/config.dpapi`，使用当前 Windows 用户的 DPAPI 加密；不会覆盖已有快照，需要新副本时指定新的 `-SnapshotPath`。快照不得提交或分享，且不能当作跨机器恢复备份。

`Check` 在内存中解密，核对运行环境及镜像/数据卷，再用保存的值渲染 Compose 并比对环境和数据卷；不打印配置内容、不启动或重建容器。它不验证管理员凭据是否有效，也不是自动回滚工具。后续重建必须显式使用经过核对的运行配置，不能假定本脚本已经更新旧 `.env`。数据库仍需单独备份。

### 自动化测试命令

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
