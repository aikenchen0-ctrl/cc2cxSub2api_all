# Yibiao BidMonitor 与 OpenBidKit 融合设计

## 1. 目标与边界

本设计把 `BidMonitor-AI` 的招标公告采集、关键词匹配、AI 过滤、通知和定时任务能力接入 `OpenBidKit-Yibiao-Web`，并把可运行的融合项目放入 `cc2cxSub2api_all/yibiao-bidmonitor/`。

融合后的用户入口、身份认证、权限判断和浏览器 API 统一由 OpenBidKit 提供。BidMonitor-AI 保留 Python 采集生态，作为只接受内部请求的监控服务运行。浏览器不直接访问 Python 服务，也不保存或接收模型密钥。

本轮不把 BidMonitor-AI 的 FastAPI 页面整体嵌入 OpenBidKit，不把现有 SQLite 存储直接暴露给 OpenBidKit，也不把两个项目的依赖强行合并为一个运行时。这样可以保留来源项目的站点适配器，同时让用户、权限、配置和结果的归属由同一套应用边界管理。

## 2. 现状与问题模型

### 2.1 行动者

| 行动者 | 主要诉求 | 当前边界 |
| --- | --- | --- |
| 业务用户 | 配置关键词、站点、通知联系人，查看匹配结果和运行日志 | 需要按用户隔离配置、结果和日志 |
| 管理员 | 管理模块权限、AI 配置和服务运行状态 | 可以查看服务级状态，但不能绕过审计访问用户数据 |
| OpenBidKit | 提供登录、权限、统一导航、持久化和 AI 配置 | 负责外部 API 和浏览器会话 |
| BidMonitor 服务 | 执行采集、匹配、AI 过滤、通知和调度 | 只接受内部服务令牌和用户作用域 |
| Sub2API | 提供统一入口、菜单配置和可选的单点登录票据 | 不把自身访问令牌当作业务应用令牌 |
| 外部招标站点 | 提供公告页面或接口 | 网络变化、限流、验证码和页面结构属于外部条件 |
| 通知渠道 | 发送邮件、短信、语音或企业微信通知 | 凭据只能在服务端保存，发送结果需要可追踪 |

### 2.2 核心矛盾

当前两个源项目的主要冲突是：BidMonitor-AI 依赖 Python 生态和全局运行状态，OpenBidKit 依赖 Fastify、Prisma、JWT 和多用户权限模型。若直接搬运 FastAPI 入口，会形成两套认证、两套用户数据和一份全局配置；若全部改写为 TypeScript，会丢失站点采集器的既有行为并扩大验证范围。

设计采用的代偿约束是增加一个内部 Python 服务边界，并由 Fastify 负责用户作用域映射。代偿成本是一次内部请求和一份跨语言 DTO；获得的收益是保留采集器、通知器和匹配器，同时把外部身份与数据权限收拢到 OpenBidKit。

## 3. 目标架构

```text
Sub2API 菜单或浏览器
          |
          v
OpenBidKit React
          |
          v
OpenBidKit Fastify + Prisma + JWT
          |  用户权限、用户作用域、审计、AI 配置
          |
          v
BidMonitor Python Service
          |  内部服务令牌、用户作用域
          +--> crawler 站点适配器
          +--> matcher 关键词匹配
          +--> ai_guard AI 过滤
          +--> notifier 通知适配器
          +--> scheduler 调度器
          +--> per-user storage 用户数据
```

### 3.1 进程边界

融合项目目录为：

```text
yibiao-bidmonitor/
├── client/                 # OpenBidKit 前端
├── server/                 # OpenBidKit API、鉴权、数据层和监控代理
├── bid-monitor/            # BidMonitor-AI 的 Python 核心服务
├── deploy/                 # 组合启动和环境模板
├── docs/                   # 集成说明和运维文档
├── LICENSES/               # 来源项目许可证与归属说明
└── README.md
```

OpenBidKit Fastify 只把 `/api/monitor/*` 暴露给已登录且拥有监控模块权限的用户。BidMonitor Python 服务监听内部地址，默认不绑定公网地址；其请求必须包含服务端共享令牌、用户标识和请求幂等标识。

### 3.2 模块迁移表

| BidMonitor-AI 模块 | 处理方式 | 融合后的责任 |
| --- | --- | --- |
| `src/crawler/` | 原样保留并补充适配接口 | 采集公告，输出统一公告对象 |
| `src/matcher/keyword.py` | 保留核心算法 | 按用户配置执行关键词匹配 |
| `src/ai_guard.py` | 保留过滤流程，改为服务配置注入 | 使用 OpenBidKit 下发的模型配置或内部 AI 代理 |
| `src/notifier/` | 保留渠道实现，包装用户作用域 | 读取用户通知配置并记录发送结果 |
| `src/scheduler/runner.py` | 保留调度能力，改为用户任务注册表 | 每个用户独立启动、停止和运行一次 |
| `src/database/storage.py` | 不作为外部接口直接复用 | 改为用户作用域存储适配器，结果主索引由 Prisma 管理 |
| `server/app.py` | 不直接暴露原入口 | 拆出业务服务、内部路由和状态管理 |
| `server/static/index.html` | 不迁移 | 由 OpenBidKit React 页面替代 |
| 原 HTTP Basic 认证 | 删除 | 使用 OpenBidKit JWT 加内部服务令牌 |

