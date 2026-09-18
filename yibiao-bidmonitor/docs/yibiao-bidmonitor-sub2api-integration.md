# Yibiao BidMonitor 与 Sub2API 集成说明

本文说明 `OpenBidKit-Yibiao-Web`、`BidMonitor-AI` 和 `cc2cxSub2api_all` 的融合目录如何启动、配置和接入。最终目录名为 `yibiao-bidmonitor`，放在载体仓库根目录下。

## 1. 目录和职责

```text
yibiao-bidmonitor/
├── client/                 # OpenBidKit React 页面
├── server/                 # OpenBidKit Fastify、JWT、Prisma 和监控代理
├── bid-monitor/            # BidMonitor-AI Python 核心和内部 FastAPI
├── deploy/                 # 部署文件
├── docs/                   # 项目文档
├── LICENSES/              # 来源项目归属记录
└── README.md
```

浏览器只访问 `client` 通过反向代理暴露的 `/api`。Python 服务只监听内网回环地址，Fastify 使用内部令牌调用它。用户登录、模块权限、公告索引和审计边界由 OpenBidKit 负责。

来源基线：

- OpenBidKit：`https://github.com/jdcome/OpenBidKit-Yibiao-Web`
- BidMonitor-AI：`https://github.com/zhiqianzheng/BidMonitor-AI`
- BidMonitor-AI 搬运提交：`63a0e13`（`v1.7`）
- 载体仓库：`https://github.com/aikenchen0-ctrl/cc2cxSub2api_all`

## 2. 环境变量

### OpenBidKit Server

在 `server/.env` 中配置：

```dotenv
DATABASE_URL=postgresql://user:password@127.0.0.1:5432/yibiao_web
JWT_SECRET=replace-with-at-least-32-random-characters
BID_MONITOR_URL=http://127.0.0.1:8080
BID_MONITOR_SERVICE_TOKEN=replace-with-a-random-internal-token
BID_MONITOR_TIMEOUT_MS=10000
SUB2API_SSO_SECRET=replace-with-a-random-shared-secret
SUB2API_SSO_AUTO_PROVISION=false
```

`BID_MONITOR_SERVICE_TOKEN` 必须和 Python 服务完全一致，长度至少 32 个随机字符。真实值只能放在部署环境或密钥管理器中，不能写入 Git、浏览器、日志或页面配置。

`SUB2API_SSO_SECRET` 必须和 Sub2API 一致且至少 32 个字符；`SUB2API_SSO_AUTO_PROVISION=false` 时，管理员需要先为外部主体建立身份映射。

### Python BidMonitor

在运行 Python 服务的进程环境中配置：

```dotenv
BID_MONITOR_HOST=127.0.0.1
BID_MONITOR_PORT=8080
BID_MONITOR_SERVICE_TOKEN=replace-with-the-same-random-internal-token
BID_MONITOR_DATA_ROOT=/var/lib/yibiao-bidmonitor/users
```

通知渠道凭据和模型服务凭据不放进浏览器请求。当前 Python 服务支持来源项目的通知适配器；生产接入时应由服务端密钥配置或受控内部代理提供凭据。

## 3. 启动顺序

### 开发环境

先启动 PostgreSQL，再在 `server` 目录执行：

```powershell
pnpm install --frozen-lockfile
pnpm exec prisma generate
pnpm exec prisma db push
pnpm run db:seed
pnpm run dev
```

另开终端，在 `bid-monitor` 目录执行：

```powershell
python -m venv .venv
\.venv\Scripts\python.exe -m pip install -r requirements.txt
$env:BID_MONITOR_HOST = '127.0.0.1'
$env:BID_MONITOR_PORT = '8080'
$env:BID_MONITOR_SERVICE_TOKEN = 'replace-with-the-same-random-internal-token'
$env:BID_MONITOR_DATA_ROOT = (Resolve-Path '.\data\users').Path
\.venv\Scripts\python.exe -m uvicorn service.app:app --host $env:BID_MONITOR_HOST --port $env:BID_MONITOR_PORT
```

前端在 `client` 目录执行：

```powershell
npm ci
npm run dev
```

启动后访问 OpenBidKit 的登录页。普通用户必须被管理员授予 `bid-monitor` 模块，管理员默认拥有该模块。

### 生产环境

生产环境使用 Nginx 或同类反向代理对外暴露前端和 Fastify，只转发 `/api` 到 `server`。Python 绑定 `127.0.0.1` 或独立内网网段，不配置公网域名，不直接开放 CORS。持久化目录至少需要备份：

```text
PostgreSQL 数据库
/var/lib/yibiao-bidmonitor/users
OpenBidKit 上传和导出目录
```

健康检查：

```powershell
Invoke-RestMethod http://127.0.0.1:8080/internal/health -Headers @{ 'X-BidMonitor-Service-Token' = $env:BID_MONITOR_SERVICE_TOKEN }
Invoke-RestMethod http://127.0.0.1:3000/api/monitor/status -Headers @{ Authorization = "Bearer $token" }
```

