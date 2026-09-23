# Yibiao BidMonitor 融合实施计划

> 设计依据：`docs/superpowers/specs/2026-09-19-yibiao-bidmonitor-sub2api-design.md`

## 1. 交付结构与验证基线

### 任务 1：建立融合目录和来源归属文件

**文件：** `bid-monitor/`、`LICENSES/`、`README.md`、`docs/yibiao-bidmonitor-sub2api-integration.md`

**步骤：**

1. 从 `BidMonitor-AI` 搬运 `src/crawler`、`src/matcher`、`src/notifier`、`src/utils`、`src/ai_guard.py`、`src/monitor_core.py`、`requirements.txt` 和相关配置模板。
2. 保留 `OpenBidKit-Yibiao-Web` 的 `client`、`server`、`deploy` 和许可证文件。
3. 删除 Python 原静态页面和 Basic Auth 入口，不把它们接入对外服务。
4. 记录两个源项目的仓库地址、提交版本、许可证和本次改动边界。

**验证：** 检查目录清单、许可证文件、Python 导入路径和文档中的启动命令。

### 任务 2：为 Python 核心增加用户作用域和内部服务

**文件：** `bid-monitor/src/database/storage.py`、`bid-monitor/src/monitor_core.py`、`bid-monitor/service/app.py`、`bid-monitor/service/manager.py`、`bid-monitor/requirements.txt`

**步骤：**

1. 先写 `storage` 的测试，验证数据库路径可以按用户目录生成、公告指纹只在当前用户库内去重、历史清理不影响其他用户。
2. 让 `Storage` 接受绝对数据库路径，保留原有默认参数，避免破坏来源模块的单独使用方式。
3. 让 `MonitorCore` 接受外部配置、存储路径和可注入日志回调，避免依赖进程当前目录和全局配置文件。
4. 新增 `MonitorManager`，按用户维护 `MonitorCore`、停止事件、运行状态、日志环形缓冲和线程任务。
5. 新增 FastAPI 内部接口，校验 `X-BidMonitor-Service-Token`，实现健康检查、启动、停止、运行一次、状态、配置、结果和日志。
6. 对路径参数做规范化和用户目录校验；服务只绑定内部地址。

**验证：** `python -m py_compile`、Python 单元测试、无令牌拒绝、跨用户路径拒绝、同一用户重复请求幂等。

### 任务 3：先写 Fastify 作用域和内部客户端测试

**文件：** `server/src/monitor/types.ts`、`server/src/monitor/client.ts`、`server/src/monitor/scope.ts`、`server/src/monitor/client.test.ts`、`server/src/monitor/scope.test.ts`

**步骤：**

1. 用 Node 测试运行器覆盖用户配置默认值、分页边界、敏感字段脱敏和内部请求头生成。
2. 实现 `BidMonitorClient`，统一设置内部令牌、超时、JSON 响应校验和错误转换。
3. 实现作用域校验函数，拒绝空用户标识、路径穿越和超长配置。
4. 对 Python 服务不可用、超时和非 JSON 响应返回稳定的服务错误码。

**验证：** `npm run test`、`npm run build`。

### 任务 4：增加 Prisma 监控实体和仓储测试

**文件：** `server/prisma/schema.prisma`、`server/src/monitor/store.ts`、`server/src/monitor/store.test.ts`

**步骤：**

1. 增加监控配置、站点、关键词、联系人、运行记录、公告索引、通知记录和日志模型。
2. 所有业务模型写入 `userId` 或从配置关系得到 `userId`，增加用户查询索引和公告指纹约束。
3. 仓储方法默认接收 `userId`，读取、更新和删除都把作用域放入 `where` 条件。
4. 外部 Python 结果进入 Prisma 时只接受允许字段，限制正文和日志长度。
5. 生成迁移并验证 Prisma Client 类型。

**验证：** `prisma validate`、`prisma generate`、隔离仓储测试、迁移检查。

### 任务 5：接入 Fastify 路由、权限门和审计

**文件：** `server/src/auth/permissions.ts`、`server/src/routes/monitor.ts`、`server/src/monitor/service.ts`、`server/src/index.ts`

**步骤：**

1. 增加 `bid-monitor` 模块权限，并同步客户端可授予模块列表。
2. 实现登录用户可访问、模块授权用户可操作、管理员可查看服务摘要的路由组。
3. 将请求用户从 JWT 传入监控服务，禁止使用请求体中的用户标识覆盖认证上下文。
4. 启动、停止、运行一次、配置修改和历史清理写审计日志。
5. 将 Python 运行结果归一化为 Prisma 记录，使用请求编号避免重复导入和重复通知。

**验证：** 未登录、无权限、跨用户、重复请求和 Python 不可用场景的 Fastify 集成测试。

### 任务 6：加入前端监控模块

**文件：** `client/src/features/bid-monitor/api/monitor.ts`、`client/src/features/bid-monitor/pages/BidMonitorPage.tsx`、`client/src/app/AppRouter.tsx`、`client/src/app/menuConfig.ts`、`client/src/shared/permissions.ts`、`client/src/shared/types/navigation.ts`、样式文件

**步骤：**

1. 增加监控模块类型、菜单、权限映射和路由。
2. 增加状态、启动、停止、运行一次、结果、日志、站点和联系人 API hooks。
3. 页面包含运行状态、站点与关键词配置、结果表格、日志查看和联系人配置。
4. 所有请求复用现有 `http` 客户端，不在浏览器保存服务令牌、AI 密钥或通知凭据。
5. 处理加载、空数据、错误、运行中和权限变更后的回退状态。

**验证：** `npm run typecheck`、`npm run build`、页面手工检查和权限深链检查。

### 任务 7：补充 Sub2API 接入文档和部署模板

**文件：** `docs/yibiao-bidmonitor-sub2api-integration.md`、`deploy/`、`.env.example` 文件、`README.md`

**步骤：**

1. 记录服务端口、内部令牌、数据目录、Python 启动命令和健康检查。
2. 记录 Sub2API 菜单地址、SSO 受众、回调地址、环境变量和单次票据流程。
3. 记录载体仓库需要增加的菜单配置或回调处理器，不写入真实密钥。
4. 补充开发、生产、升级、备份和故障排查命令。

**验证：** 文档命令与实际目录一致，环境变量名称在代码、部署模板和文档中一致。

### 任务 8：复制到载体仓库并执行发布前检查

**文件：** `cc2cxSub2api_all/yibiao-bidmonitor/`

**步骤：**

1. 复制融合项目目录，排除 `node_modules`、`.venv`、`dist`、`data`、日志、真实环境文件和数据库文件。
2. 检查载体仓库原有未提交改动，保证只新增 `yibiao-bidmonitor` 文件夹。
3. 在融合目录执行前端类型检查和构建、服务端构建、Prisma 校验、Python 编译检查和测试。
4. 执行目录级敏感信息扫描，确认没有 API Key、JWT Secret、服务令牌或用户数据。
5. 输出最终变更清单、验证结果和仍需外部站点联调的风险。

**验证：** 载体仓库 `git status`、目录哈希抽样、全部测试命令和最终文档链接。

## 2. 执行规则

每个任务先写能失败的测试，再写最小实现；测试通过后再连接下一层。复制到载体仓库只在源项目完成构建和安全扫描后执行。任何外部采集站点失败都记录为站点适配风险，不通过关闭用户作用域或放开内部接口解决。