## 4. 数据与作用域设计

### 4.1 归属原则

用户配置、站点开关、关键词、通知联系人、匹配公告、运行记录和日志都必须带 `userId`。服务级信息只包括进程健康、版本、任务数量和最近错误摘要，不允许通过服务级接口读取任意用户的完整业务数据。

Prisma 作为用户和业务元数据的主索引；Python 存储层只保存采集过程需要的内容和去重字段，并使用 `data/users/<userId>/` 作为目录前缀。所有用户标识由 Fastify 从已验证 JWT 得到，客户端传入的用户标识只用于校验，不能决定访问范围。

### 4.2 推荐实体

| 实体 | 关键字段 | 作用 |
| --- | --- | --- |
| `MonitorProfile` | `id`, `userId`, `name`, `enabled`, `schedule` | 用户监控配置入口 |
| `MonitorSite` | `profileId`, `siteKey`, `enabled`, `options` | 站点开关和站点参数 |
| `MonitorKeyword` | `profileId`, `include`, `exclude`, `mode` | 匹配规则 |
| `MonitorContact` | `userId`, `channel`, `target`, `enabled` | 通知联系人，敏感字段加密或脱敏 |
| `MonitorRun` | `userId`, `profileId`, `status`, `startedAt`, `finishedAt`, `counts` | 单次运行状态 |
| `MonitorBid` | `userId`, `runId`, `fingerprint`, `title`, `url`, `source`, `publishedAt`, `matchedAt` | 统一公告索引 |
| `MonitorNotification` | `userId`, `bidId`, `channel`, `status`, `attempts`, `lastError` | 通知投递记录 |
| `MonitorLog` | `userId`, `runId`, `level`, `message`, `createdAt` | 用户可见日志 |

实体名称和字段可随现有 Prisma 命名约定调整，但必须保留 `userId` 作用域和唯一指纹约束。跨用户查询必须由服务层显式传入作用域，不允许提供无作用域的仓储方法。

## 5. API 边界

### 5.1 外部 Fastify API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/monitor/status` | 当前用户任务状态和服务摘要 |
| `POST` | `/api/monitor/start` | 启动当前用户的监控任务 |
| `POST` | `/api/monitor/stop` | 停止当前用户的监控任务 |
| `POST` | `/api/monitor/run-once` | 触发当前用户的一次运行 |
| `GET` | `/api/monitor/config` | 读取当前用户配置 |
| `PUT` | `/api/monitor/config` | 更新当前用户配置 |
| `GET` | `/api/monitor/results` | 分页读取当前用户匹配结果 |
| `GET` | `/api/monitor/logs` | 分页读取当前用户日志 |
| `DELETE` | `/api/monitor/history` | 清理当前用户历史数据 |
| `GET` | `/api/monitor/sites` | 读取可用站点及启用状态 |
| `GET` | `/api/monitor/contacts` | 读取脱敏联系人 |
| `PUT` | `/api/monitor/contacts` | 更新当前用户联系人 |

每个路由先执行 JWT 校验，再执行 `bid-monitor` 模块权限校验，最后从认证上下文得到 `userId`。启动、停止、清理和配置修改写入审计记录。

### 5.2 内部 Python API

内部接口不携带浏览器 JWT，只接受 Fastify 生成的服务令牌。请求体必须包含 `userId`、`requestId` 和接口所需的作用域数据；Python 服务不接受来自客户端的任意数据库路径。

```text
GET  /internal/health
POST /internal/users/{userId}/start
POST /internal/users/{userId}/stop
POST /internal/users/{userId}/run-once
GET  /internal/users/{userId}/status
GET  /internal/users/{userId}/results
GET  /internal/users/{userId}/logs
PUT  /internal/users/{userId}/config
```

Fastify 对路径中的 `userId` 与认证上下文再次比对。Python 服务对用户目录做规范化检查，禁止 `..`、绝对路径和跨用户读取。

## 6. AI 配置与 Sub2API 单点登录

### 6.1 AI 配置

OpenBidKit 继续作为 OpenAI 兼容配置的唯一管理方，浏览器只看到模型名称、启用状态和过滤参数。API 密钥只在服务端配置或密钥存储中出现，不写入 Prisma 普通业务字段、浏览器响应、日志和 Python 用户配置文件。