第二个请求必须使用已登录用户的 OpenBidKit JWT，并且该用户必须拥有 `bid-monitor` 权限。

## 4. Sub2API 菜单和 SSO

### 4.1 菜单入口

在 `cc2cxSub2api_all` 的自定义菜单中增加一个外部应用入口，地址指向融合项目的公开域名，例如：

```text
名称：Bid Monitor
地址：https://bidmonitor.example.com/
打开方式：新窗口或当前窗口均可
```

菜单只负责导航。应用的业务权限仍由 OpenBidKit 的 `bid-monitor` 模块控制。

### 4.2 票据格式

Sub2API 与 OpenBidKit 共用 `SUB2API_SSO_SECRET`，但 OpenBidKit 仍然签发自己的 JWT。票据载荷至少包含：

```json
{
  "iss": "sub2api",
  "aud": "yibiao-bidmonitor",
  "sub": "external-user-id",
  "email": "user@example.com",
  "username": "user",
  "displayName": "User",
  "iat": 1700000000,
  "exp": 1700000120,
  "jti": "random-one-time-id",
  "next": "/"
}
```

完整票据是：

```text
base64url(JSON payload).base64url(HMAC-SHA256(payload, SUB2API_SSO_SECRET))
```

票据有效期最多 120 秒，`jti` 只能消费一次，`aud` 必须固定为 `yibiao-bidmonitor`，`next` 只能是融合项目内部的相对路径。Sub2API JWT、模型 API Key 和服务令牌不能放入票据。

### 4.3 载体仓库需要提供的启动接口

载体仓库的 Sub2API 后端应增加与现有兄弟应用一致的登录启动接口，例如：

```text
GET /api/v1/auth/integrations/yibiao/start?next=/
```

接口使用当前 Sub2API 登录用户生成票据，并返回：

```json
{
  "redirect_url": "https://bidmonitor.example.com/api/auth/sso/callback?ticket=..."
}
```

生产配置：

```dotenv
SUB2API_SSO_SECRET=<与 OpenBidKit 相同的随机密钥>
YIBIAO_SSO_CALLBACK_URL=https://bidmonitor.example.com/api/auth/sso/callback
```

若载体仓库尚未包含 `yibiao` 专用 handler，先按 `sub2api/backend/internal/handler/aiexcel_sso.go` 的结构增加 handler、路由和测试，再在自定义菜单中调用该启动接口。融合目录自身不接收或解析 Sub2API JWT。

### 4.4 OpenBidKit 回调边界

回调接口负责验证票据、校验重放、映射本地用户并创建 OpenBidKit 会话。新用户是否自动创建由服务端显式配置控制；默认建议关闭自动开通，先由管理员建立或审批用户。被停用用户、受众错误、过期票据、签名错误和重复票据都必须失败。

## 5. API 与数据隔离

浏览器访问的接口：

```text
GET    /api/monitor/status
POST   /api/monitor/start
POST   /api/monitor/stop
POST   /api/monitor/run-once
GET    /api/monitor/config
PUT    /api/monitor/config
GET    /api/monitor/results
GET    /api/monitor/logs
DELETE /api/monitor/history
```

Fastify 从 JWT 读取 `userId`，客户端不允许指定或覆盖这个值。Fastify 到 Python 的内部接口使用 `X-BidMonitor-Service-Token`，Python 只接受内部地址和内部令牌。Python 数据目录按 `<data-root>/<userId>/` 隔离，Prisma 业务表的读取、写入和删除都必须带 `userId`。

## 6. 故障检查

1. 页面显示服务未配置：检查 `BID_MONITOR_URL` 和两端 `BID_MONITOR_SERVICE_TOKEN`。
2. 页面显示无模块权限：管理员在用户管理中授予 `bid-monitor`，然后刷新 `/api/me`。
3. Python 健康检查通过但运行失败：检查用户目录权限、站点网络、关键词配置和通知凭据。
4. 结果为空：先调用 `run-once`，再检查 Python 日志和外部站点返回；单站点失败不会代表整个服务不可用。
5. SSO 失败：检查回调地址路径、两端密钥、服务器时钟、票据受众和 `jti` 是否被重复使用。
6. 不要通过打开 Python 公网端口、关闭令牌校验或把模型密钥写入前端来绕过故障。

## 7. 发布前清单

- [ ] `server` 和 `bid-monitor` 使用同一随机内部令牌。
- [ ] Python 只绑定内网地址，数据目录不在 Git 中。
- [ ] PostgreSQL 已执行 `prisma generate` 和 `prisma db push`。
- [ ] 管理员已授予目标用户 `bid-monitor`。
- [ ] Sub2API `aud` 为 `yibiao-bidmonitor`，回调路径准确。
- [ ] 票据过期、错误签名、错误受众、重复消费测试已通过。
- [ ] 浏览器响应和日志中没有服务令牌、模型密钥和通知凭据。
- [ ] 已备份数据库、用户数据目录、上传目录和导出目录。