Python 服务需要执行 AI 过滤时，由 Fastify 生成一次性内部调用上下文，或调用仅在内网开放的 AI 代理。两种实现都必须满足：请求带用户作用域、超时有限、错误可重试、响应大小受限、密钥不下发到浏览器。

### 6.2 Sub2API 接入

Sub2API 菜单指向融合项目的统一入口。单点登录使用短时效、单次消费的 HMAC 票据，票据至少包含发行方、受众、外部主体、签发时间、过期时间和唯一编号。OpenBidKit 回调验证签名、受众、时钟窗口和重复使用状态后，把外部主体映射到本地用户，再签发 OpenBidKit 自己的会话令牌。

票据受众固定为 `yibiao-bidmonitor`，不能复用其他子应用的受众。`next` 只能是融合项目内部的相对路径。回调地址、共享密钥、时钟容差和允许的来源均通过环境变量提供，生产环境禁止使用示例密钥。

## 7. 部署与运行

组合部署至少包含以下服务：

```text
yibiao-client      OpenBidKit React 静态资源
yibiao-server      OpenBidKit Fastify API
yibiao-monitor     Python BidMonitor 内部服务
database           OpenBidKit PostgreSQL
```

开发环境允许分别启动 Node、Python 和数据库；生产环境使用 `deploy/` 下的组合配置。Python 服务的监听地址只绑定内部网络，数据目录、共享令牌、AI 代理地址、抓取并发数、请求超时和通知凭据均从环境变量或服务端配置读取。

健康检查分为三层：Fastify 进程健康、Python 内部服务健康、外部采集器可用性。外部站点失败不能使整个 API 进程退出；单个用户任务失败只影响该用户的运行状态。

## 8. 安全与失败处理

1. 浏览器永远不直接访问 Python 服务。
2. JWT 只用于 OpenBidKit 外部会话，内部服务令牌只用于 Fastify 到 Python 的调用。
3. 每个配置、结果、日志和通知记录都必须带用户作用域。
4. 站点响应、公告正文、AI 输入和通知内容都要限制大小，并在日志中脱敏。
5. 启动、停止、配置修改、清理历史和通知失败都保留审计信息。
6. 任务重复触发使用 `requestId` 或数据库唯一约束避免重复运行和重复通知。
7. Python 进程重启后，从持久化配置恢复任务，不恢复正在执行的网络请求。
8. 单站点异常、单通知渠道异常和 AI 超时都降级为可见的运行错误，不阻塞其他站点和其他用户。

## 9. 测试策略与验收条件

实现前先为以下行为写测试：

1. 未登录用户不能访问监控 API。
2. 没有监控模块权限的用户不能启动、停止或读取结果。
3. 用户 A 不能读取、修改或清理用户 B 的配置、结果和日志。
4. Fastify 到 Python 的无效服务令牌被拒绝。
5. 用户启动、停止和运行一次请求能映射到正确的 Python 作用域。
6. 同一公告在同一用户作用域下按指纹去重，不同用户互不影响。
7. 同一通知请求重试不会产生重复发送。
8. SSO 票据过期、受众错误、签名错误和重复消费都会失败。
9. AI 配置不会出现在浏览器响应、日志或用户配置文件中。
10. 主要站点采集器、关键词匹配和通知适配器保留原有行为。

验收时执行前端类型检查和构建、Fastify 单元及集成测试、Python 语法与测试、Prisma 迁移校验、组合启动健康检查，以及一次包含启动、采集、匹配、结果展示和通知记录的端到端流程。

## 10. 交付顺序

1. 复制 OpenBidKit 到 `yibiao-bidmonitor/`，保留客户端、服务端、部署和文档结构。
2. 将 BidMonitor-AI 的采集、匹配、通知和调度模块放入 `bid-monitor/`，先建立独立可启动的内部服务。
3. 在 Prisma 中增加监控实体和用户作用域约束，完成迁移与仓储测试。
4. 在 Fastify 增加监控代理、权限门和审计记录。
5. 在 React 增加监控配置、运行状态、结果、日志和联系人页面。
6. 接入 Sub2API 的菜单入口和 SSO 配置文档，必要时补充载体仓库的最小集成改动。
7. 运行分层测试和组合启动验证，再把最终目录复制到 `cc2cxSub2api_all/yibiao-bidmonitor/`。

## 11. 明确的风险

外部招标站点的页面结构和反爬策略可能导致部分采集器需要单独维护；这属于站点适配风险，不应通过放宽用户数据边界来解决。通知凭据和 AI 服务的供应商差异可能需要按渠道增加配置适配器。若载体仓库的 Sub2API 后端尚未提供 Yibiao 专用回调处理器，首次交付可以先提供融合项目的 SSO 回调和菜单配置，并在集成文档中列出载体仓库需要应用的最小环境变量与路由改动。
