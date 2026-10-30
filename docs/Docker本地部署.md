# Docker 本地部署

> **AI 部署前必读：[部署清单.md](部署清单.md)。** 统一分类为 11 个卫星应用、AgentAPI 代理站、Sub2API 主站；归因探测是主站内部模块。Cost Console 仅为参考项目，不部署、不启动、不计入运行依赖。业务全量为 13 个项目入口、16 个应用容器、6 个依赖容器。下文按日期保留历史执行结果，不能直接当作下一次部署范围。

## 2026-10-08 Livart 发送与 Agent 错误修复

本次仅覆盖 **1 个 Sub2API 主站、1 个 Livart 卫星、0 个代理站，共 2 个应用容器**。其 **5 个依赖容器**（Sub2API PostgreSQL / Redis，Livart PostgreSQL / RabbitMQ / MinIO）继续使用现有实例和数据，已配置的健康检查通过；其余 10 个卫星、AgentAPI、参考 Cost Console 及仓库外容器不在本次范围内。

Livart 的发送按钮和 Ctrl+Enter 共用提交逻辑，提交时立即清空草稿；连续点击不重复提交，旧任务不会清除新草稿。意图识别和规划的服务端等待预算改为可配置的 150 秒，避免原先 20 秒意图识别预算在上游切换或慢响应时提前失败。超时按 HTTP 504 / `SPRING_AI_TIMEOUT` 返回。站内问答缺少可选向量配置时继续已有关键词检索，其他向量上游错误仍保留。Sub2API 对托管 OpenAI 兼容账号的接口 / 模型 404 继续切换账号；策略拒绝、上下文超限和 `previous_response_not_found` 保留既有请求错误处理。

最终镜像为 `local-deploy/livart-livart:livart-fix-20261008-140356` 和 `local-deploy/deploy-sub2api:livart-fix-20261008-143037`，对应容器均重新创建且健康，目标镜像匹配，环境变量、挂载、端口、网络、Entrypoint 和 Cmd 均核对保留。期间主站已有另一次账号导入部署，本次按更新后的运行快照保留其配置与数据库，不回滚该变化。

验证通过：Chromium 发送 / 清空 / 重复点击 / 新草稿保护回归；JDK 21 定向后端测试 61 项（Spring AI 11、规划 42、知识库回答 8）；Sub2API 404 定向测试 6 个测试函数，覆盖流式聊天切换以及续接上下文错误边界。真实 Livart SSO、3 天 HttpOnly Session、余额和 `no-store` 验证通过；`POST /api/agent/runs` 功能问答在最终部署后约 33 秒成功返回 HTTP 200。前一轮约 98 秒成功，日志确认期间遇到多个失败账号并继续切换；原始约 26 秒超时失败记录保留。未执行实际图片生成、编辑输出或全量卫星业务回归，不能将功能问答成功视为全部图片能力验收通过。

回滚镜像、数据库备份和实际 Compose 配置保存在 Git 忽略目录 `.local/livart-fix-20261008-132925/`、`.local/livart-fix-20261008-140356/`、`.local/livart-fix-20261008-143037/`。最终证据为最后目录中的 `integration-verification.json` 与 `final-runtime-audit.json`；这些本地目录包含私密配置和数据，不得公开或提交。

## 2026-10-08 业务全量覆盖部署

按本次用户要求覆盖部署 **1 个 Sub2API 主站、11 个卫星和 1 个 AgentAPI 代理站，共 13 个项目入口、16 个应用容器**。全部目标容器已重新创建，运行镜像与本次 `20261008-102159` 标签一致，环境变量、端口、网络和数据挂载经前后核对保留；**6 个依赖容器**继续使用已有实例和数据，已配置的健康检查通过。`sub2api-cost-console/` 及其已有三个参考容器明确排除，仓库外容器未纳入本次操作。AgentAPI 原收费模式和凭据保留，QRCode 冻结目录未修改；归因探测随主站部署，`/admin/upstream-audit` 返回 200。

14 个应用镜像使用实际运行配置对应的既有构建文件构建当前工作区源码；PPT 沿用此前已验证的本地 `Dockerfile.ppt.system-cache`，其应用源码仍来自当前工作区。AI 剪辑标准构建因 npm `ECONNRESET` 失败，AI 获客后端依赖下载持续缓慢；核对发现两者依赖清单与旧运行镜像完全一致、基础镜像 digest 未变，但对应 BuildKit 安装层未被复用。为完成本次部署，使用本地临时 `Dockerfile.aicut.dependencies`、`Dockerfile.aihuoke.dependencies` 复用已核验的 Linux 依赖，并重新复制当前应用源码。临时构建文件包含依赖清单 SHA-256 校验，清单变化时拒绝复用；AI 获客先清除基础镜像的 Python 包目录再完整复制旧依赖，`pip check` 通过。仓库原 Dockerfile 未修改。这是本次部署的依赖复用措施，不代表已为所有标准构建配置持久下载缓存；不能从现有日志断言缓存未命中的具体回收原因。

验收通过：13 个首页返回 200；16 个应用运行正常、镜像匹配，已配置的健康检查通过；11 个卫星和 AgentAPI 共 12 项 SSO 启动、回调与必要交换成功，登录后余额为 200 且带 `no-store`，匿名余额为 401，新签发 Session 均为 3 天并使用 HttpOnly Cookie，重放票据不签发 Cookie 且不能读取余额。Sub2API 拒绝将 SuperKey 当普通 API Key（403），拒绝缺少用户身份的卫星请求（401）；主站与 AgentAPI 健康接口均为 200。PPT LiteParse 导入和导出组件检查通过，导出版本为 `1.0.18`。未执行收费模型生成、支付退款或全量业务回归。

回滚镜像、Sub2API/Livart PostgreSQL 导出、AgentAPI 一致性 SQLite 备份、实际 Compose 配置和构建日志位于 Git 忽略目录 `.local/redeploy-all-20261008-102159/`，指针为 `.local/all-redeploy-current.txt`。最终证据为 `audit-after.json`、`verification.json`、`validation.json`、`dependency-checks.json`、`completion-summary.json`；两项依赖复用依据为 `aicut-dependency-reuse.json`、`aihuoke-dependency-reuse.json`。该目录含私密配置和数据，不得公开或提交。QRCode 切换期间一次旧容器移除并发错误已通过最终容器审计排除，已删除本次产生的未启动临时副本；应用入口与数据挂载正常。

## 2026-10-07 全项目源码部署

已从当前工作区构建并覆盖 Sub2API、全部 11 个卫星应用和 AgentAPI 代理站，共 16 个已有应用容器；当时还误将仅供参考的 `sub2api-cost-console/` 的 Web 服务独立部署到本机 Docker。因此历史实际合计 **14 个项目入口、17 个应用容器、8 个依赖容器**，此数字包含参考项目的误部署，不是当前业务部署范围。AgentAPI 不计入卫星数量；归因探测是 Sub2API 内部模块，随主站部署，不额外计数。Windows 桌面程序本身不在 Linux 容器中运行。

| 项目 | 本地入口 |
| --- | --- |
| Sub2API | `http://localhost:18080` |
| AgentAPI | `http://localhost:18081` |
| Cost Console（历史误部署的参考实例，不纳入当前范围） | `http://localhost:18082` |
| aicut | `http://localhost:5199` |
| aiexcel | `http://localhost:4173` |
| ai3d | `http://localhost:5174` |
| aihuoke | `http://localhost:3001` |
| canvas | `http://localhost:3522` |
| ju | `http://localhost:3000` |
| livart | `http://localhost:8080` |
| ppt | `http://localhost:8341` |
| qrcode | `http://localhost:5221` |
| screen2code | `http://localhost:5173` |
| yibiao | `http://localhost:8081` |

16 个已有应用容器均已重新创建，运行镜像与本次构建匹配；原环境变量、数据挂载和端口经部署前后核对保留。AgentAPI 原收费配置未改动。QRCode 只构建部署，未修改冻结目录。Cost Console 使用独立 PostgreSQL、Redis、命名卷和网络，入口仅绑定宿主机回环地址；它不是新增的主站 SSO 卫星，也未改动主站菜单。其初始管理员登录信息保存在本次私密部署目录的 `cost-console-login.txt`。

部署前发现 JU 前端会把共享网络中的 `backend` 解析成 Screen2Code 后端，导致间歇性 502。本次在实际运行的本地 Compose 配置中，将 JU 和 Screen2Code 前端限定到各自项目网络；两者后端仍连接 `deploy_sub2api-network`。已验证域名解析、SSO 与余额访问恢复正常。

PPT 首轮构建遇到 Debian 大型字体包反复下载失败，改用本地临时 `Dockerfile.ppt.system-cache`：复用已验证镜像中的系统依赖，先删除继承的 `/app`、Python 虚拟环境和模型缓存，再复制当前源码构建的 FastAPI、Next.js、导出组件、解析 runner 和配置。LiteParse 仍复用固定版本 `1.5.2` 的依赖安装目录。原项目 Dockerfile 未修改。新容器的 LiteParse 导入和导出组件 `--check-only` 均成功，导出版本为 `1.0.18`。字体目录实测约 758 MiB。

最终验收：14 个首页返回 200；所有应用及依赖运行正常，已配置的健康检查通过；11 个卫星应用和 AgentAPI 代理站（共 12 个 SSO 接入项目）经主站 SSO 启动、回调与必要交换后，余额查询均为 200、匿名余额均为 401、余额响应带 `no-store`，新签发的持久 Session Cookie 为 3 天，票据重放未签发新 Cookie。主站拒绝将 SuperKey 当普通 API Key（403），拒绝缺少用户身份的卫星请求（401）。AgentAPI 部署边界检查通过，Cost Console 初始管理员登录与身份查询通过。未执行收费模型生成、支付退款或全量业务回归。

本次标签为 `20261007-164350`。回滚镜像、Sub2API/Livart PostgreSQL 备份、AgentAPI 一致性 SQLite 备份、实际 Compose 配置及验收证据位于 Git 忽略的 `.local/redeploy-all-20261007-164350/`，指针为 `.local/all-redeploy-current.txt`。目录包含密钥和数据，不得公开或提交。主要证据为 `audit-after.json`、`verification.json`、`validation.json`、`cost-console.verification.json`、`agentapi-verification.json` 和 `ppt-dependencies.json`。部署期间仓库外的 `jk/crm-qq` 两个容器被其他操作更新，单独记录为外部变化，未回滚或修改它们。

当时的 `scripts/Start-LocalDocker.ps1` 曾加入 AgentAPI 和 Cost Console，运行后上述入口检查通过。用户随后明确 Cost Console 仅供参考，现已从该脚本的应用、依赖及首页检查列表中移除；脚本当前只启动 13 个业务项目，不负责源码重新构建。本次纠正未停止或删除历史参考实例，也未删除其数据。

## 2026-10-04 当前工作区部署

已将 Sub2API 和 `卫星应用.txt` 中全部 11 个卫星从当前工作区构建并部署到本机 Docker，共 15 个应用容器。15 个容器均已重新创建，运行镜像与本次构建匹配；原有环境变量、数据挂载、端口和 Docker 网络均通过切换前后核对。6 个依赖容器保持运行，已配置的健康检查均通过。原有未提交代码保留，未修改业务源码或 QRCode 冻结目录。

14 个应用镜像使用仓库原 Dockerfile。PPT 的 LiteParse 安装步骤长时间停滞，最终使用本机临时 `Dockerfile.ppt.dependency-cache`：仅复用旧镜像中已经验证的 LiteParse `1.5.2` 安装目录及包清单，并把资产构建阶段改为符合导出依赖要求的 Node 22；FastAPI、Next.js、解析 runner、导出脚本和配置继续来自当前源码，导出组件按项目固定版本重新安装。仓库 `ppt/Dockerfile` 未修改。新容器内 LiteParse 导入检查和 `presentation-export --check-only` 均通过，导出组件版本为 `1.0.18`。

最终验收通过：12 个首页均为 200；11 个卫星的主站 SSO 启动、回调和必要交换成功，登录后余额接口均为 200，匿名余额请求均为 401，余额响应均带 `no-store`，回调 Origin 均与本地入口一致，票据重放未签发新 Cookie。存在持久 Session Cookie 的项目其新签发期限均为 3 天。主站拒绝将 SuperKey 当普通 API Key 使用（403），拒绝没有用户身份的卫星调用（401）。该验收没有执行收费模型生成或全部业务回归，也不替代下文所述 Canvas/Livart 旧会话方式的改造。

镜像标签为 `local-deploy/<Compose项目>-<服务>:20261004-160629`，旧镜像保留为对应的 `local-rollback/...:20261004-160629`，Sub2API 和 Livart PostgreSQL 已备份。运行快照、构建配置、临时 PPT Dockerfile、构建日志、数据库备份、`audit-after.json`、`verification.json`、`gateway-boundaries.json` 和 `ppt-dependencies.log` 位于 Git 忽略的 `.local/redeploy-20261004-160629/`。该目录包含敏感配置及数据，不得公开或提交。`.local/local-redeploy-current.txt` 指向本次目录。主站旧容器引用的已丢失临时 Compose 文件已通过现存配置和运行参数恢复为本次持久化的本地部署配置。

AgentAPI 不属于上述 11 个卫星的部署清单，本次未重建其容器或调整收费模式。已完成源码接手梳理、后端测试、前端类型检查及 304 项前端测试；其运行实例虽健康，但主站应用凭据不一致、缺少公开 `LINK` 等问题仍需后续处理。详见 [AgentAPI 接手记录](AgentAPI接手记录.md)。

## 2026-10-01 源码覆盖部署

已按 `卫星应用.txt` 覆盖部署全部 11 个卫星及 Sub2API，共 15 个应用容器。所有镜像均通过当前工作区的标准 Dockerfile 构建（允许复用匹配的构建缓存），包括 PPT；本次未使用历史应急 Dockerfile，也未修改各应用源码。QRCode 仅构建、部署和只读验收，遵守目录冻结规则。AgentAPI 不在本次覆盖部署范围内。

部署前保留了 15 个旧镜像并备份 Sub2API、Livart PostgreSQL。部署后核对确认所有应用容器已重新创建、运行镜像与本次构建一致，原有环境变量、数据挂载、端口及 Docker 网络均保留。6 个依赖容器保持运行，已配置的健康检查均通过。

验收通过：12 个首页返回 200；11 个卫星通过主站 SSO 启动、回调及必要的会话交换，登录后余额接口均返回 200、匿名请求均为 401、余额响应均带 `no-store`，回调地址均为对应本地入口，重放票据未签发新 Cookie。未执行收费模型生成或全量业务回归。

本次镜像为 `local-deploy/<Compose项目>-<服务>:20261001-115020`，回滚镜像为 `local-rollback/<Compose项目>-<服务>:20261001-115020`。运行配置、构建与部署日志、数据库备份、`audit-after.json` 和 `verification.json` 保存于 Git 忽略的 `.local/redeploy-20261001-115020/`；其中包含敏感配置与数据，不得公开或提交。`.local/local-redeploy-current.txt` 指向此次目录。

## 2026-09-28 源码覆盖部署

已从当前工作区重新构建 Sub2API 和 `卫星应用.txt` 的全部 11 个卫星，并覆盖现有 15 个应用容器。原数据库、上传卷、端口和运行身份配置保留；未更新清单外应用。Ju 显式关闭桌面 relay 模式。

PPT 常规构建因 Debian `fonts-noto-cjk-extra` 下载失败而中断，最终复用本机旧镜像的系统依赖，重新构建并替换当前 FastAPI、Next.js、导出组件和配置。实际构建文件为 `.local/redeploy-20260928/Dockerfile.ppt`，不是旧代码镜像直接重启。

最终验证：15 个应用容器均运行且使用本次目标镜像，已配置的健康检查均通过；12 个首页返回 200；11 个卫星 SSO、登录后余额查询均成功，匿名余额均为 401，余额响应均为 no-store。未进行收费模型生成或全部业务回归；AI 剪辑全量类型检查的既有错误及历史未验证 bug 不因部署完成而视为解决。

回滚镜像标记为 `local-rollback/<项目>-<服务>:20260928`。本次运行配置、构建日志、镜像核对和接入验证保存在 Git 忽略的 `.local/redeploy-20260928/`，其中运行配置含密钥，不得公开或提交。该目录的 `results.json` 保留首轮失败记录，后续成功以 `final-audit.json`、`verification.json` 及 `ppt-source-rebuild.log` 为准。

本机 Docker Desktop 使用 Linux 引擎，已部署 `sub2api/` 和 `卫星应用.txt` 中全部 11 个应用。沿用现有镜像、Compose 项目和持久化数据，未清空数据库。入口均为 `http://localhost:端口`。

| 应用 | 端口 | Compose 项目 | 容器 |
| --- | --- | --- | --- |
| Sub2API | 18080 | deploy | sub2api |
| aicut | 5199 | aicut | aicut |
| aiexcel | 4173 | aiexcel | aiexcel |
| ai3d | 5174 | ai3d | ai3d |
| aihuoke | 3001 | ai-hunter-root | aihuoke-frontend / aihuoke-backend |
| canvas | 3522 | deploy | canvas-app |
| ju | 3000 | ju | ju-web-1 / ju-backend-1 |
| livart | 8080 | livart | livart-livart-1 |
| ppt | 8341 | ppt | ppt-production-1 |
| qrcode | 5221 | qrcode | qrcode |
| screen2code | 5173 | screen2code | screen2code-frontend-1 / screen2code-backend-1 |
| yibiao | 8081 | yibiao | yibiao |

共 15 个应用容器、6 个依赖容器。依赖包括 Sub2API PostgreSQL / Redis、Ju SSO Redis、Livart PostgreSQL / RabbitMQ / MinIO。未列入清单的其他已有容器不属于本次部署范围。

## 2026-10-08 Sub2API 线上备份导入

已将 `sub2api-20261008T054833Z.tar.gz` 中的完整 PostgreSQL 18 导出导入本机 Docker Desktop 的 Sub2API PostgreSQL。备份快照时间为 2026-10-08 05:48:47 UTC；导出校验通过，临时库恢复后 106 张表的记录数与 `restored-table-counts.txt` 全部一致。

本次只切换 Sub2API 主站数据库和运行配置，未停止、重建或修改 11 个卫星、AgentAPI、Cost Console 或仓库外容器。导入后核对：182 个用户、747 个账号、143 个 API Key、340,914 条用量记录、295 条迁移记录；其中未软删除的用户为 180 个、账号为 245 个。支付订单状态数量为 PENDING 2、COMPLETED 43、EXPIRED 50、CANCELLED 103、FAILED 31。当前入口为 `http://localhost:18080`，健康接口返回 200。

启动时成功补齐 `246_agent_shared_tenants.sql` 和 `247_retire_agent_provisioning.sql` 两项迁移：新增两张共享租户表、移除六张源库为空的旧代理控制表，因此当前为 102 张表。源迁移 checksum 经校验一致；本次临时 Dockerfile 仅规范构建副本中 SQL 的 LF 换行，避免 Windows CRLF 改变线上 checksum，没有修改源 SQL 文件。

为防止线上数据在本地启动时产生外部副作用，本地配置关闭了 OAuth 自动刷新、支付订单后台对账/过期任务、渠道主动监控，并将 `payment_enabled` 置为 false、SMTP 主机置空；历史支付供应商、订单、账号和余额数据仍保留。新增配置 `PAYMENT_ORDER_EXPIRY_ENABLED` 默认值为 true，本次本地值为 false；该开关只关闭后台任务，手动支付验证、取消和退款接口仍可能访问恢复的供应商，验收时未调用这些接口。

配套 `TOTP_ENCRYPTION_KEY` 与数据库中的 JWT 密钥已通过非输出式校验；监控密文可解密。恢复后的 JWT 使用数据库持久密钥，本地旧会话需要重新登录。Redis 改用 DB 1，原 DB 0 未清空并保留作回滚；旧数据库重命名为 `sub2api_local_before_20261008_140108`，未删除数据。

回滚和证据均在 Git 忽略目录 `.local/sub2api-import-20261008-140108/`，包括切换前 PostgreSQL dump、Redis RDB、容器快照、实际 Compose、源 TOC、逐表计数和构建日志。不要提交该目录或输出其中的密钥。该目录的 `rollback.ps1` 先备份当前库并保留其数据库名，再切回旧库、原 Compose 和环境，恢复使用 Redis DB 0；脚本未在验收中执行。不要执行 `docker compose down -v`。当前主站 Compose 为该目录的 `deploy-sub2api.compose.json`，保留原端口、网络、挂载和 SSO 地址。

验收已通过健康接口、公开设置、恢复管理员短时签名鉴权、用户/账号只读列表及 11 个卫星的 SSO 启动地址（均为 localhost）。未尝试账号密码登录、卫星端回调/余额全流程、收费生成、支付或 SMTP。配置默认值/环境覆盖与支付后台启停的相关 Go 测试通过。源 Linux 的 Redis、上传文件和备份之后的新数据不在此归档中，本次不会凭数据库恢复这些内容。

## 启动和检查

在仓库根目录执行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/Start-LocalDocker.ps1
docker ps --format "{{.Names}}\t{{.Status}}\t{{.Ports}}"
docker logs --tail 50 sub2api
```

脚本用于启动本机现有部署，目标为 Sub2API、11 个卫星和 AgentAPI 代理站，共 13 个首页、16 个应用容器、6 个依赖容器。Cost Console 及其 PostgreSQL / Redis 已排除，不要求这些参考实例存在。局部范围应只操作对应目标，不能直接运行此完整启动脚本。脚本不会删除或重建容器，不适用于尚无镜像和容器的新机器，也不能代替源码覆盖部署。现有容器的重启策略未因本次脚本调整而改变；排除参考实例不等于停止它们。

## 本次修复

- Sub2API Redis 的 AOF 尾部损坏：先完整备份，再由 `redis-check-aof --fix` 截去 48 字节无效尾部，重新启动后通过健康检查。
- Canvas 本地 Compose 覆盖中的公开 `LINK` 改为 `localhost:18080`；内部模型调用仍为 `http://sub2api:8080/v1`。
- PPT `/api/v1/` 反向代理保留 Host 端口，解决 localhost:8341 的同源 SSO exchange 被误判为 403。
- Yibiao 余额接口补齐 `JSONResponse`、`relay_base_url`、`satellite_headers` 导入，解决登录后 500。
- PPT 与 Yibiao 使用已有本机镜像构建增量修复镜像并重建对应容器，原镜像保留为 `ppt:before-deployment-fix`、`yibiao:before-deployment-fix`。源码同步修复，常规 Dockerfile 后续完整构建也包含修复。

## 配置与数据

共享 Docker 网络为 `deploy_sub2api-network`。SSO 密钥和项目凭据沿用现有运行配置，未生成新的 SSO 协议或共享用户 Key。不要输出或提交真实环境配置。

本次运行快照、修复前 Redis 备份、临时 Compose 环境覆盖和详细验收结果在已被 Git 忽略的 `.local/docker-deployment/`。该目录含敏感配置，应只保留在本机。数据库和上传目录继续使用原有卷 / bind mount；不要执行 `docker compose down -v`。

PPT 和 Yibiao 本次重建命令（仓库根目录）：

```powershell
docker compose -p yibiao --env-file sub2api/deploy/.env -f yibiao/docker-compose.yml -f .local/docker-deployment/yibiao.override.json up -d --no-build
docker compose -p ppt --env-file sub2api/deploy/.env -f ppt/docker-compose.yml -f ppt/docker-compose.sub2api.yml -f .local/docker-deployment/ppt-production-1.override.json up -d --no-build --no-deps production
```

## 验收范围

12 个首页均返回 200；Sub2API 健康检查通过。11 个卫星逐一通过 Sub2API SSO 启动、回调 / 必要交换、登录后余额查询；匿名余额请求均返回 401，余额响应均带 `no-store`，充值地址均指向本机 Sub2API。重复使用 SSO 票据均未签发新 Cookie。SuperKey 作为普通 API Key 返回 403，卫星凭据缺少用户身份返回 401。

此次是容器部署与接入连通性验收，未执行收费的文本 / 图片 / 视频生成，也未覆盖每个应用的全部业务功能。Canvas 与 Livart 现有代码仍使用本应用本地 token 的旧会话方式；本次验证通过其现有客户端流程，不代表已完成公共规范要求的 HttpOnly 会话改造。它们未向浏览器传递 Sub2API SuperKey。

## Canvas 文本模型切换为 gpt-6

本次仅更新 Canvas：主站 0 个、卫星 1 个、代理站 0 个、应用容器 1 个、依赖容器 0 个。Sub2API、其他卫星、AgentAPI、参考 Cost Console 及仓库外容器均排除。

Canvas 前端默认文本模型、服务端 Relay 兜底、示例环境变量与 Compose 配置统一为 `gpt-6`。在 `sub2api/deploy` 只构建 `canvas` 服务，并以 `up -d --no-deps --no-build --pull never canvas` 更新 `canvas-app`；当前镜像为 `canvas:workspace`，镜像 ID 为 `sha256:f6ae3ff7f4c4fe230d373d153f0318577f404d938926894c52fe73b46cc3f800`。首次构建遇到 Go 依赖下载 EOF，重试成功。保留 `canvas_integrated_data` 数据卷以及原有 Session、SSO 和 Relay 凭据，Sub2API 容器未重建。

验收：3522 首页和健康接口返回 200，公开设置的默认文本模型及唯一托管文本模型均为 `gpt-6`；匿名余额返回 401 并带 `Cache-Control: no-store`。图片与视频默认模型未变化。未执行登录后的 SSO、余额或收费模型调用，不能据此认定此前上游错误映射已修复。变更前镜像、运行配置、数据库副本、构建日志和验收结果保存在 Git 忽略目录 `.local/canvas-gpt6-20261008-135137/`。

## 2026-10-08 AI表格局部修复与验收

本次仅覆盖部署 `aiexcel/`：0 个主站、1 个卫星、0 个代理站，共 1 个应用容器、0 个依赖容器。其余卫星、AgentAPI、Cost Console 参考项目及仓库外容器排除。复用同期 Livart 任务已部署的 Sub2API 404 故障切换修复，本任务未重建主站。沿用原端口 4173、网络、SSO 密钥和应用凭据，应用无持久化挂载；运行环境的业务配置仅修改 `SUB2API_RELAY_MODEL=gpt-6`。

已删除对话面板的“DeepSeek 已连接”，修正连接设置说明。AIExcel 使用文本模型生成严格 JSON 操作计划，由浏览器确认后执行数据修改。默认模型、Compose 示例和公开模型目录同步为 `gpt-6`，表格规划使用低推理档；补齐无协议 LINK 规范化、非 JSON 上游错误处理，并修复重复请求提前释放原请求锁的问题。

当前镜像为 `local-deploy/aiexcel-aiexcel:20261008-134116`，容器健康。构建、10 项页面/路由测试、路由 ESLint 与 diff 格式检查通过。SSO 回调及交换、HttpOnly 3 天 Session、登录后身份与余额查询、余额 no-store、匿名 AI 401 和票据重放拒绝通过。全项目 TypeScript 检查仍有既有 ExcelJS 列 key 类型及 Cloudflare 类型声明错误，未将其算作通过。

真实 AI 链路未完全验收通过。初始 `gpt-6` 依次遇到上游无渠道、404 与上游余额不足；实际 `gpt-6-astra` 曾在账号 55 成功返回（100.6 秒）。已通过现有管理 API 仅修正该账号的 `gpt-6 → gpt-6-astra` 映射，保留凭据、地址、优先级和其他映射。一次同期全量模型刷新覆盖该别名后重新修复，并恢复该已验证账号的可调度状态。完整表格计划首次遇到上游 524；低推理档最终请求在新模型候选中耗尽 10 次切换并返回 502，未到达账号 55。剩余问题在主站模型能力目录与调度候选，未为此改动其他账号优先级或禁用状态。未执行用户真实工作簿修改/导出或全量业务回归。

部署前镜像、私密 Compose 与账号配置快照、脱敏验收记录保存在 Git 忽略目录 `.local/aiexcel-fix-20261008-134116/`，回滚镜像为 `local-rollback/aiexcel-aiexcel:20261008-134116`。该目录含私密配置，不得公开或提交。

## 2026-10-08 AI表格默认模型直接切换为 gpt-6-astra

按用户后续要求，仅更新 AIExcel 默认文本模型为 `gpt-6-astra`，直接发送该公开模型名，保留低推理档。服务端兜底、本地与线上 Compose 默认值、已有路由测试和当前文档同步更新。本次为 0 个主站、1 个卫星、0 个代理站、1 个应用容器、0 个依赖容器；其余卫星、AgentAPI、参考 Cost Console 及仓库外容器排除，未修改主站账号、映射或调度策略。

本地镜像 `local-deploy/aiexcel-astra:20261008-151416` 已构建并覆盖部署，容器健康，运行环境确认 `SUB2API_RELAY_MODEL=gpt-6-astra`。仅模型环境值变化，原端口 4173、网络、SSO 与 Relay 凭据保留。10 项页面/路由测试通过；首页 200、匿名身份/余额/AI 请求 401、SSO 回调 307 与交换 200、HttpOnly 3 天 Session、登录后身份和余额 200、余额 no-store 验收通过。未执行收费模型生成，因此本次不据此认定之前的上游可用性问题已解决。

部署前配置快照和本次私密 Compose 在 Git 忽略目录 `.local/aiexcel-astra-20261008-151416/`，回滚镜像为 `local-rollback/aiexcel-astra:20261008-151416`。私密配置不得公开或提交。

## 2026-10-08 AI表格按要求再次覆盖部署 Docker

本次仅覆盖 AIExcel：0 个主站、1 个卫星、0 个代理站、1 个应用容器、0 个依赖容器。主站、其余卫星、AgentAPI、参考 Cost Console 及仓库外容器排除。基于当前源码执行 Docker build（源码未变，命中此前已通过 10 项测试的构建缓存），再使用 `--force-recreate --no-deps --no-build --pull never` 重建 `aiexcel`。

当前镜像 `local-deploy/aiexcel-docker:20261008-151951`，容器运行且健康，默认模型仍为 `gpt-6-astra`。全部运行环境值、端口 4173、网络与凭据保持一致。首页 200、SSO 回调 307/交换 200、HttpOnly 3 天 Session、登录后身份/余额 200、余额 no-store、匿名身份/余额/AI 请求 401 检查通过。未执行收费生成或工作簿业务回归，上游生成可用性不计作通过。

本次私密 Compose 与部署前容器快照位于 Git 忽略目录 `.local/aiexcel-docker-20261008-151951/`，回滚镜像 `local-rollback/aiexcel-docker:20261008-151951`；不得公开私密配置。

## 2026-10-08 Screen2Code 默认模型 gpt-6-astra 覆盖部署

本次仅更新 Screen2Code：主站 0 个、卫星 1 个、代理站 0 个，重建应用容器 2 个、依赖容器 0 个。现有 Sub2API 仅检查健康；其他卫星、AgentAPI、参考 Cost Console 及仓库外容器均排除。

前后端最新源码已构建并覆盖到 `screen2code-backend-1`、`screen2code-frontend-1`，镜像为 `local-deploy/screen2code-backend:astra-20261008-144402` 与 `local-deploy/screen2code-frontend:astra-20261008-144402`。依赖清单与原容器一致，构建复用原有依赖层并复制当前源码。默认模型 API 名为 `gpt-6-astra`，保留 high / low reasoning，前端默认 high；图片理解配置和本地计费项同步。端口仍为 5173 / 7001，原环境变量、SSO 与服务端凭据、网络和 `/app/data` 挂载经比较保持一致。

验收通过：两个容器运行正常，前后端首页 200，前端实际服务源码包含新模型，运行中后端默认候选均为 `gpt-6-astra`；匿名身份接口 200、匿名余额 401 且带 `Cache-Control: no-store`、无效 SSO 票据 400，现有 Sub2API 健康接口 200。登录数据库在线备份及运行后只读完整性检查均为 ok。此前源码校验：后端 294 项、前端 42 项测试通过，前端构建通过，类型检查无新增诊断；全量 lint 存在既有问题，修改文件检查通过。

旧镜像、容器运行配置、私密 Compose、数据库备份、构建日志和验收记录保存在 Git 忽略目录 `.local/screen2code-astra-20261008-144402/`；该目录含私密配置，不得提交。`rollback.compose.json` 可只恢复两个应用镜像，继续使用现有数据挂载。未执行登录后的 SSO/余额全流程、收费生成或视频路径验证；模型切换不代表支持原始视频输入。

## 2026-10-08 Livart 发送按钮修复与图片编辑 502 排查

此前本任务局部覆盖部署了 Sub2API 和 Livart：主站 1 个、卫星 1 个、代理站 0 个，涉及应用容器 2 个、依赖容器 0 个。其他卫星、AgentAPI、参考 Cost Console 及仓库外容器排除；原有凭据、端口、网络和数据保留。当前镜像分别为 `local-deploy/deploy-sub2api:livart-fix-20261008-143037` 和 `local-deploy/livart-livart:livart-fix-20261008-144648`，两个容器健康。本次追加排查没有重建或重启容器，也没有修改账号配置。

Livart 发送按钮固定为 40px 点击区域，移除悬停/按下时的缩放，图标不参与鼠标事件；发送与 Ctrl+Enter 复用提交逻辑，提交后清空草稿。最新部署的真实页面验证通过：桌面鼠标、移动触控各完成中心及 10 个边缘位置点击，均只提交一次并清空输入框，Ctrl+Enter 通过，无页面错误。提交接口由测试脚本模拟，验证的是按钮交互，不代表上游生成成功。证据为 `.local/livart-send-desktop-verification.json` 和 `.local/livart-send-mobile-verification.json`；最新 Livart 构建、部署和回滚资料在 `.local/livart-fix-20261008-144648/`。

图片编辑失败 requestId 为 `eaaceea5-fd4e-4d89-b5e8-4db225479f22`。Sub2API 日志及 `ops_error_logs.upstream_errors` 确认：`POST /v1/images/edits`、模型 `gpt-image-2`、multipart、high、2048x2048，实际结束时间 14:46:46，耗时 62,921ms。共尝试 11 个不同账号并对两个 403 账号各额外重试 3 次，达到 10 次账号切换上限后，由主站统一返回 502 / upstream_error / Upstream request failed。不能将这个统一 502 当成某一个上游的原始状态。

| 账号 ID | 上游状态 | 持久化错误原因 |
| --- | --- | --- |
| 737、735、736 | 503 | 对应 GLM、DeepSeek、Kimi 路由没有 gpt-image-2，返回的路由模型列表仅包含文本模型 |
| 4、34 | 503 | 对应 Claude / 视频分组没有可用的 gpt-image-2 渠道 |
| 65 | 403 | API Key 所属分组已删除 |
| 30 | 403 | Insufficient account balance |
| 29 | 405 | 上游不接受该端点/方法，错误消息为空 |
| 738、741 | 502 | Upstream access forbidden, please contact administrator |
| 699 | 404 | Model "gpt-image-2" is not available for this group |

上述账号仍在主站 `model_mapping` 中显式声明 `gpt-image-2 → gpt-image-2`，与部分上游实际可调用能力不一致，使不可用账号进入图片候选。代码确认 404、405、502、503 均进入账号切换；这次没有证据表明 Livart 点击、multipart 参数或缺少故障切换导致错误。主站账号 48 在 14:41:52 的 `/v1/images/generations` 请求成功返回 200 并生成一张图片，但未进入本次编辑的 11 个尝试；生图成功不能证明该账号支持编辑，也不能据本次失败认定全部候选账号均不可用。

后续恢复重点是核对账号图片能力目录与其 Key 分组实际权限，清理错误的图片能力声明，修复已删除分组、余额及上游访问限制，并对有真实图片权限的账号验证 `/v1/images/edits`。本次未改账号凭据、余额、优先级、地址或模型映射，未额外发起收费图片请求。失败前的 Livart 容器已被前次部署替换，旧进程 image-proxy 日志不在当前容器中；本次诊断依据主站保留的完整尝试链路。发送按钮已验证通过，图片编辑恢复尚未验证。

## 2026-10-08 Livart 失败提示中文化

本次仅覆盖部署 Livart：主站 0 个、卫星 1 个、代理站 0 个，重建应用容器 1 个、依赖容器 0 个。Sub2API、其他卫星、AgentAPI、参考 Cost Console 及仓库外容器排除。镜像为 `local-deploy/livart-livart:livart-fix-20261008-152207`，原环境、SSO/Session 配置、端口、网络和数据保留；容器 healthy，`/api/health` 返回 200。部署前运行配置、回滚镜像标识、Livart PostgreSQL 备份及构建日志位于 Git 忽略目录 `.local/livart-fix-20261008-152207/`。

新增统一失败说明组件和错误分类，覆盖右侧对话（包含历史错误）、图片任务、画布局部编辑/图层拆分、上传/导出、登录/注册、连接设置、外部图片、产品分析和扩展工具加载等失败入口。原因使用中文，处理建议使用常规文字颜色，技术状态码/代码/请求编号默认折叠在“查看排查详情”中，并提供复制。API 请求保留状态和代码用于分类，未知错误不把英文异常或 JSON 作为默认解释；排查详情隐藏密钥、凭据和服务地址。

明确区分用户余额不足与上游通道额度不足；统一 502 提示只列可能原因并说明无法确认具体原因，不把上游额度问题错误归因于用户余额，也不建议用户无依据充值。对 404、权限失效、通道删除、模型不可用、接口不支持、限流、超时、网络中断、文件过大、任务过期和内容限制等错误提供对应处理建议。未修改上游账号配置，未宣称图片编辑服务已恢复。

验证：32 项错误文案/分类/脱敏测试通过，Vite 生产构建通过。桌面 1440×900 和移动 390×844 的实际部署页面回归通过：历史 502、新通道余额错误中文化，详情展开/折叠和复制请求编号成功，发送后清空输入框，无页面错误和横向溢出；移动登录错误验证通过。页面验证模拟失败接口，没有发起收费 AI 请求。证据和截图为 `.local/livart-errors-desktop.json`、`.local/livart-errors-mobile.json` 及对应 PNG。TypeScript 全量检查仍报告既有的 `activeTaskTimers` Object.values 推断和 PanoramaViewer CSS 模块声明共 3 条诊断，本次新增模块无诊断；不能将全量类型检查记为通过。

## 2026-10-08 Canvas 默认文本模型 gpt-6-astra 覆盖部署

本次仅更新 Canvas：主站 0 个、卫星 1 个、代理站 0 个，重建应用容器 1 个、依赖容器 0 个。Sub2API、其他卫星、AgentAPI、参考 Cost Console 及仓库外容器均排除。

前端默认值、服务端 Relay 兜底、示例配置和实际 Compose 覆盖文件统一为 `gpt-6-astra`。仅构建并重建 Compose 服务 `canvas`，容器为 `canvas-app`，镜像为 `canvas:workspace`（`sha256:51f243a01c0546d14f08e7384bec8c262b65cf5ac015b201d6f85f54518f53ce`）。与部署前比较，仅 `SUB2API_RELAY_MODELS` 环境变量变化，服务端凭据、SSO 密钥、端口 3522、数据卷 `canvas_integrated_data:/app/data` 和网络保持一致；Sub2API 的容器 ID、镜像和启动时间均未变化。

验收通过：镜像构建成功，Canvas 首页与健康接口 200，公开设置的 `defaultTextModel` 和 `defaultModel` 均为 `gpt-6-astra`，托管模型目录移除旧 `gpt-6`；默认图片仍为 `gpt-image-2`，默认视频仍为 `grok-imagine-video-1.5`。匿名身份接口 200、匿名余额 401 且带 `Cache-Control: no-store`，Sub2API 健康接口 200。已检查 TODO，此次没有可移除的独立待办，真实创作验收保留在待测试文档。

构建日志、容器配置快照、数据库副本与脱敏验收记录保存在 Git 忽略目录 `.local/canvas-astra-20261008-151800/`，其中私密配置不得公开或提交。旧镜像保留为 `local-rollback/canvas:astra-20261008-151800`，`rollback.compose.json` 可覆盖 Canvas 镜像回退并继续使用现有数据卷。未发起收费模型请求，未验证登录后的创作 Agent 回复；模型切换不代表此前上游 404 或候选账号能力声明问题已经解决。


## 2026-10-08 Screen2Code 全面汉化与录屏输入修复

本次仅覆盖 Screen2Code：主站 0 个、卫星 1 个、代理站 0 个，更新应用容器 2 个、依赖容器 0 个。Sub2API 仅检查健康返回 200，不重建；其他卫星、AgentAPI、参考 Cost Console 与仓库外容器排除。前后端镜像分别为 `local-deploy/screen2code-backend:zh-20261008-162126`、`local-deploy/screen2code-frontend:zh-20261008-162126`。原环境、凭据、SSO、3 天 Session、端口、网络及 `/app/data` 挂载保留，数据库身份与会话数量和备份一致。部署快照、SQLite 一致性备份、旧镜像标签及回滚 Compose 保存在 Git 忽略目录 `.local/screen2code-zh-20261008-162126/`。

汉化覆盖用户界面、主题显示值、上传/录屏/网址/文本/代码导入、设置、设计规范、历史版本、预览、生成进度、错误提示、评测页和模型提示词/工具说明。API 字段、模型 ID、工具名及代码语法保留；默认模型继续为 `gpt-6-astra`。新生成页面和 AI 回复默认简体中文，复刻截图保留原图文字。匿名设计规范 401 不再误报加载失败，登录请求继续携带会话 Cookie。

修复原始 `data:video/...` 被发送至图片字段造成的 400：视频先通过 FFmpeg 转为按时间排序的 JPEG 帧，每段最多 16 帧，附中文连续画面说明；历史视频也转换，并在 OpenAI 转换边界拒绝原始视频 MIME。视频解析错误提供中文提示；明确声明 `imageio-ffmpeg` 依赖。

验证：后端 299 项测试通过，包括真实 WebM 抽帧和 Responses 图片输入结构；pyright 0 错误、33 个基线警告，新增诊断 0。前端 45 项测试通过，6 项收费端到端测试跳过；TypeScript 编译通过，生产构建通过，新增调整文件 ESLint 通过。全量 ESLint 仍为原有 19 个错误、6 个警告。首页 200、身份接口 200、匿名余额 401 且 `Cache-Control: no-store`、无效 SSO 票据 400、匿名设计规范 401；运行容器日志无启动异常，前端显示 TypeScript 0 错误。浏览器已检查上传、文字、网址、导入和设置页面汉化及首页无设计规范误报 Toast。未发起真实收费生成请求，尚未验证模型对用户录屏的最终复刻质量。

### 2026-10-08 17:05 Screen2Code 中文显示补充部署

范围：主站 0、卫星 1（screen2code）、代理站 0；仅重建前端应用容器 1，依赖容器 0。后端容器标识与此前部署记录一致。本次排除 Sub2API、其他 10 个卫星、AgentAPI 和参考 Cost Console。

前端镜像：`local-deploy/screen2code-frontend:zh-20261008-20261008-170324`，运行容器 `screen2code-frontend-1`。配置及回滚依据保存在 `.local/screen2code-zh-followup-20261008-170324/`。保留原环境、网络及端口；没有修改认证数据库。

补充中文状态/工具/任务兜底、中文容量单位、方案编号、模型推理档显示和服务商显示；修复评测对比页被误译的会话查询参数，仍使用协议规定的 `session`。模型 ID、品牌、技术栈名和原始请求字段保持有效协议格式。

验证：前端 45 项测试通过（6 项收费端到端测试跳过）、生产构建通过、此次修改的 4 个文件 ESLint 通过；全量 ESLint 仍有原基线 19 个错误、6 个警告。后端在无生产环境凭据/无数据挂载的临时容器中使用 Poetry 虚拟环境验收，299 项测试通过，pyright 0 错误、33 个既有警告。首页及身份接口 200，匿名余额 401 且 no-store，匿名设计规范 401；运行前端日志 TypeScript 0 错误，实际提供的脚本包含中文兜底和单位。未执行真实收费生成和完整登录评测交互。

## 2026-10-08 AI 剪辑默认文本模型 gpt-6-astra

本次仅更新 AI 剪辑 `aicut`：主站 0 个、卫星 1 个、代理站 0 个，重建应用容器 1 个、依赖容器 0 个。Sub2API、其余 10 个卫星、AgentAPI、参考 Cost Console 和仓库外容器均排除。

前后端共用的文本默认提供商改为 OpenAI，默认公开模型名改为 `gpt-6-astra`；同步示例环境、本地与在线 Compose 的 aicut 配置、使用指南和公开模型目录。未改其他应用的默认模型或图片、视频模型配置。

运行镜像为 `local-deploy/aicut:astra-20261008170943`，容器 `aicut` 已重建并运行。保留原环境中的凭据与 SSO/Session 配置、端口、网络、`aicut_data` 挂载及原 `.env.local` 文件；依赖清单哈希与原 Linux 镜像一致，因此复用现有依赖后构建当前源码。配置快照、原设置文件、回滚 Compose、回滚镜像依据和构建日志保存在 Git 忽略目录 `.local/aicut-astra-20261008170943/`。

验证：`ai-sdk.verify.ts`、`local-models.verify.ts`、`llm-proxy.verify.ts` 三组校验通过；运行容器内文本默认值和环境均为 OpenAI / `gpt-6-astra`，浏览器实际加载的共用模型模块包含新默认值。首页及模型模块 200，匿名身份和余额接口 401，余额带 `Cache-Control: no-store`，无效 SSO 票据 401，匿名 LLM 请求被拒绝（403）。原配置、挂载、端口和网络保持一致，原设置文件哈希校验通过。未执行真实登录 SSO 或收费模型生成，未验证模型最终剪辑效果。

扩展检查未全通过：`tsc -b --noEmit` 在本次未修改的 LoginGate、SessionBar、template-host、Vite 配置及 tenant 文件报告既有类型诊断；`executor.verify.ts` 在默认模型检查之前因 `browse_library` 工具 schema 非 canonical 失败。本次没有修改这些模块。具体运行验收结果见上述私密目录的 `runtime-verification.json`。


## 2026-10-08 AI 获客默认模型 gpt-6-astra 覆盖部署

本次仅覆盖 AI 获客：主站 0 个、卫星 1 个（aihuoke）、代理站 0 个，重建应用容器 2 个、依赖容器 0 个。Sub2API、其他 10 个卫星、AgentAPI、参考 Cost Console 及仓库外容器均排除。

前后端当前源码已构建并覆盖部署，镜像为 `local-deploy/aihuoke-backend:astra-20261008-172025`、`local-deploy/aihuoke-frontend:astra-20261008-172025`。后端沿用已有经验证的依赖复用构建文件，requirements.txt SHA-256 校验及 pip check 通过。运行环境 `SUB2API_RELAY_MODEL` 从 `gpt-5.5` 切换为 `gpt-6-astra`，文本与 Agent 推理默认值、设置页显示保持一致。原端口 3001、网络、SSO/服务端凭据、启动命令和 `aihuoke_data:/app/data` 挂载经部署前后比较保留。

验收通过：两个应用容器重新创建、运行镜像匹配；首页与后端健康接口 200；运行容器模型解析为 `gpt-6-astra`；SSO 启动 200、回调 303、HttpOnly Session 259200 秒（3 天）、登录后余额 200 且 no-store；匿名余额 401，票据重放不签发 Cookie 且不能读取余额。此前模型修改相关测试 63 项通过。未发起收费模型生成，未执行实际获客或邮件发送业务验证。

4 个 SQLite 数据库已完成部署前在线备份并通过完整性检查。运行配置快照、私密 Compose、数据库备份、构建日志、回滚配置和验收结果保存在 Git 忽略目录 `.local/aihuoke-astra-20261008-172025/`，不得公开或提交私密配置。旧镜像保留为 `local-rollback/aihuoke-backend:astra-20261008-172025`、`local-rollback/aihuoke-frontend:astra-20261008-172025`，可使用该目录 `rollback.compose.json` 恢复两个应用。
# 2026-10-09 Sub2API 调度修复远程灰度记录

本次仅更新Sub2API主站调度（1个主站、0个卫星、0个代理站），保留PostgreSQL/Redis依赖不重建。已有生产基线为20261009；新增网关候选容器`sub2api-scheduling-20261009`和对应审计代理，旧实例保持运行。真实公共入口两域两模型共16项Chat/Responses流式及非流式验证通过，数据库迁移清单295条不变。参考Cost Console排除，未操作其他应用。源码与验证边界详见[调度可靠性修复记录](Sub2API调度可靠性修复-20261009.md)。


## 2026-10-23 PPT 数据卷挂载损坏修复

范围：主站 0 个、卫星 1 个（ppt）、代理站 0 个，重建应用容器 1 个、依赖容器 0 个。Sub2API、其他 10 个卫星、AgentAPI、参考 Cost Console 及仓库外容器均排除。

故障：容器 `ppt-production-1` 的数据卷绑定源 `C:\Users\kodesh\Desktop\CC2CX\ppt\app_data` 已被删除，`/app_data` 变成损坏的 9p 挂载（`d?????????`，`ls`/`stat` 均失败）。`APP_DATA_DIRECTORY=/app_data` 读不到 `userConfig.json` 与 `fastapi.db`，导致 `/api/v1/auth/status`、`/api/v1/ppt/presentation/all`、`/api/v1/ppt/theme`、`/api/v1/ppt/template/all` 全部返回 500，日志为 `FileExistsError: [Errno 17] File exists: '/app_data'`。首页 `/` 仍为 200，故障仅体现在 API 层。

数据核对：真实数据保存在 `C:\Users\kodesh\Desktop\cc2cx_sub2api\ppt\app_data`（229 个文件、31.0 MB），`fastapi.db` 4,493,312 字节，SHA-256 `6B62E43A055F71214736FCCF9807AF78F8244D0341B2588EAC2505826356756A`，`userConfig.json` 时间戳与该容器创建时间一致，确认即为原运行数据。部署前已整目录备份至 Git 忽略目录 `.local/ppt-restore-20261023-194632/`（含 `app_data/`、`fastapi.db`、`userConfig.json`、`env.sub2api.bak`）。

修复方式：把 `.env.sub2api` 复制到工作区 `ppt/`（已被 `.gitignore` 忽略），并在 `ppt/` 下建立指向真实数据目录的目录联接 `ppt/app_data`，使默认 `docker-compose.yml` 的相对路径 `./app_data:/app_data` 直接解析到正确位置。随后用 `docker compose --env-file .env.sub2api -f docker-compose.yml -f docker-compose.sub2api.yml up -d --no-deps --force-recreate --no-build production` 重建容器，镜像沿用已验证的 `local-deploy/ppt-production:20261009-175351`，不重新构建。未删除任何卷，未覆盖数据库。

后续变化（同日 20:28 全量覆盖部署）：用户另行执行了全量覆盖部署，`ppt-production-1` 已改用 `local-deploy/ppt-production:modelplaza-platinum-20261023`，挂载源同时变为 `C:\Users\kodesh\Desktop\CC2CX\ppt\app_data`。该次部署新建了空数据库，本节所述"修复完成"状态随之失效，数据恢复见下一节。

验收通过：容器 `ppt-production-1` running、`RestartCount=0`，挂载解析为 `C:\Users\kodesh\Desktop\Workspace\sub2api\ppt\app_data -> /app_data`；`/` 与 `/upload` 200，`/api/v1/auth/status` 200 且 `configured=true`，受保护接口由 500 恢复为 401；`/api/v1/auth/sso/callback` 无效票据 303 跳 `/?error=SSO_failed`，`/api/v1/auth/sso/exchange` 无 Cookie 401（`Cache-Control: no-store`、`Referrer-Policy: no-referrer`）、缺少 `X-Presenton-SSO` 头 403；容器内查询数据库为 9 个用户（1 管理员 + 8 个 SSO 用户）、16 个演示文稿、13 张幻灯片、8 个模板，与修复前一致；`userConfig.json` 十个字段修复前后逐项比对完全相同；容器内经 Docker DNS 访问 `http://sub2api:8080/health` 返回 200；近 5 分钟日志无异常。会话时长仍为 `SESSION_TTL_SECONDS = 60*60*24*3`（259200 秒，3 天）。

未执行：未在浏览器中完成一次真实 SSO 登录并生成演示文稿（需真实主站账号与上游额度），未重新构建镜像，未修改任何源码。

## 2026-10-23 模型广场分组改造 + 白金主页迁移后全量覆盖本地 Docker

范围：主站 1 个（Sub2API）、卫星 11 个（aicut、aiexcel、ai3d、aihuoke、canvas、ju、livart、ppt、qrcode、screen2code、yibiao）、代理站 1 个（AgentAPI），共 13 个项目，重建应用容器 15 个、依赖容器 0 个。参考 Cost Console 及所有 `crm-*`、`cs-activation-*`、`xianyu-*` 仓库外容器均排除；`sub2api-postgres`、`sub2api-redis`、`ju-sso-redis-1`、`livart-postgres`、`livart-rabbitmq-1`、`livart-minio-1` 六个依赖容器未重建。

源码基线为工作区 `C:\Users\kodesh\Desktop\Workspace\sub2api`（分支 `talent`，HEAD `61aa73ee`）。本次两个功能改造：模型广场改为按分组展示全部当前中转站支持的模型（厂家 → 功能 → 分组 → 倍率四维联动过滤），AgentAPI 代理站主页换成白金粒子效果版。其余功能、定制化配置与业务代码未改动。

镜像统一使用 `modelplaza-platinum-20261023` 标签：`local-deploy/deploy-sub2api`、`local-deploy/deploy-canvas`、`local-deploy/aicut-aicut`、`local-deploy/aiexcel-aiexcel`、`local-deploy/ai3d-ai3d`、`local-deploy/ai-hunter-root-aihuoke-backend`、`local-deploy/ai-hunter-root-aihuoke-frontend`、`local-deploy/ju-web`、`local-deploy/ju-backend`、`local-deploy/livart-livart`、`local-deploy/ppt-production`、`local-deploy/qrcode-qrcode`、`local-deploy/screen2code-backend`、`local-deploy/screen2code-frontend`、`local-deploy/yibiao-yibiao`、`local-deploy/agentapi`。

模型广场验收：`GET /api/v1/model-plaza` 匿名可访问，返回 25 个分组、732 个模型、0 个空分组；厂家维度为 `[OI]` 418、`Anthropic` 198、`Grok` 77、`智谱 GLM` 31、`DeepSeek` 3、`MiniMax` 2、`字节豆包` 2、`Kimi` 1；功能维度为文本 636、图片 83、视频 13。此前 `Zhipu GLM` 与 `智谱 GLM` 双标签并存的问题已修正为单一 `智谱 GLM`。

配置保持：重建前对 16 个容器逐项比对 live 环境变量、挂载与端口和 Compose 解析结果，全部一致（`sub2api` 117 项、`ppt-production-1` 127 项、`livart-livart-1` 51 项、`ju-backend-1` 24 项、`aihuoke-backend` 23 项、`ai3d` 20 项、`canvas-app` 17 项、`aicut`/`yibiao` 各 16 项、`qrcode` 15 项、`screen2code-backend-1` 14 项、`aiexcel` 9 项、`ju-web-1` 6 项、`aihuoke-frontend`/`screen2code-frontend-1` 各 5 项、`agentapi-agentapi-1` 31 项）。部署后再比对一次，仍全部一致；端口映射、数据卷、网络、SSO 与服务端凭据、启动命令均未改变。

过程中修正的两处构建问题（均非业务代码缺陷）：

1. `livart/docker/entrypoint.sh` 在 Windows 上被 `core.autocrlf=true` 检出为 CRLF，shebang 变成 `#!/bin/sh\r`，容器报 `exec /app/entrypoint.sh: no such file or directory` 并反复重启。仓库内 blob 本身是 LF，将该文件按 LF 重新检出后重建即恢复，`git status` 干净。`canvas/docker-entrypoint.sh` 有 `eol=lf` 属性保护，未受影响。
2. `ppt` 运行阶段需从 `deb.debian.org` 拉取约 302 MB 的 Noto 字体包，构建沙箱内连续两次超时失败。改为在宿主机预取同一批 Debian 包（8 个 `.deb`，`!<arch>` 校验通过），构建时以 BuildKit 命名上下文 `fontdebs` 挂载后用 `dpkg -i` 安装；`apt-get install ./*.deb` 会重新解析包名走网络，故未采用。字体包内容与原 Dockerfile 一致，其余指令逐行照搬。

验收通过：15 个应用容器全部以新镜像运行，`sub2api`、`aiexcel`、`ju-web-1`、`ju-backend-1`、`livart-livart-1` 健康检查为 healthy；`sub2api` 主页与模型广场、`canvas`、`aicut`、`aiexcel`、`ai3d`、`aihuoke` 前端、`ju` web、`livart` `/api/health`、`ppt`、`qrcode`、`screen2code` 前后端、`yibiao`、`agentapi` 均返回 200。`aihuoke-backend` 不映射宿主端口，容器内请求 `/docs` 返回 401 属其自身鉴权行为，启动日志显示数据加载与请求处理正常。`ppt` 启动日志显示默认模板导入完成、`Application startup complete`。

未执行：未在浏览器中完成真实 SSO 登录与业务操作（需真实主站账号与上游额度），未发起收费模型生成，未改动数据库数据。构建日志、Compose 快照、环境变量与挂载比对脚本、字体包预取目录保存在 Git 忽略目录 `.local/modelplaza-platinum-20261023/`。回滚镜像为 `local-rollback/<容器名>:modelplaza-platinum-20261023`，共 15 个。

## 2026-10-24 PPT 数据恢复至 modelplaza-platinum 镜像 + 真实端到端验收

范围：主站 0 个、卫星 1 个（ppt）、代理站 0 个，重建应用容器 0 个（停止/启动同一容器各 1 次）、依赖容器 0 个。Sub2API、其他 10 个卫星、AgentAPI、参考 Cost Console 及仓库外容器均排除；`sub2api-postgres`、`sub2api-redis` 仅做只读查询，未重建未改动。

背景：2026-10-23 20:28 的全量覆盖部署把 `ppt-production-1` 换成 `local-deploy/ppt-production:modelplaza-platinum-20261023`，并新建了空的 `C:\Users\kodesh\Desktop\CC2CX\ppt\app_data`。新库为空导致 `GET /api/v1/ppt/presentation/all` 返回 428（`Login setup is required`，见 `api/v1/auth/router.py:187`），而不是未登录应有的 401，且原有 9 个用户、16 个演示文稿全部不可见。

兼容性核对：新镜像源码与工作区一致（`sub2api_sso.py` 等文件仅 CRLF/LF 差异，规范化后哈希相同），`MIGRATE_DATABASE_ON_STARTUP=false`，启动走 `SQLModel.metadata.create_all` 且不删表。旧库 20 表、新镜像 18 表，差异仅为旧库多出 `alembic_version`、`api_keys` 两表及 `async_tasks.payload` 一列，新代码均不读取，向后兼容。

恢复方式：先 `docker stop ppt-production-1`，把真实库 `fastapi.db`（4,493,312 字节，SHA-256 `6B62E43A…56A`）与 `userConfig.json`、`userConfig.json.bak` 覆盖到当前挂载目录，再用 robocopy `/XC /XN /XO` 补齐缺失资产（不覆盖较新文件），然后 `docker start`。覆盖前已把当时的空数据目录整目录备份到 `.local/ppt-restore-to-platinum-20261024-100649/empty-app_data`（219 个文件）。

验收通过（真实链路，非模拟）：用容器内 `SUB2API_SSO_SECRET` 签发真实票据，`/api/v1/auth/sso/callback` 303 → `/api/v1/auth/sso/exchange`（带 `X-Presenton-SSO: 1`）200，会话 Cookie `presenton_session` 为 `Max-Age=259200`（3 天）、HttpOnly、SameSite=lax，handoff Cookie 兑换后 `Max-Age=0` 清除；票据重放 303 `/?error=SSO_failed`；`role=user` 访问 `/api/user-config` 为 403；`/api/runtime-config` 中 `CUSTOM_LLM_API_KEY` 显示 `__configured__`。完整生成链路：创建演示文稿 200 → 大纲流式 200（约 10–12 秒，2 页）→ `prepare` 200（版式 id 必须传模板 id 如 `general`，传 `general:general` 会 404 `Template layout not found`）→ 幻灯片流式 200（2 页）→ 导出 PPTX 200，产物 493,772 字节、2 张幻灯片、10 个媒体文件、ZIP 校验通过。模型调用经 `http://sub2api:8080/v1`，网关记录 `POST /v1/chat/completions` 200。

数据核对：恢复后 `user=9`（`talent` 管理员 + 8 个 `sub2api-*` SSO 用户）、`presentations=16`、`slides=13`、`template_v2=8`，与恢复前完全一致；`userConfig.json` 十个字段逐项相同（`IMAGE_PROVIDER=openai_compatible`、`OPENAI_COMPAT_IMAGE_MODEL=gpt-image-2` 均已找回）。验收产生的合成用户、3 个演示文稿与 2 个导出文件已全部删除，回到 9/16/13。

未解决（与本次部署无关）：幻灯片配图请求 `POST /v1/images/generations` 全部 502，上游账号 `731`、`745` 返回 403/EOF，`switch_count` 达上限 10 次。属上游图片账号可用性问题，本次未处理；文本链路不受影响。

未执行：未重新构建镜像，未修改任何源码，未改动 Sub2API 侧配置。

## 2026-10-24 卫星源码更新后重建覆盖（12 个项目）

范围：主站 1 个（Sub2API）、卫星 11 个（aicut、aiexcel、ai3d、aihuoke、canvas、ju、livart、ppt、qrcode、screen2code、yibiao），共 12 个项目，重建应用容器 15 个、依赖容器 0 个。AgentAPI 代理站本次未在要求范围内，保持上一轮镜像不动；参考 Cost Console 及所有 `crm-*`、`cs-activation-*`、`xianyu-*` 仓库外容器均排除。`sub2api-postgres`、`sub2api-redis`、`ju-sso-redis-1`、`livart-postgres`、`livart-rabbitmq-1`、`livart-minio-1` 六个依赖容器未重建。

源码基线为工作区 `C:\Users\kodesh\Desktop\Workspace\sub2api`（分支 `talent`，HEAD `61aa73ee`）。相较上一轮 `modelplaza-platinum-20261023`，本轮源码有三处实质改动，均已随镜像生效：

1. `ai3d` 托管文本与视觉默认模型由 `gpt-5.5` 改为 `gpt-6-astra`（`server/config.mjs` 新增 `managedRelayModel()`，`server/providers/vision.mjs` 改用该函数），并改进视觉分类：未知 `categoryId` 时用模型描述内容作为回退提示，不再一律落入通用商品桶。
2. `ju` 后端 `configuredSub2APIModels()` 默认模型由 `gpt-5.6-sol` 改为 `gpt-6-astra`（`internal/app/sub2api_integration.go`）。
3. `yibiao` 修复 `server/app.py` 中 `run_once` 的乱码提示，`"妫€绱㈠姟姝ｅ湪杩愯"` 更正为 `"检索任务正在运行，请等待本轮完成后再试。"`。

镜像统一使用 `modelplaza-astra-20261024` 标签，覆盖 `local-deploy/` 下 15 个应用镜像。`aihuoke-backend`、`ju-web`、`qrcode` 因源码未变命中层缓存，镜像内容与上一轮一致。

构建过程中的网络故障与处置（均为构建环境问题，不涉及业务代码）：

1. `aicut`、`aiexcel` 首次构建 npm 报 `ECONNRESET`，重试后成功。构建前已验证 `registry.npmjs.org` 与 `registry.npmmirror.com` 均可 ping 通，属瞬时断流。
2. `ppt` 在批量构建中走原始 Dockerfile，运行阶段需从 `deb.debian.org` 拉取约 302 MB Noto 字体包，再次超时失败；改用上一轮已验证的补丁 Dockerfile（`Dockerfile.ppt`，以 BuildKit 命名上下文 `fontdebs` 挂载预取的 8 个 `.deb` 后用 `dpkg -i` 安装）后构建成功。
3. `screen2code-backend` 的 `playwright install --with-deps chromium` 连续三次失败：先是 4 个 Debian 包（`fonts-wqy-zenhei`、`libz3-4`、`fonts-ipafont-gothic`、`fonts-noto-color-emoji`）下载超时；改为预置 apt 归档缓存后，`apt-get update` 本身又收到 `deb.debian.org` 的 502。最终在宿主机预取这 4 个包，并在构建阶段把 apt 源切换到 `mirrors.aliyun.com`（实测 InRelease 响应 0.17 s，`deb.debian.org` 为 2.24 s），配合归档缓存后构建通过。
4. 期间 `docker buildx` 出现一次 `NotFound: forwarding Ping: no such job`，为 buildx 内部任务丢失，执行 `docker buildx prune` 后重试即恢复。

配置保持：重建前后各做一次逐项比对，16 个容器的环境变量、挂载、端口全部一致（`sub2api` 117 项、`ppt-production-1` 127 项、`livart-livart-1` 51 项、`ju-backend-1` 24 项、`aihuoke-backend` 23 项、`ai3d` 20 项、`canvas-app` 17 项、`aicut`/`yibiao` 各 16 项、`qrcode` 15 项、`screen2code-backend-1` 14 项、`aiexcel` 9 项、`ju-web-1` 6 项、`aihuoke-frontend`/`screen2code-frontend-1` 各 5 项）。端口映射、数据卷、网络、SSO 与服务端凭据、启动命令均未改变。

验收通过：15 个应用容器全部以新镜像运行且 `RestartCount=0`，`sub2api`、`aiexcel`、`ju-web-1`、`ju-backend-1`、`livart-livart-1` 健康检查为 healthy。`sub2api` 主页与 `GET /api/v1/model-plaza`、`canvas`、`aicut`、`aiexcel`、`ai3d`、`aihuoke` 前端、`ju` web、`livart` `/api/health`、`ppt`、`qrcode`、`screen2code` 前后端、`yibiao` 全部返回 200；11 个卫星的 `GET /api/v1/auth/integrations/:slug/start` 均返回 401（未登录时的预期行为，路由注册正常）。模型广场仍为 25 个分组、732 个模型、0 个空分组，厂家维度 `[OI]` 418、`Anthropic` 198、`Grok` 77、`智谱 GLM` 31、`DeepSeek` 3、`MiniMax` 2、`字节豆包` 2、`Kimi` 1，功能维度文本 636、图片 83、视频 13，与上一轮一致。

源码改动生效核验：`yibiao` 容器内 `app.py` 含正确文案 1 处、乱码 0 处；`ju` 后端二进制含 `gpt-6-astra` 1 处、`gpt-5.6-sol` 0 处。`ai3d` 容器内 `config.mjs` 已含 `gpt-6-astra`，但运行时 `SUB2API_RELAY_MODEL=gpt-5.5` 由 Compose 环境变量显式提供，按“保留既有配置”原则未改动，因此 ai3d 实际仍调用 `gpt-5.5`；若需切换到 `gpt-6-astra`，需另行修改该环境变量。

数据核对：`aihuoke` 的 `auth.db`、`automation_queue.db`、`email_automation.db`、`domain_email_patterns.json`、`user-settings` 及 `hunts` 历史记录均在；`ppt` 的 `fastapi.db` 为 4,493,312 字节，与上一轮一致；`livart` 的 `livart.jar` 76,725,818 字节正常。未删除任何卷，未覆盖数据库。

已知现象：`ju-backend-1` 在构建高峰期间出现 3 次 `read tcp ...:6379: i/o timeout`（concurrency lease 与 task dispatch），构建结束后 1 分钟内无复现，`ju-sso-redis-1` 健康且 `PING`/`nc` 连通，`ju` web 返回 200，判定为构建期 I/O 竞争导致的瞬时超时，非配置或代码缺陷。日志中 `record not found` 为 GORM 查询 `system_settings.runtime_policy` 的常规模式。

未执行：未在浏览器中完成真实 SSO 登录与业务操作（需真实主站账号与上游额度），未发起收费模型生成，未改动任何源码与环境变量。构建日志、补丁 Dockerfile、deb 预取目录、比对脚本保存在 Git 忽略目录 `.local/modelplaza-astra-20261024/`。回滚镜像为 `local-rollback/<容器名>:modelplaza-astra-20261024`，共 15 个，指向上一轮 `modelplaza-platinum-20261023` 镜像。

## 2026-10-24 Sub2API 与 11 个卫星再次源码覆盖部署

范围：主站 1 个、卫星 11 个、代理站 0 个；重建 15 个应用容器，6 个依赖容器未重建。源码为 `C:\Users\kodesh\Desktop\Workspace\sub2api`（`talent`，HEAD `61aa73ee`）。AgentAPI、Cost Console 及仓库外容器排除。镜像统一标签为 `redeploy-20261024-1520`。

15 个应用容器均从当前源码构建并强制重建；Sub2API、aiexcel、JU 前后端、Livart 健康检查通过，所有目标 RestartCount 为 0。部署前后逐项核对环境变量、数据挂载、端口、网络均无差异。12 个入口均返回 200，11 个卫星的 SSO 启动在匿名状态下均返回预期 401；主站健康接口、模型广场、Livart 健康接口及 Screen2Code 后端入口正常。PPT 数据完整性检查为 ok，用户 9、演示文稿 16、幻灯片 13；Screen2Code 认证库完整性检查为 ok。

构建期间 AI 剪辑首次 npm 下载遇到 ECONNRESET，重试成功；PPT 与 Screen2Code 后端使用已验证的补丁配方并分别补齐字体与 Chromium 依赖构建上下文，应用源码仍来自当前工作区。Sub2API 和 Livart PostgreSQL 已导出，PPT 与 Screen2Code 的 SQLite 已做一致性备份。构建日志、运行快照和备份位于 Git 忽略目录 `.local/redeploy-20261024-1520/`。回滚镜像为 `local-rollback/<容器名>:redeploy-20261024-1520`。未执行真实登录后的卫星业务、收费模型生成或支付退款验证。

## 2026-10-24 Sub2API 与 11 个卫星第三次源码覆盖部署

范围：主站 1 个、卫星 11 个、代理站 0 个；重建 15 个应用容器，6 个依赖容器未重建。源码为 `C:\Users\kodesh\Desktop\Workspace\sub2api`（`talent`，HEAD `61aa73ee`）。AgentAPI、Cost Console 及仓库外容器排除。镜像统一标签为 `redeploy-20261024-1645`。

15 个应用容器均从当前源码构建并强制重建，构建日志无失败。工作区自上一轮 15:20 部署后无源码改动，15 个新镜像的 RootFS 层与上一轮逐一相同（构建缓存全命中），镜像 ID 因重新导出 manifest 而不同；容器已全部切换到新标签运行。Sub2API、aiexcel、JU 前后端、Livart 健康检查通过，全部目标 `RestartCount=0`。部署前后逐项核对 15 个容器的环境变量、数据挂载、端口、网络与启动命令，均无差异；6 个依赖容器容器 ID 未变。

12 个入口全部返回 200，11 个卫星的 `GET /api/v1/auth/integrations/:slug/start` 在匿名状态下均返回预期 401；主站 `/health`、`GET /api/v1/model-plaza`（25 个分组、732 个模型）、Livart `/api/health`、Canvas `/api/health`、PPT `/api/v1/auth/status` 正常。近 10 分钟日志无 panic、Traceback 或 fatal error。

数据核对：PPT 的 `fastapi.db` 完整性 ok，用户 9、演示文稿 16、幻灯片 13、模板 8，字节数 4,493,312 与部署前一致，`userConfig.json` 哈希未变；Screen2Code、AI 获客（身份 3、会话 33，含 3 个用户库）、易标（身份 5、会话 25）、Canvas、JU（`open_ai_canvas.db` 3.4 MB、120 个插件包）的 SQLite 完整性均为 ok。Sub2API 与 Livart PostgreSQL 已导出，8 个命名卷已做文件级快照，PPT 与 Screen2Code 的 SQLite 另做一致性副本。备份、构建日志、运行快照与比对结果位于 Git 忽略目录 `.local/redeploy-20261024-1645/`。回滚镜像为 `local-rollback/<容器名>:redeploy-20261024-1645`，指向本次部署前的运行镜像。

未执行：未在浏览器中完成真实 SSO 登录与卫星业务操作（需真实主站账号与上游额度），未发起收费模型生成，未修改任何源码或环境变量。

## 2026-10-24 Sub2API 左侧栏移除版本号

范围：主站 1 个（Sub2API），卫星 0 个、代理站 0 个；重建应用容器 1 个，依赖容器 0 个。其余 11 个卫星、AgentAPI、Cost Console 及仓库外容器均未操作。

改动：`sub2api/frontend/src/components/layout/AppSidebar.vue` 移除侧栏品牌区的 `<VersionBadge>` 渲染及其导入和 `siteVersion` 计算属性；`VersionBadge.vue` 组件与主站版本检查接口保留，仅不再显示在左侧栏。

验证：`pnpm vitest run` 的 AppSidebar 与 VersionBadge 用例 19 项通过，`pnpm typecheck` 通过。重建后 `sub2api` 运行新镜像 `local-deploy/sub2api:sidebar-noversion-20261024-1725`，状态 running、健康检查 healthy、`RestartCount=0`；首页、`/health`、`/api/v1/model-plaza` 均返回 200。新容器内嵌前端二进制中 `VersionBadge` 计数为 0，回滚镜像对应计数为 1，确认产物已更新。构建日志与回滚快照位于 Git 忽略目录 `.local/sidebar-noversion-20261024-1725/`，回滚镜像为 `local-rollback/sub2api:sidebar-noversion-20261024-1725`。

未执行：未在浏览器中登录后逐页点击核对侧栏视觉效果。

## 2026-10-26 AgentAPI 前后端覆盖部署

范围：主站 0 个、卫星 0 个、代理站 1 个（AgentAPI）；重建应用容器 1 个，依赖容器 0 个。Sub2API、11 个卫星、Cost Console 及仓库外容器均未操作。AgentAPI 前端编入后端镜像并由同一 Go 进程托管，本次一次重建即覆盖前后端。

源码为 `C:\Users\kodesh\Desktop\Workspace\sub2api\agentapi` 当前工作区，包含尚未提交的参考式首页改版。镜像为 `local-deploy/agentapi:redeploy-20261026-0953`，运行容器 `agentapi-agentapi-1`；部署前后环境变量、端口 `18081`、数据卷 `agentapi_agentapi-data`、`agentapi_default` / `deploy_sub2api-network` 双网络、只读根文件系统与启动参数均无差异，新容器 `RestartCount=0` 且健康检查为 healthy。

验证：后端 `go test ./...` 通过；前端 `pnpm typecheck`、`pnpm test:run`（66 个文件、328 项）通过，Docker 内前端与 Go 构建成功。`VerifyDeployment.ps1 -AllowHttp` 全部通过：首页 200、`/healthz` ok、`/readyz` ready、未知 Host 421、20 条退役路由 404、入口 HTML 与 JavaScript 无服务端密钥或 SuperKey。运行中首页资源包含新版 `reference-home` 与 `CCSwitch` 内容；主站 AgentAPI SSO 启动匿名状态返回 401。部署前后 SQLite 一致性备份 `integrity_check` 均为 ok，26 张表、74 条会话未变；启动日志无 panic/ERROR。

构建、运行快照、验收 JSON 与部署前后备份位于 Git 忽略目录 `.local/agentapi-deploy-20261026-0953/`；回滚镜像为 `local-rollback/agentapi:redeploy-20261026-0953`，指向部署前运行的 `local-deploy/agentapi:modelplaza-platinum-20261023` 镜像。首次构建因 Docker Hub 拉取 `golang:1.27-bookworm` 元数据超时失败，补齐本地基础镜像后重试成功，未修改 Dockerfile。

未执行：未使用真实主站账号完成浏览器 SSO 登录，未发起充值、支付、模型请求或跨租户隔离业务回归。

## 2026-10-26 Sub2API 主站覆盖部署（一键开分站体验）

范围：主站 1 个（Sub2API），卫星 0 个、代理站 0 个；重建应用容器 1 个，依赖容器 0 个。AgentAPI 保持上一轮镜像继续运行，未重建；11 个卫星、Cost Console 及仓库外容器均未操作。

功能核对：左侧菜单 `nav.agentApi` 仍为“一键开分站”，入口为 `GET /api/v1/auth/integrations/agentapi/start?next=%2Fdashboard`；`satellite_sso.go` 在 `agentapi` slug 上调用 `EnsureSharedTenant`，按当前登录用户幂等创建或复用共享租户后签发 SSO 票据。部署镜像内已确认包含“一键开分站”和 `/auth/integrations/agentapi/start`，数据库现有 2 个 active 共享租户记录保留。

镜像为 `local-deploy/sub2api:oneclick-20261026-1022`，容器 `sub2api` 已强制重建；构建缓存命中，镜像内容与部署前运行镜像一致。部署前后 117 项环境变量、端口 `18080`、数据卷 `deploy_sub2api_data`、`deploy_sub2api-network`、启动参数与资源限制均无差异，`RestartCount=0`、健康检查 healthy。

验证：首页与 `/health` 返回 200，`GET /api/v1/model-plaza` 返回 200（688,700 字节）；AgentAPI SSO 启动在匿名状态返回预期 401；AgentAPI 容器仍为 running/healthy，`/healthz` 200。运行日志无 panic/ERROR。前端 `pnpm typecheck` 与侧栏/模型广场定向测试 52 项通过；后端 `-tags unit` 的 AgentAPI SSO、模型广场相关测试通过。

部署前已导出 PostgreSQL 自定义格式备份（`pg_restore -l` 检出 103 个表数据段）和 `deploy_sub2api_data` 卷快照；构建、运行前后快照、部署日志与备份位于 Git 忽略目录 `.local/sub2api-oneclick-20261026-1022/`。回滚镜像为 `local-rollback/sub2api:oneclick-20261026-1022`。

未执行：未使用真实主站账号在浏览器中点击“一键开分站”完成登录后体验，未发起模型请求、充值或支付。

## 2026-10-26 AgentAPI 品牌设置修复与覆盖部署

范围：主站 0 个、卫星 0 个、代理站 1 个（AgentAPI）；重建应用容器 1 个，依赖容器 0 个。Sub2API、11 个卫星、Cost Console 及仓库外容器均未操作。

问题与修复：系统设置页原本把后端站点标识名称与公开站点名称混作同一个显示字段，导致修改名称后侧栏、Header、登录页和浏览器标题不一致；共享入口匿名访问无法解析租户时，前端还可能回退到默认品牌。复测时进一步确认，两个名称字段并排仍会让站长把公开名称填进后台备注字段：保存请求成功、副标题已落库，但 `site_name` 未改变，因此界面看起来没有生效。最终界面只保留一个主字段“站点名称”，直接控制 `site_name`，并应用于首页、侧栏、Header、登录页和浏览器标题；“后台备注名称”移入高级折叠项且可留空，留空时自动使用公开名称。品牌预览同步显示名称与副标题。Session 登录/SSO 后从租户上下文刷新品牌，退出后清理会话品牌；页面标题随品牌和路由同步更新。登录、注册、找回密码和重置密码页的副标题改为读取当前租户品牌。Logo 上传地址增加基于文件内容的版本参数，避免同扩展名覆盖后浏览器与 favicon 继续使用旧缓存。共享入口继续保留匿名 Host 421 的安全边界。

该轮最终镜像为 `local-deploy/agentapi:branding-20261026-125040`，容器 `agentapi-agentapi-1`，入口 `http://localhost:18081`。部署前后 31 项环境变量、端口 `18081 -> 8080`、数据卷 `agentapi_agentapi-data`、`agentapi_default` / `deploy_sub2api-network` 双网络、只读根文件系统、init、重启策略、capabilities 和启动参数均保持一致；最终状态 running/healthy、`RestartCount=0`。

验证：前端 `vitest` 67 个文件、333 项通过，`vue-tsc --noEmit` 与新增/改动文件 ESLint 通过；后端未在本轮继续修改，前轮 `go test ./... -count=1` 全量通过；Docker 内前端与 Go 构建成功。`VerifyDeployment.ps1 -AllowHttp` 全部通过：首页 200、`/healthz` 200/ok、`/readyz` 200/ready、未知 Host 421、20 条退役路由 404、入口 HTML 与 JavaScript 未包含服务端密钥或 SuperKey。运行资源包含“高级：后台备注名称”等新文案；容器日志无 panic/ERROR。部署后 SQLite `integrity_check=ok`，27 张表、75 条 Session、12 个 active 租户；已确认历史保存的数据仍在。

构建、容器配置前后快照、数据库副本、验收 JSON 与日志位于 Git 忽略目录 `.local/agentapi-branding-20261026-125040/`。一步回滚镜像为 `local-rollback/agentapi:branding-20261026-125040`，上一版回滚镜像为 `local-rollback/agentapi:branding-20261026-120443`，原始修复前回滚镜像为 `local-rollback/agentapi:branding-20261026-115505`。

未执行：未使用真实主站账号完成浏览器 SSO 登录后的逐页视觉核对，未发起充值、支付、模型请求或跨租户隔离业务回归。

## 2026-10-26 AgentAPI Logo 图片 Base64 入库与覆盖部署

范围：主站 0 个、卫星 0 个、代理站 1 个（AgentAPI）；重建应用容器 1 个，依赖容器 0 个。Sub2API、11 个卫星、Cost Console 及仓库外容器均未操作。

功能与修复：系统设置的 Logo 上传改为读取图片字节、按真实文件头识别 PNG / JPEG / WEBP / GIF，并生成 `data:image/<type>;base64,...` 写入当前站点的 `agent_config.site_logo`，不再向本地 `logos` 目录写入新文件。单张图片限制为 1 MB；后端同时校验 data URL 前缀、Base64 编码、声明 MIME、解码后大小和实际图片类型，拒绝 HTML、SVG、脚本协议、错误 Base64 及 MIME 与内容不一致的输入。设置页上传成功后显示“图片已内嵌为 Base64，并存入当前站点数据库”，不再展示超长 Base64 文本，并提供“移除 Logo”。旧 `/branding/logos/...` 读取接口保留，兼容数据库中已有的文件型 Logo。

最终镜像为 `local-deploy/agentapi:logo-base64-20261026-131015`，镜像 ID `sha256:e16393135aa6501ae677d164db3cbf2b727f6cf220429fef79c6789bb10b4523`，容器 `agentapi-agentapi-1`，入口 `http://localhost:18081`。部署前后 31 项环境变量、端口 `18081 -> 8080`、数据卷 `agentapi_agentapi-data`、`agentapi_default` / `deploy_sub2api-network` 双网络、只读根文件系统、重启策略和挂载均无差异；最终状态 running/healthy、`RestartCount=0`。

验证：后端 `go test ./... -count=1` 全部通过；前端定向设置页测试 9 项通过，本轮改动前全量 `vitest` 67 个文件、334 项通过。Docker 内 Go 与前端构建成功，运行资源包含“图片已内嵌为 Base64”；`VerifyDeployment.ps1 -AllowHttp` 全部通过：首页 200、`/healthz` 200/ok、`/readyz` 200/ready、未知 Host 421、20 条退役路由 404、入口 HTML 与 JavaScript 未包含服务端密钥或 SuperKey。部署后 SQLite `integrity_check=ok`，27 张表（26 张业务表加 `sqlite_sequence`）、12 个 active 站点、75 条 Session 均保留。

构建、部署前后容器快照、数据库副本、验收 JSON 与日志位于 Git 忽略目录 `.local/agentapi-logo-base64-20261026-131015/`；`postflight.json` 汇总本轮检查。一步回滚镜像为 `local-rollback/agentapi:logo-base64-20261026-131015`，镜像 ID `sha256:5a9ec73810ac7d45eb699b79f73402becec433dd8668e868cae04a5f98e85201`。

未执行：未使用真实站点账号上传用户提供的 Logo 做视觉验收（避免擅自修改现有品牌），未发起充值、支付或模型请求。

## 2026-10-26 AgentAPI 线上覆盖部署（Logo Base64）

范围：主站 0 个、卫星 0 个、代理站 1 个（AgentAPI）；重建应用容器 1 个，依赖容器 0 个。Sub2API 主站、11 个卫星、Cost Console、CRM 及其他仓库外容器均未操作；线上除 `agentapi` 外没有容器在本次部署时间窗内重启。

线上基线：Ubuntu 24.04、Docker 29.7.2、Compose 5.5.0，原容器 `agentapi` 使用 `cc2cx/agentapi:20261026-1103`，公网入口 `https://agentapi.cc2.cx`。本次将本地已通过测试的 `linux/amd64` 镜像 `cc2cx/agentapi:20261026-1338` 上传并加载到线上，镜像 ID 为 `sha256:e16393135aa6501ae677d164db3cbf2b727f6cf220429fef79c6789bb10b4523`。因此线上包含本轮 Logo 图片 Base64 入库、品牌名称修复和设置页提示。

部署前后 28 项环境变量数量及值摘要、端口 `127.0.0.1:18081 -> 8080`、`agentapi-data` 数据卷、`sub2api-deploy_sub2api-network` 网络、只读根文件系统、init、重启策略、tmpfs、安全选项和 capabilities 全部一致。最终容器为 running/healthy，`RestartCount=0`，容器错误日志为 0 字节。

验证：通过 SSH 隧道将容器端口映射到本机后运行官方 `VerifyDeployment.ps1`，首页 200、`/healthz` 200/ok、`/readyz` 200/ready、未知 Host 421、20 条退役路由 404、入口资源不含服务端密钥，7 项全部通过；公网 `https://agentapi.cc2.cx` 的首页、健康与就绪接口也均为 200。公网带未知 Host 请求会先落到已有 Nginx 默认站点并返回 200，直接访问 AgentAPI 时同一请求返回 421；这是外层反代行为，本次未修改 Nginx。

部署前后均使用 AgentAPI 自带 `--backup` 生成一致性 SQLite 副本并复制到数据卷之外。两份数据库 SHA-256 均为 `30e7af8fd59cbba613b9ff612f3443e89638f2277272897ee964851ab22f9b94`，`integrity_check=ok`，27 张表、14 个 active 站点、53 条 Session 均保留。线上备份在部署验收后已按用户要求清理；本地一致性副本与证据仍保留在 Git 忽略目录 `.local/remote-agentapi-logo-base64-20261026-133800/`，其中 `postflight.json` 汇总部署结果。

回滚镜像为 `cc2cx/agentapi:rollback-logo-base64-20261026-133800`，对应原镜像 ID `sha256:a568255a4dd5686d3d4bd3c071804270121b76c6a6a35133de2e058885a1c0c7`。线上保留了 `agentapi-logo-base64-20261026-133800.override.json` 供本次部署重建和追溯。

风险提示：部署时服务器根分区已使用约 97%，仅余约 5.9 GB；本次只增加约 19 MB 镜像归档，未清理或删除任何既有镜像、容器和数据。未执行真实用户 SSO 登录、Agent Key 调用、充值、支付或模型请求；服务器上其他业务的高负载与磁盘空间问题不在本次 AgentAPI 部署范围内。

## 2026-10-26 线上非源码备份清理

用户要求清理服务器端备份并明确禁止删除任何项目源码。清理前先生成删除清单和源码保护哈希，执行前校验受保护文件与目录，执行后再次校验；删除操作为一次性精确清单，不使用全局清理脚本、不删除源码树、数据库数据卷或运行中的业务容器。

本次删除了 26 项历史数据库/数据备份和本次 AgentAPI 临时镜像归档，共约 3.78 GB；同时移除两个未被任何容器使用的旧 AgentAPI 镜像标签 `cc2cx/agentapi:20260929`、`cc2cx/agentapi:20261009`。删除内容包括 `/opt/sub2api-backups/20261001T112512Z`、`/opt/sub2api_backup` 各旧快照中的数据库/Redis/数据文件、`/opt/cc2cx-backups` 中的数据库导出与本次 AgentAPI 临时备份、`/opt/cc2cx-production/agentapi-backup-20261026-1103`。

明确保留且已通过 SHA-256 或文件数/大小复核的源码资源包括：`/opt/sub2api_backup/20260928T0315Z/previous-source.tar.gz`（614,768,781 字节）、AgentAPI 被覆盖源码归档与 `.go` 文件、`repo-untracked-files.tar.gz`、`repo-working-tree.patch`、`/opt/payment-reliability/backups/api.php.before-20261015`、`/opt/upstream-svg-lab-backups/`、`/opt/upstream-svg-lab.backup-group-20261018004045/`，以及 `/opt/cc2cxSub2api_all`、`/opt/cc2cxSub2api_260920`、`/opt/sub2api-back`、`/opt/cc2cx-production/release-20261026` 等源码/运行源码目录。全部复核未变化。

清理后服务器根分区可用空间由约 6.3 GB 增至约 9.5 GB，使用率由 97% 降至 96%；AgentAPI 仍为 `running/healthy`、`RestartCount=0`，公网首页、健康与就绪接口均返回 200。立即回滚镜像 `cc2cx/agentapi:rollback-logo-base64-20261026-133800` 和上一版 `cc2cx/agentapi:20261026-1103` 保留。清单、删除结果和源码校验记录位于 Git 忽略目录 `.local/remote-backup-cleanup-20261026-135929/`。

## 2026-10-27 Sub2API 主站与 AgentAPI 代理站覆盖部署及登录修复

范围：主站 1 个、卫星 0 个、代理站 1 个；重建应用容器 2 个（`sub2api`、`agentapi`），依赖容器 0 个。11 个卫星、Cost Console、CRM 及仓库外容器均未操作。`deploy` Compose 项目中的 `canvas-app`、`sub2api-postgres`、`sub2api-redis` 仅被识别为 orphan 警告，未重建、未删除。

镜像与回滚：主站为 `local-deploy/sub2api:ui-redeploy-20261027-133209`，回滚 `local-rollback/sub2api:redeploy-20261027-133209`；AgentAPI 首轮为 `local-deploy/agentapi:ui-redeploy-20261027-133209`，登录修复后为 `local-deploy/agentapi:login-sso-20261027-1425`，回滚 `local-rollback/agentapi:redeploy-20261027-133209`，修复前中间镜像 `local-rollback/agentapi:pre-login-sso-20261027-1425`。

备份位于 `.local/sub2api-agentapi-redeploy-20261027-133209/`：`sub2api-before.dump` 69,264,555 字节，SHA-256 `a5199e290ad91207959a88eddb45b221d2356e093d770d87f731da1ac1fd7559`；`sub2api_data-before.tgz` 3,010,627 字节，SHA-256 `c02cd63538ff77fd95ad1b809c8fa08554d4e88bd0cb524b16d8b3b1dbf0c74e`；`agentapi-before.db` 344,064 字节，SHA-256 `155050ca73b22340094af2d70ad7cb61b200b3e6047bc39653839d8bfc21287e`，`integrity_check=ok`，27 表、75 Session。部署后 `agentapi-after.db` 与部署前哈希相同；登录测试清理后 `agentapi-after-cleanup.db`，`integrity_check=ok`，27 表、75 Session，测试租户 0 行。

验证：两容器 running/healthy，`RestartCount=0`；镜像、端口、卷、网络、环境变量数量与部署前一致（`sub2api` 117 项、`agentapi` 31 项）；其他容器 ID 未变化，`sub2api-postgres`/`sub2api-redis` 未重建。`http://127.0.0.1:18080/health` 返回 200 `{status:ok}`，主站首页 200。AgentAPI `VerifyDeployment.ps1` 7 项全部通过：首页/`healthz`/`readyz` 200、未知 Host 421、20 条退役路由 404、入口资源无服务端密钥。主站计数保持 `users=183`、`api_keys=147`、`agent_shared_tenants=2`。新版产物已确认包含 `UNIFIED AI ACCESS`、`欢迎回来`、`共享代理站`、`使用主站账号登录`。

登录问题排查与修复：共享入口 `localhost:18081` 使用邮箱密码直登时，主站 `/auth/login` 成功后，AgentAPI `establishSession` 仍返回 421 `TENANT_NOT_FOUND`；共享主机在认证前没有租户上下文，这是结构性限制，不是密码错误。临时本地账号复现为主站登录 200、AgentAPI 直登 421；主站 SSO 入口 302 → AgentAPI 回调 302 `/dashboard` → `/api/v1/auth/me` 200，`agent_admin=true`。测试账号/租户已清理，主站计数恢复 183/147/2，AgentAPI 恢复 27 表/75 Session。

当时修复内容（该共享入口登录策略已被同日 16:25 的“AgentAPI 密码登录与白金主题修复”取代）：新增 `GET /api/v1/auth/login-options`，按当前 Host 返回 `shared_runtime`、`password_login_enabled`、`main_site_sso_enabled`；共享入口暂时只展示主站 SSO，隐藏邮箱密码、Passkey、注册入口；后端在当时直接拒绝密码登录并返回 `MAIN_SITE_SSO_REQUIRED`。后端 `go test ./...`、前端 `pnpm typecheck`、`pnpm test:run`（67 文件、337 测试）、`pnpm build` 均通过。

界面验收：已使用一次性主站账号完成真实 SSO，浏览器检查 `/dashboard`、`/keys`、`/usage`、`/admin/dashboard`、`/admin/settings`、`/admin/users`、`/admin/usage`、`/admin/payments` 共 8 个登录后页面，浅色/深色各一张，共 16 张；同时检查首页与共享登录页浅深主题。页面均为暖白/暖黑底与金色强调，侧栏、卡片、表格、表单、状态提示和图表容器一致。源码扫描未发现 `blue`、`indigo`、`violet`、`purple`、`cyan`、`sky` 系列 Tailwind 冷色类名；剩余深灰 `#111827` 仅用于二维码定位。新手引导先写入已读标记再复拍，避免遮罩影响主题判断。截图位于 `.local/agentapi-theme-audit-20261027/`。

验收清理：一次性账号、自动创建的 Super Key、共享租户、AgentAPI 用户/站点/钱包/支付配置、Session 与 SSO 票据均已删除。重启后 AgentAPI `/healthz` 返回 200 `ok`、`/readyz` 返回 200 `ready`（`active_tenants=12`），SQLite `integrity_check=ok`，75 Session、12 站点、12 用户映射、临时记录 0；主站计数恢复 `users=183`、`api_keys=147`、`agent_shared_tenants=2`。两容器均为 running/healthy、`RestartCount=0`。

当时限制/未执行：该节完成时本地主站库尚未出现 `vip@qq.com`，因此未直接验证该账号。15:53 后主站已存在 `vip@qq.com`（用户 ID 187、status active）；本次最新部署确认该账号可见，但未取得其密码，仍不能代替用户输入密码完成登录。未执行支付/充值或收费模型请求。Cost Console、11 卫星、CRM 与仓库外容器排除。

## 2026-10-27 AgentAPI 密码登录与白金主题修复（16:25）

范围：主站 0 个、卫星 0 个、代理站 1 个（AgentAPI）；重建应用容器 1 个（`agentapi-agentapi-1`），依赖容器 0 个。Sub2API 主站、11 个卫星、Cost Console、CRM 及其他仓库外容器均未操作；主站代码未修改。上一节“共享入口拒绝密码登录”的结论从本次部署起失效。

根因：共享入口在认证前只有 Host、没有租户上下文，旧实现直接拒绝邮箱密码并返回 `421 MAIN_SITE_SSO_REQUIRED`，因此同一个 `vip@qq.com` 在 `18080` 主站可以登录、在 `18081` 代理站不能登录，并不是密码错误。新流程为：主站邮箱密码登录成功并取得主站 access token → AgentAPI 服务端携带 Bearer 调用主站既有 `GET /api/v1/auth/integrations/agentapi/start?next=/dashboard` → 主站创建或复用该用户的共享租户并返回签名 ticket → AgentAPI 校验 ticket、确认 `sub` 与登录用户一致、解析租户 → 建立 3 天 HttpOnly 租户 Session。普通密码、2FA 和 Passkey 登录路径均已覆盖；主站 SSO 保留为第二种登录方式。

镜像与回滚：新镜像 `local-deploy/agentapi:password-theme-20261027-1625`（镜像 ID `sha256:7f978190cf1aa147e434e7d52e8fc8595d044bb8df24968ce27aa47658bbf105`，18,824,015 字节）；回滚镜像 `local-rollback/agentapi:pre-password-theme-20261027-1625`（`sha256:f54b1d4e97cd5afece5478efc78b78930b2776ca07ad0963b3cc53a4e52e8974`）。容器 ID `b18bdc85d06a901e71ddde1f4928b5f39659e3ef3dc5120eccb1c63b92d2813b`，端口 `18081 -> 8080`，数据卷、网络、只读根文件系统和安全配置沿用原部署。

备份位于 `.local/agentapi-password-theme-20261027-1625/`：部署前 `agentapi-before.db` 405,504 字节，SHA-256 `3c2277efcf05c153b2cc6dc8a8f7a553f6e5aee748837f70e2c137fd489c781b`；清理前一致性副本 `agentapi-before-cleanup.db` 409,600 字节，SHA-256 `5bb8f2b378e254b7c510e92b1c03d84c79b104d3759ddee197512422ab68e1e9`；清理后 `agentapi-after-cleanup.db` 405,504 字节，SHA-256 `dc28b2ac735184bba12777ff10e8c9f82dfabe1def3c29f19c62778e6e5ca4f0`。三份数据库均 `integrity_check=ok`；备份、镜像前后状态和部署日志同目录保留。

验证：真实密码登录返回 200，身份为租户 owner 且 `agent_admin=true`；`/api/v1/auth/me` 和 `/api/v1/agent/context` 认证成功，Cookie 为 HttpOnly。清理后用同一测试账号登录返回 401、旧 Cookie 访问 `/api/v1/auth/me` 返回 401。`/api/v1/auth/login-options` 返回 `shared_runtime=true`、`password_login_enabled=true`、`main_site_sso_enabled=true`、`registration_enabled=false`。`VerifyDeployment.ps1` 7 项全部通过：首页/`healthz`/`readyz` 200、未知 Host 421、20 条退役路由 404、入口资源无服务端密钥。最终容器 `running/healthy`、`RestartCount=0`，`/readyz` 返回 `active_tenants=12`。

界面验收：浏览器检查首页、登录页、用户总览、API 密钥、用量、站长仪表盘、系统设置、用户管理、管理用量、支付统计的浅色/深色效果，共 21 张；首页、登录页与控制台共用 `localStorage.theme`，控制台顶部栏主题按钮实测可从浅色切到深色，随后进入 `/keys` 仍保持深色。页面均为暖白/暖黑画布与金色强调，首页独立主题键已移除；截图和四张拼图位于 `.local/agentapi-password-theme-20261027-1625/`。登录页同时展示邮箱密码表单和“使用主站账号 SSO 登录”，关闭注册时不再显示注册入口。

清理：一次性主站账号、自动创建的 Super Key、共享租户及成员、AgentAPI 站点/用户/钱包/支付配置、Session 和 SSO 票据均已定向删除。主站恢复 `users=184`、`api_keys=147`、`agent_shared_tenants=2`、`tenant_members=2`；`vip@qq.com`（ID 187）保持 active。AgentAPI 保持 27 表、75 Session、12 站点、12 用户映射、12 钱包，测试租户和 Session 均为 0，`foreign_key_check` 为空。未删除其他站点或正常用户。

未执行：未取得 `vip@qq.com` 的密码，因此没有用该账号本身完成最终登录；已用同一共享入口和一次性主站账号完成等效真实密码登录验证。未发起充值、支付或收费模型请求；11 个卫星、Cost Console、CRM 与仓库外容器排除。

## 2026-10-27 AgentAPI 公网密码登录与主题一致性部署（17:27）

范围：主站 0 个、卫星 0 个、代理站 1 个（AgentAPI）；重建应用容器 1 个（`agentapi`），依赖容器 0 个。11 个卫星、Cost Console、CRM 和 Nginx 均未操作。主站仅做只读核验与一次性测试账号清理，未重建主站容器。

原因：本地 16:25 版本已经支持共享入口邮箱密码登录并统一白金主题，但公网仍运行 `cc2cx/agentapi:20261026-1338`；公网 `/api/v1/auth/login-options` 返回 404，首页资源为旧 `assets/index-DgT0LQ2U.js`，因此用户看到的仍是旧登录策略和旧主题状态。此次将本地已验证版本重新构建并覆盖公网。

镜像与发布：新镜像 `cc2cx/agentapi:20261027-1727`，镜像 ID `sha256:30fde9cf81afaf775454384000f2a5a0caa515a883a2b37d23f3492e3d49a9ec`，`linux/amd64`，18,825,516 字节；归档 `agentapi-20261027-1727.tar` 18,842,624 字节，SHA-256 `4167be88b02a0456ee7d4a3425c4e6cee7d9a44db69db7c49b221caf7b54c2ab`。新容器 `3868c952a452cd0bfd6b980d175dd16132598bce15d3c2e00fe2c32d975785d6`，入口 `127.0.0.1:18081 -> 8080`，容器状态 `running/healthy`、`RestartCount=0`。新 Compose override 为 `/opt/cc2cx-production/release-20261026/agentapi-login-theme-20261027-1727.override.json`，仅修改镜像标签。

本地同步：同一镜像已标记为 `local-deploy/agentapi:password-theme-20261027-1727` 并更新本地容器 `agentapi-agentapi-1`，容器 ID `11a371d4eb4de85e7466c7be81fa1a2a6438a0a75e7aa9ef086a901d4db55bcb`，入口 `http://localhost:18081`。本地部署前后环境变量新增/删除/修改均为 0，31 项环境、端口、卷、双网络、只读根文件系统和 HostConfig 保持一致；回滚标签为 `local-rollback/agentapi:password-theme-20261027-1727`，指向原 `local-deploy/agentapi:password-theme-20261027-1625`。官方 `VerifyDeployment.ps1` 本地 7 项全部通过。

配置保持：部署前后均为 28 项环境变量，新增、删除、修改均为 0，环境值哈希一致；端口绑定、`agentapi-data -> /app/data` 卷、`sub2api-deploy_sub2api-network` 网络、只读根文件系统、`init`、`cap_drop: ALL`、`no-new-privileges`、tmpfs 和重启策略全部一致。除 `agentapi` 外，其他运行容器均仍为约 2 周启动时间，本次未重启。

备份与回滚：AgentAPI 部署前备份位于 `/opt/cc2cx-production/agentapi-backup-20261027-1727/`，SQLite `agentapi-before-20261027-1727.db` 339,968 字节、SHA-256 `09dae06f6284d4d713f136da72975bf97a241d6ef65c498cd37596187c7548ec`、`integrity_check=ok`；数据卷快照 172,852 字节。旧镜像回滚标签为 `cc2cx-backup/agentapi:20261027-1727`。主站清理前完整备份为 `/opt/sub2api_backup/agentapi-login-test-cleanup-20261027-1727/sub2api-before-cleanup-20261027-1727.dump`，106,584,865 字节，SHA-256 `4060ca01ad8eb5012cdf5d89991adb0ace311f6e6ce76b11fc1a5d599c6d9789`。本次没有数据库结构变更。

回滚命令：

```bash
docker compose -p cc2cx-source \
  -f /opt/cc2cx-production/release-20261026/agentapi.compose.json \
  -f /opt/cc2cx-production/release-20261026/agentapi-logo-base64-20261026-133800.override.json \
  up -d --no-deps --no-build --force-recreate agentapi
```

登录验证：公网 `GET https://agentapi.cc2.cx/api/v1/auth/login-options` 返回 200，数据为 `shared_runtime=true`、`password_login_enabled=true`、`main_site_sso_enabled=true`、`registration_enabled=false`。使用一次性主站账号 ID 365 完成公网注册、AgentAPI 邮箱密码登录和 `/api/v1/auth/me`，登录 200，租户为 `agt_aa12eaf375a449b49b33bfd35cf46094`，角色 owner、`agent_admin=true`、HttpOnly Cookie 1 个。清理后同一账号登录返回 401 `INVALID_CREDENTIALS`，确认账号删除生效。

主题验证：公网首页入口已更新为 `assets/index-B2ccmClT.js` 并引用 `/theme-init.js`；CDP 浏览器实测首页与登录页浅色、深色均正确，点击首页主题按钮后 `theme=dark`，跳转登录页仍保持深色。控制台 toolbar 主题按钮和 `/dashboard` 到 `/keys` 的主题保持由同一构建及本地 21 张截图覆盖。主题统一键为 `localStorage.theme`，`agentapi_landing_theme` 与 `agentapi_theme` 会被迁移后删除；首屏由 `theme-init.js` 预置，避免先浅后深。

清理：公网一次性账号、共享租户、成员关系、自动 API Key（本次为 0）、AgentAPI 站点/用户/钱包/支付配置、Session 和 SSO 票据已定向清理。清理后主站 `users=364`、`agent_shared_tenants=16`、`tenant_members=16`，`vip@qq.com` 保持 active；AgentAPI `active_tenants=16`、58 Session、`integrity_check=ok`、`foreign_key_check` 为空。本地一次性账号 189 与租户 `agt_2f4a4c32aeae49b7b6edd3e06f6e91c2` 也已清理，本地恢复 `users=184`、`tenants=2`、`members=2`、AgentAPI 12 站点、75 Session，`vip@qq.com` 保持 active。

验证与证据：后端 `go test ./... -count=1` 通过；前端 `pnpm typecheck`、`pnpm build` 通过；`pnpm test:run` 为 67 个测试文件、339 项测试全部通过；`pnpm lint:check` 仅保留未修改的 `frontend/scripts/capture-home-preview.mjs` 6 个历史错误。公网浅深截图、CDP 结果、部署日志、备份哈希和清理结果位于 Git 忽略目录 `.local/online-agentapi-login-theme-20261027/`。未取得 `vip@qq.com` 密码，未用该账号本身输入密码；未发起充值、支付或模型请求。

## 2026-10-27 公网 Sub2API 主站覆盖部署

范围：主站 1 个、卫星 0 个、AgentAPI 0 个；只重建 `sub2api` 应用容器 1 个，PostgreSQL、Redis、AgentAPI、11 个卫星及仓库外容器均未重建。公网主站仍使用 Compose 项目 `sub2api-deploy`，依赖容器 `sub2api-postgres` 与 `sub2api-redis` 容器 ID 未变化。

发布产物：镜像 `cc2cx/sub2api:20261027-1815`，镜像 ID `sha256:cc1b0e0b17e1ec1d52273aa0602778b70349018b4504791a38383a3ab6f44582`，`linux/amd64`，46,161,080 字节。本地未压缩镜像归档 46,183,936 字节，SHA-256 `fc661afa35fa03d40a384a3ffcf24eeca66494aa9164e91e2e9c5fc968947b07`，上传后远端校验一致。新容器 ID `710bbe2cbeae081e9b271e49881649ed7c407f3ba93187d0c678c2d271f433ef`，状态 `running/healthy`、`RestartCount=0`，入口保持 `0.0.0.0:8080 -> 8080`。

Compose 文件仍为 `/opt/cc2cx-production/release-20261009/sub2api.compose.json`；本次新增仅修改镜像的 `/opt/cc2cx-production/release-20261009/sub2api-image-20261027-1815.override.json`。部署前后容器环境变量均为 112 项，值哈希均为 `55ce18e184aaeb4b2d588a8ea40c7f0ea5560efd4429d8ffc3b6e3b8d8ee6644`；端口绑定、`/opt/sub2api-deploy/data -> /app/data` 绑定挂载、`sub2api-deploy_sub2api-network` 网络、只读根文件系统设置、`no-new-privileges`、`Init`、`CapDrop`、`Tmpfs` 和重启策略全部一致。运行容器前后快照差异只有 `sub2api` 一行，其他容器 ID 与创建时间均未变化。

后置校验：容器内 `/health`、公网 `https://api.cc2.cx/health`、首页、`/api/v1/settings/public`、`/api/v1/model-plaza` 均返回 200；新首页资源为 `assets/index-jY0nw7ba.js`；最近 10 分钟日志中 panic/fatal 和 error 计数均为 0。公网 AgentAPI `/healthz` 与 `/api/v1/auth/login-options` 仍为 200，登录选项保持 `password_login_enabled=true`、`main_site_sso_enabled=true`、`registration_enabled=false`。`vip@qq.com` 账号保持 ID 358、`active`；主站 `users` 在核对窗口内由 364 增至 366，SQL 确认其中 2 个为部署窗口内的实时新增注册，`agent_shared_tenants=16` 不变。

备份：`/opt/sub2api_backup/sub2api-deploy-20261027-1815/`。PostgreSQL 自定义格式备份 102 MB，SHA-256 `1a068530660adfde51956b6e109032d100f66ae4d60f3ae9b9a805b1d2e6ce7e`；数据目录快照 26 MB，SHA-256 `543a7cb158f4bae3b519da6af991e570881288003f02881daf7c637cc38cc7e3`；部署前 Compose 快照 SHA-256 `f389171d82d521f075ed551ac6eb334e5305e280ee53de5e95808353396fe9ed`。旧镜像回滚标签为 `cc2cx-backup/sub2api:20261027-1815`，镜像 ID `sha256:2fea67e21fc05d8806117d6abd4b2ebb8b76a70fec452d904c08e4d668302ae7`。

回滚命令：

```bash
docker compose -p sub2api-deploy \
  -f /opt/cc2cx-production/release-20261009/sub2api.compose.json \
  -f /opt/cc2cx-production/release-20261009/sub2api-rollback-20261027-1815.override.json \
  up -d --no-deps --no-build --force-recreate sub2api
```

测试与未执行项：后端 `go test ./...` 除 `internal/repository` 的 Windows 无 `sh` 环境用例失败外均通过；前端 `pnpm typecheck` 通过，Docker 内生产构建通过；全量前端测试 2181 项中 2179 项通过，2 项为既有测试期望/测试上下文问题（监控供应商按钮数量仍写死 8、GroupsView 测试缺 Pinia 上下文），本轮未修改。未取得 `vip@qq.com` 密码，因此本次没有直接用该账号输入密码；AgentAPI 邮箱密码登录链路的真实一次性账号验证沿用同日 17:27 发布结果，本次主站发布前后没有修改登录认证代码。未发起充值、支付或收费模型请求。

## 2026-10-27 AgentAPI 邮箱密码登录与白金主题复验

范围：复核公网 AgentAPI 及其依赖的主站登录接口，本轮未重建任何容器；沿用 `cc2cx/agentapi:20261027-1727` 与 `cc2cx/sub2api:20261027-1815`，部署后容器均保持 `healthy`。主站 `https://api.cc2.cx/health` 返回 200，AgentAPI `https://agentapi.cc2.cx/healthz` 返回 200，`/api/v1/auth/login-options` 保持 `shared_runtime=true`、`password_login_enabled=true`、`main_site_sso_enabled=true`、`registration_enabled=false`。

登录链路：新建一次性主站账号 ID 369（`probe-387c9fc9c5f7@example.com`），`POST https://agentapi.cc2.cx/api/v1/auth/login` 返回 200，随后 `GET /api/v1/auth/me` 返回同一用户；租户为 `agt_537f9209b7334fbb949530929e4fc6c1`、owner、`agent_admin=true`，并下发 1 个 `agentapi_session` HttpOnly Cookie。主站日志同一秒出现 `/api/v1/auth/login` 200 与 `/api/v1/auth/integrations/agentapi/start` 200，确认 AgentAPI 使用主站校验结果建立租户会话，而不是在共享入口伪造会话。

`vip@qq.com` 根因复核：主站用户 ID 358 为 `active`，共享租户 `agt_95b3bd2700964a009877ccfea6103451` 为 `active`，成员关系为 owner/active。使用错误密码对照请求时，主站与 AgentAPI 均返回 401 `INVALID_CREDENTIALS`；AgentAPI 不再返回旧的 421 `MAIN_SITE_SSO_REQUIRED`，说明该账号已能进入主站密码校验，旧版“共享入口在认证前直接拒绝”的问题已消除。未取得 `vip@qq.com` 的正确密码，因此没有直接用该账号的正确密码完成最终登录。

主题验证：使用 Playwright + 本机 Chrome 完成 15 项桌面与移动端检查，全部通过。浅色首页、登录页、控制台和 API 密钥页背景为 `rgb(251, 250, 247)`，深色为 `rgb(14, 12, 9)`；首页切深色后进入登录页仍为深色，登录后控制台继承浅色，控制台 toolbar 切深色后进入 `/keys` 仍为深色，再切回浅色并刷新后保持浅色。首页、登录页、控制台 toolbar 的主题按钮均可见，移动端无横向溢出，首页移动菜单和登录页右上角按钮位置正常。唯一持久化键为 `localStorage.theme`，`agentapi_landing_theme` 与 `agentapi_theme` 会迁移并删除，首屏由 `/theme-init.js` 预置。

已知控制台噪声：未登录时共享入口调用 `/api/v1/settings/public`、`/api/v1/agent/context` 会返回 421，`/api/v1/auth/me` 返回 401，这是认证前没有租户上下文时的预期状态；页面错误为 0，登录、品牌和主题均不受影响，本轮未扩大范围修改该接口语义。

清理：一次性账号、共享租户、成员关系、自动 API Key、AgentAPI 站点/用户/钱包/支付配置、9 个 Session 和 SSO 票据已定向清理。清理后主站 `users=367`、`api_keys=12449`、`agent_shared_tenants=16`、`tenant_members=16`；AgentAPI `sessions=58`、`agent_users=31`、`agent_wallets=16`、`agent_config=16`，`integrity_check=ok`、`foreign_key_check` 为空，目标租户残留为 0。`vip@qq.com` 及其租户保持 `active`。AgentAPI 清理前 SQLite 备份为 `/var/lib/docker/volumes/agentapi-data/_data/backups/agentapi-before-probe-cleanup-20261027-1853.db`（364,544 字节）。

证据与未执行项：浏览器断言、HTTP 状态和截图位于 `.local/agentapi-login-theme-recheck-20261027/browser/`，汇总为 `recheck-browser-result.json`；本轮未重新构建或发布镜像，未发起充值、支付或模型请求，未用 `vip@qq.com` 的正确密码登录。

## 2026-10-28 公网 AgentAPI 覆盖部署

范围：仅 AgentAPI 代理站 1 个项目、1 个应用容器；Sub2API 主站 0 个、卫星 0 个、依赖容器 0 个。使用现有 Compose 项目 `cc2cx-source` 只强制重建 `agentapi`，主站、11 个卫星、PostgreSQL、Redis、Nginx 和仓库外容器均未重建。Compose 输出的 orphan 警告只提示同项目存在其他历史容器，本次未使用 `--remove-orphans`。

发布产物：新镜像 `cc2cx/agentapi:20261028-1040`，镜像 ID `sha256:c7126796e7e132d76a7922d200c140cee7034183f53e290f4e4938f7764e6284`，`linux/amd64`，18,825,516 字节；本地归档 `agentapi-20261028-1040.tar` 18,842,624 字节，SHA-256 `c6eb2098f12431f7f349e33576741374845c3dd02f701f15229e5fa5c9ddb029`。上传后远端 SHA-256 与本地一致。新增 Compose override 为 `/opt/cc2cx-production/release-20261026/agentapi-20261028-1040.override.json`，仅修改镜像标签。

部署前测试：`agentapi/backend` 执行 `go test ./... -count=1` 通过；`agentapi/frontend` 执行 `pnpm typecheck` 通过，`pnpm test:run` 为 67 个测试文件、339 项测试全部通过。

容器与配置：容器 ID 由 `3868c952a452...` 变为 `42af654a06f6...`，运行镜像为 `cc2cx/agentapi:20261028-1040`，状态 `running/healthy`、`RestartCount=0`。端口保持 `127.0.0.1:18081 -> 8080`，数据卷保持 `agentapi-data -> /app/data`，网络保持 `sub2api-deploy_sub2api-network`；只读根文件系统、`init`、`cap_drop: ALL`、`no-new-privileges`、tmpfs 和 `unless-stopped` 均未变化。部署前后环境变量均为 29 项，值哈希均为 `72d4de08b5e61ef965bc1d51b73cc33874b06b296aeba26b0cfba437311cbc0a`，新增、删除、修改均为 0。

公网验收：`https://agentapi.cc2.cx/`、`/healthz`、`/readyz`、`/api/v1/auth/login-options` 均返回 200；`/readyz` 报告 `active_tenants=16`，登录选项保持 `password_login_enabled=true`、`main_site_sso_enabled=true`、`registration_enabled=false`。首页资源仍为 `assets/index-B2ccmClT.js`，其 SHA-256 `ec2824a7afa9140de9ef89d7bccddad65e502e4b05d60c614782401901ff8d90` 与本地构建一致；`theme-init.js` 的 SHA-256 `e3ce1654ad51c3a114f5025adeda3bf6b92c5bb126063a8e8f371a4193f7938b` 也一致。`vip@qq.com` 的错误密码对照仍返回 401 `INVALID_CREDENTIALS`，不是旧的 421 拦截。

数据与日志：部署前后 SQLite 均通过 `integrity_check=ok`，`foreign_key_check` 为空，26 张表的逐表行数差异为 0。最近 10 分钟日志无 panic、fatal 或 error。主站 `sub2api` 仍为 `cc2cx/sub2api:20261027-1815`、healthy，其他已核对卫星容器保持原启动时间。

备份与回滚：备份目录 `/opt/cc2cx-production/agentapi-backup-20261028-1040/`，权限 `700`，文件权限 `600`。部署前 SQLite 一致性备份 356 KB、SHA-256 `fb48f60e9c4c4654b91879397876620e40431a2acb055055cb1a7e21870c153d`；数据卷快照 243 KB、SHA-256 `1b83ff395a4590d82022922c23672a9a6b77a8596e732992709b090a3f4cc01a`；另含 Compose、旧 override、部署前后容器快照、运行环境文件和 `SHA256SUMS`。旧镜像已标记为 `cc2cx-backup/agentapi:20261028-1040`，镜像 ID `sha256:30fde9cf81afaf775454384000f2a5a0caa515a883a2b37d23f3492e3d49a9ec`。

回滚命令：

```bash
docker compose -p cc2cx-source \
  -f /opt/cc2cx-production/release-20261026/agentapi.compose.json \
  -f /opt/cc2cx-production/release-20261026/agentapi-login-theme-20261027-1727.override.json \
  up -d --no-deps --no-build --force-recreate agentapi
```

未执行：未发起充值、支付或真实模型请求；未用 `vip@qq.com` 的正确密码登录；未重建主站、卫星或依赖容器。

## 2026-10-28 Sub2API 主页 V2 本地部署（13:01）

范围：Sub2API 主站 1 个项目、1 个应用容器；11 个卫星 0 个、AgentAPI 0 个、Cost Console 0 个、依赖容器 0 个。仅重建 `sub2api`，未重建 PostgreSQL、Redis、Canvas，未使用 `--remove-orphans`。

发布产物：新镜像 `local-deploy/sub2api:home-v2-20261028-125553`，镜像 ID `sha256:f84c1d025952c98c59336b484c6d6d08a0f101ce67a22cc3f0774baceea106db`，大小 46,168,540 字节。该镜像按当前工作区源码构建，包含此前未提交的主站后端改动，本次未回滚这些改动。新容器 ID `4a67d91ff401daa914abfaa5cee66aa1bb28c693c6235974938002d093c8b6ca`。

构建验证：`sub2api/frontend` 执行 `pnpm build` 通过，包含 i18n 完整性检查 3/3、`vue-tsc` 和 Vite 生产构建；Docker 多阶段构建成功。构建上下文为 `C:\Users\kodesh\Desktop\Workspace\sub2api\sub2api`，显式使用当前仓库 `sub2api/Dockerfile`，未使用 Compose 中指向旧目录的 build 配置。

运行验证：容器状态 `running/healthy`、`RestartCount=0`；`http://127.0.0.1:18080/health` 返回 200 和 `{status:ok}`；首页返回 200，入口资源为 `assets/index-Dp2xtKHT.js`，6 个首屏 JS/CSS 资源均返回 200。部署前后环境变量 117 项，值哈希均为 `CF15AC4A2ABE0B4EA16B830AF862765948A49B80B306D875BE40197AF9E2E24E`；端口 `0.0.0.0:18080 -> 8080`、卷 `deploy_sub2api_data -> /app/data`、重启策略 `unless-stopped`、`no-new-privileges:true` 和 `nofile` ulimit 均未变化。近期日志无 panic、fatal 或 error；`sub2api-postgres`、`sub2api-redis`、`canvas-app` 保持原启动时间。

界面验证：Chrome 桌面 1440x1000 截图正常；DevTools 设备模拟 390x844 实测 `innerWidth=390`、`documentElement.scrollWidth=390`，无横向溢出。截图位于 `.local/sub2api-home-v2-20261028-125553/home-desktop.png` 和 `home-mobile-cdp.png`。

备份与回滚：部署前容器快照为 `.local/sub2api-home-v2-20261028-125553/container-before.json`，旧镜像回滚标签为 `local-rollback/sub2api:home-v2-20261028-125553`，镜像 ID `sha256:cd7a68bd33d33389dcfd540de95379869ebec4b7730462f0bee63dbc1ac9222a`。回滚命令：

```powershell
docker compose -p deploy `
  -f '.local\sub2api-oneclick-20261026-1022\sub2api.compose.json' `
  -f '.local\sub2api-home-v2-20261028-125553\sub2api.rollback.json' `
  up -d --no-deps --no-build --force-recreate sub2api
```

未执行：未重建任何卫星、AgentAPI 或依赖容器；未发起真实模型、充值或支付请求；未部署到公网。

## 2026-10-28 AgentAPI 本地部署与支付开关修复（14:04）

范围：AgentAPI 代理站 1 个项目、1 个应用容器；Sub2API 主站 0 个、11 个卫星 0 个、Cost Console 0 个、依赖容器 0 个。仅重建 `agentapi-agentapi-1`，未重建 `sub2api`、卫星、PostgreSQL 或 Redis，数据卷 `agentapi_agentapi-data` 保留。

发布产物：镜像 `local-deploy/agentapi:20261028-141550`，容器 `agentapi-agentapi-1` 为 `running/healthy`、`RestartCount=0`，入口 `0.0.0.0:18081 -> 8080`。本地租户映射保持 `localhost` / `127.0.0.1` -> `agt_ded8719e814142edb8cb9f8885b8ae0c`，站长仍为 `vip@qq.com`（主站用户 187）。

支付问题与修复：复查发现支付宝服务商及其支付方式已保存并启用，但站点收款总开关 `station_payment_settings.enabled` 仍为 0，充值页因此返回空支付方式。现已改为：支付总开关切换后立即保存并显示结果；渠道已配置但未开启时，设置页显示“立即开启收款”；充值页在站长未开放收款时提示“站长尚未开启在线收款”；内置渠道显示名改为“支付宝 / 微信支付 / Stripe / Airwallex”。本地租户的现有支付宝配置已开启，未新增或替换商户凭据。

验收：后端 `go test ./...` 通过；前端 `pnpm typecheck` 通过，全量 67 个测试文件、341 项测试通过（其中支付配置与充值页 21 项）；`VerifyDeployment.ps1` 全部 PASS，包含首页、健康检查、unready、未知 Host 421、20 条退役路由 404 和前端密钥扫描。`/healthz`、`/readyz` 返回 200；充值接口返回 `charging_enabled=true`、`balance_disabled=false`、`methods=alipay`、`display_name=支付宝`，浏览器截图确认白金色充值页显示支付宝。未创建支付订单，未发起真实支付、退款或模型请求。

数据清理：一次性测试账号的主站用户、API Key、认证身份和共享租户成员关系已定向删除；AgentAPI 对应 Session、用户、钱包、注册意向、注册限流和审计记录已删除。保留 `vip@qq.com`、租户 `agent_config` / `agent_wallets`、支付宝服务商及已开启的收款开关。AgentAPI SQLite `integrity_check=ok`，`foreign_key_check` 为空。

备份与回滚：AgentAPI 数据卷备份为 `/app/data/backups/agentapi-qa-20261028-060909.db`（清理前）和 `agentapi-qa-20261028-055136.db`（开启收款前）。旧镜像标记为 `local-rollback/agentapi:before-20261028-141550`；更早镜像为 `local-rollback/agentapi:before-20261028-140243` 和 `local-rollback/agentapi:before-20261028-135422`。回滚命令：

```powershell
docker compose -p agentapi `
  -f '.local\agentapi-deploy-20261026-0953\compose.json' `
  -f '.local\agentapi-deploy-20261028\agentapi-local-20261028-140243.override.json' `
  up -d --no-deps --no-build --force-recreate agentapi
```

未执行：未重建主站、卫星或依赖容器；未部署到公网；未使用真实支付商户完成交易。

## 2026-10-28 Sub2API 首页全屏 3D 粒子与标题修复（14:18）

范围：Sub2API 主站 1 个项目、1 个应用容器；11 个卫星 0 个、AgentAPI 0 个、Cost Console 0 个、依赖容器 0 个。仅重建 `sub2api`，未重建 `sub2api-postgres`、`sub2api-redis`、`canvas-app` 或任何卫星容器；数据卷 `deploy_sub2api_data -> /app/data` 保留。

界面修改：首页新增全屏 `ambient` 3D 粒子层，粒子在整页范围内按 yaw / pitch 旋转、景深缩放、远近透明度和大小变化，并包含粒子连线、拖尾流光、高亮光晕与鼠标视差；右侧原有 `focus` 圆形 HUD 保留位置和尺寸，继续负责 `AIFX / CC2CX / OPENAI / DEEPSEEK` 文字汇聚。文字粒子与环绕粒子分层绘制，并改为按帧时间步长收敛，慢帧下也不会停在过渡形状。同步修复“超参池”标题底部被裁切的问题，调整行高和文本上下留白，保持原布局高度不变。

发布产物：镜像 `local-deploy/sub2api:home-v3-20261028-1415`，镜像 ID `sha256:08e741b9fa97d1115521407ef6d0d9e529319579a064968d017870527d6e3dce`；容器 `sub2api` 为 `running/healthy`、`RestartCount=0`，入口 `0.0.0.0:18080 -> 8080`。

构建与测试：`sub2api/frontend` 执行 `pnpm test:run src/views/__tests__/HomeView.compact.spec.ts`，14/14 通过；`pnpm typecheck` 通过；`pnpm build` 通过，包含 i18n 完整性检查 3/3、`vue-tsc` 与 Vite 生产构建。Docker 多阶段构建成功，构建上下文为 `C:\Users\kodesh\Desktop\Workspace\sub2api\sub2api`，显式使用当前仓库 `sub2api/Dockerfile`，未使用 Compose 中指向旧目录的 build 配置。

运行验证：`http://127.0.0.1:18080/health` 返回 200；`/home` 返回 200，首屏 6 个 JS/CSS 资源均返回 200。Compose 中 117 项环境变量逐项比对全部一致，端口、数据卷、重启策略、`no-new-privileges:true` 和 `nofile` ulimit 未变化；`sub2api-postgres`、`sub2api-redis`、`canvas-app` 保持原启动时间。近期日志无 panic、fatal 或 error。

界面验证：已部署容器在 Chrome 桌面 1440x1000 实测全屏粒子覆盖四角，右侧 `AIFX` 文字汇聚清晰且位置不变，标题笔画完整；DevTools 设备模拟 390x844 实测 `innerWidth=390`、`documentElement.scrollWidth=390`，无横向溢出。截图位于 `.local/sub2api-home-title-fix/deployed-home-desktop.png` 和 `.local/sub2api-home-title-fix/deployed-home-mobile.png`。

备份与回滚：部署前旧镜像回滚标签为 `local-rollback/sub2api:home-v3-20261028-1415`，镜像 ID `sha256:f84c1d025952c98c59336b484c6d6d08a0f101ce67a22cc3f0774baceea106db`。回滚命令：

```powershell
docker compose -p deploy `
  -f '.local\sub2api-oneclick-20261026-1022\sub2api.compose.json' `
  -f '.local\sub2api-home-v3-20261028-1415\sub2api.rollback.json' `
  up -d --no-deps --no-build --force-recreate sub2api
```

未执行：未重建任何卫星、AgentAPI 或依赖容器；未发起真实模型、充值或支付请求；未部署到公网。

## 2026-10-28 公网 Sub2API 与 AgentAPI 覆盖部署（16:35）

范围：主站 1 个项目、2 个应用容器（`sub2api`、公网网关 `sub2api-scheduling-20261009`）；AgentAPI 代理站 1 个项目、1 个应用容器（`agentapi`）；11 个卫星 0 个；依赖容器 0 个重建（`sub2api-postgres`、`sub2api-redis` 容器 ID 与启动时间未变）；Nginx 配置、仓库外容器、`sub2api-cost-console/` 参考项目均排除。

前置核对：公网 `cc2.cx` / `api.cc2.cx` 的 443 入口经 `00-cc2-audit-map.conf` 的 `$cc2_gateway_backend` 指向 `127.0.0.1:28080`，实际由容器 `sub2api-scheduling-20261009` 承载（Compose 项目 `cc2-scheduling`、服务 `gateway`、原镜像 `cc2cx/sub2api:reliability-20261011`）；审计代理 18489 的 `config-audit-candidate.yaml` 同样转发该端口。因此仅更新名为 `sub2api` 的容器不会改变公网页面，本轮把这两个实例都覆盖部署到同一新镜像，网关侧保留原环境变量、端口 `127.0.0.1:28080→8080`、网络 `sub2api-deploy_sub2api-network`、数据目录 `/opt/cc2cx-production/scheduling-20261009/candidate-data` 和回滚标签。

发布产物：`cc2cx/sub2api:20261028-1635`（镜像 ID `sha256:1b56afa68b04f1c9982037fefe34255cb2daf2db65afbfa2add142a79b41b3fc`，amd64，46.2 MB）部署到容器 `sub2api`（`0.0.0.0:8080→8080`）与 `sub2api-scheduling-20261009`（`127.0.0.1:28080→8080`），两者均 `running/healthy`、`RestartCount=0`；`cc2cx/agentapi:20261028-1635`（`sha256:577a252ce5a39292224f360133c6ec43fb60afe530d224ba5db27e8ef644a8d5`，18.8 MB）部署到容器 `agentapi`（`127.0.0.1:18081→8080`），同为 `running/healthy`。

构建与测试：本机构建上下文为仓库内 `sub2api/`、`agentapi/`，`docker build --platform linux/amd64` 均成功（前端 `pnpm build` 与服务端编译在镜像内完成）；`agentapi/backend` 的 `go test ./...` 通过。

部署前备份：`/opt/cc2cx-production/sub2api-backup-20261028-1635`（旧容器、PostgreSQL、Redis 快照，`pg_dump -Fc` 与 `data` 目录压缩包）、`/opt/cc2cx-production/agentapi-backup-20261028-1635`（容器快照、SQLite 逻辑备份与数据卷压缩包）、`/opt/cc2cx-production/gateway-backup-20261028-1635`（网关容器切换前后快照与旧镜像 ID）。旧镜像保留标签 `cc2cx-backup/sub2api:20261028-1635`（原 20261028-1141）、`cc2cx-backup/sub2api:gateway-before-20261028-1635`（原 reliability-20261011）、`cc2cx-backup/agentapi:20261028-1635`（原 20261028-1141）。

逐项核对：`sub2api` 容器环境变量哈希、端口绑定、挂载、网络、重启策略切换前后完全一致（112 项），PostgreSQL/Redis 容器未重建；网关容器 116 项环境变量、端口、挂载、网络、只读配置、Cmd/Entrypoint 全部一致；AgentAPI 13 项检查全部 PASS，环境变量差异仅为 `AGENTAPI_ENTRY_TENANTS`、`AGENT_EMAIL_VERIFY_ENABLED`、`SUB2API_ADMIN_API_KEY` 三项既定注册设置。

功能验证：`https://cc2.cx/` 首页已切换到本次构建（`/assets/index-CaQXMq2X.js`，`HomeView-Buwnrdk5.js` 含 `AI ROUTING CORE`、`SYS.01`、`aurora` 全屏粒子标记），桌面 1440×1000 与移动 390×844 截图正常；`https://api.cc2.cx/health` 200、`/login` 200、`/api/v1/settings/public` 200；`/v1/models` 与 `/v1/chat/completions` 无 Key 返回 401 `API_KEY_REQUIRED`；审计链路 `127.0.0.1:18489/openai/chat/completions` 返回网关 401，网关日志记录到该请求，说明归因代理仍正常转发到新网关；`https://agentapi.cc2.cx/` 首页、`/healthz`、`/api/v1/auth/login-options` 均 200，页面为当前白金主题；`/api/v1/auth/integrations/agentapi/start` 未登录返回 401（预期）。数据库计数 `users=369`、`api_keys=12983`、`agent_shared_tenants=16`，切换前后无丢失。

回滚命令：

```bash
docker compose -p cc2-scheduling \
  -f /opt/cc2cx-production/scheduling-20261009/candidate.compose.json \
  -f /opt/cc2cx-production/scheduling-20261009/gateway-rollback-20261028-1635.override.json \
  up -d --no-deps --no-build --force-recreate gateway

docker compose -p sub2api-deploy \
  -f /opt/cc2cx-production/release-20261009/sub2api.compose.json \
  -f /opt/cc2cx-production/release-20261009/sub2api-image-rollback-20261028-1635.override.json \
  up -d --no-deps --no-build --force-recreate sub2api

docker compose -p cc2cx-source \
  -f /opt/cc2cx-production/release-20261026/agentapi.compose.json \
  -f /opt/cc2cx-production/release-20261026/agentapi-rollback-20261028-1635.override.json \
  up -d --no-deps --no-build --force-recreate agentapi
```

（AgentAPI 回滚覆盖文件 `/opt/cc2cx-production/release-20261026/agentapi-rollback-20261028-1635.override.json` 已生成，镜像解析为 `cc2cx-backup/agentapi:20261028-1635`，租户绑定保持 `agentapi.cc2.cx=agt_95b3bd2700964a009877ccfea6103451`。）

本地证据：`.local/online-deploy-20261028-1635/`（构建日志、镜像 tar、覆盖文件、部署与核对脚本、公网与 AgentAPI 截图）。未执行：未重建 11 个卫星与 PostgreSQL/Redis；未修改 Nginx 配置与审计代理配置；未发起真实模型推理、充值或支付请求；未改动 `sub2api-cost-console/`。
## 2026-10-29 公网 11 个卫星覆盖部署（20261029）

范围：11 个卫星、14 个应用容器，镜像统一 `cc2cx/<service>:20261029`；排除主站 `sub2api`、AgentAPI 代理站 `agentapi` 与参考项目 `sub2api-cost-console/`。`ju-sso-redis` 以及 Livart 的 PostgreSQL、RabbitMQ、MinIO 均未重建。

构建：在线上 `/opt/cc2cx-production/sat-deploy-20261029/src/<slug>` 构建 amd64 镜像。JU 源码包此前漏带 `web/src/pages/admin/logs/logs-page.tsx` 与 `plugin-packages/**/docs/interface.md`，以完整 JU 源码覆盖上传后前后端构建成功；PPT 镜像在本次补建。最终 14 个镜像全部就绪。

部署方式：逐服务执行 `docker compose -p cc2cx-source -f <service>.compose.json -f /opt/cc2cx-production/sat-deploy-20261029/override/<service>.override.json up -d --no-deps --no-build --force-recreate <service>`。覆盖文件只替换镜像；`qrcode` 额外设置 `QR_ART_API_BASE_URL=http://172.18.0.1:18083`。未执行整项目 `up` 或 `down -v`。

备份与回滚：旧镜像保留标签 `cc2cx-backup/<service>:20261009`；原 Compose 与 14 个容器的 `docker inspect` 快照位于 `/opt/cc2cx-production/sat-deploy-20261029/rollback-20261029/`；JU 数据卷在停止旧容器后打包为 `ju-data-20261029.tgz`（84 MB）。回滚时使用同一基础 Compose 加 `cc2cx-backup/*:20261009` 镜像覆盖文件逐服务重建。

验收：14 个容器均 `running`，已配置健康检查的 5 个容器为 `healthy`，`RestartCount=0`；11 个卫星域名首页均返回 200。使用测试账号在真实 Chromium 中打开主站 `auth/integrations/<slug>/start`，11 个卫星全部建立本地 Session（`occ_session`、`aiexcel_session`、`ai3d_session`、`aihuoke_session`、`canvas_session`、`open_ai_canvas_session`、`livart_session`、`presenton_session`、`qrcode_session`、`__Host-screen2code_session`、`yibiao_session`），最终落在 `/`、`/projects`、`/upload` 等正确页面；未出现 `SSO_failed`、`UNAUTHORIZED` 或 `Authorization header is required`；浏览器端 `localhost` / `127.0.0.1` 请求计数为 0，图标均为 `https://api.cc2.cx/logo.jpg` 或线上站点地址。11 个应用余额接口返回 200 且 `recharge_url=https://api.cc2.cx/purchase`（PPT 使用 `/api/v1/sub2api/balance`）。

AI3D：登录态 `/api/3d/health` 返回 `hunyuan.configured=true`、`mode=cloud`、`model=3.0`、`baseUrl=https://api.ai3d.cloud.tencent.com`；`vision.configured=true`、`model=gpt-6-astra`、`baseUrl=http://sub2api:8080/v1`。

二维码：原 `QR_ART_API_BASE_URL` 指向宿主机反代，而该反代只固定一个 SCF 节点，节点在 TLS 握手阶段随机重置，导致 `stylize-qr: fetch failed`。将 `/etc/nginx/conf.d/cc2cx-qrcode-art-proxy.conf` 改为三个 SCF 节点组成的 upstream，并启用 `proxy_next_upstream`（最多 8 次重试），容器继续使用 `172.18.0.1:18083`；原文件备份为 `.bak-20261029`。反代实测 10/10 次 POST、5/5 次 GET 成功；真实 `POST /api/stylize-qr` 返回 200，生成 PNG（data URL 2,268,702 字符，任务耗时 22.4 秒，`cost=1`）。`callbackUrl` 仍为失效 tunnel，但流程使用轮询，未修改冻结的 qrcode 代码。PPT 的 1455 端口沿用原 Compose 发布，应用本身监听 80/3000/8000/8001，`ppt.cc2.cx` 返回 200。

清理：执行 `docker builder prune -af` 与 `docker buildx prune -af`，回收 23.36 GB；构建缓存剩 1.16 GB 且可回收 0 B，根分区由 84% 降至 76%。验收证据保存在 `/opt/cc2cx-production/sat-deploy-20261029/verification-20261029.json`。

未验证：测试账号余额为 0，未执行付费文本、图片、视频模型生成；未做数据库回滚演练；未重建或修改主站与 AgentAPI。
## 2026-10-29 本地 Sub2API 覆盖部署（KeysView 文案）

范围：主站 1 个项目、1 个应用容器（`deploy` / `sub2api`）；未重建 `sub2api-postgres`、`sub2api-redis`、`canvas-app`、AgentAPI 与 11 个卫星。

改动：`sub2api/frontend/src/i18n/locales/zh/dashboard.ts` 的 `keys.desktopDownloads.imageVideo` 由“生图生视频”改为“SKILL”，`ccLaunch` 由“CC-Launch”改为“一键安装skill”；侧边栏 `common.infiniteCanvas` 的“AI生图生视频”保持不变。

构建与部署：`docker build --platform linux/amd64 --build-arg GOPROXY=https://goproxy.cn,direct --build-arg NPM_CONFIG_REGISTRY=https://registry.npmmirror.com -t local-deploy/sub2api:skills-20261029-1449 sub2api` 成功（镜像 ID `sha256:2ba63e878b82f9de16d84882d74e0a85406bcb85ca52c54b4756f9b657178d07`）。使用基础 Compose `.local/sub2api-oneclick-20261026-1022/sub2api.compose.json` 加镜像覆盖 `.local/sub2api-skills-20261029/sub2api.override.json`，执行 `docker compose -p deploy ... up -d --no-deps --no-build --force-recreate sub2api`。容器 `sub2api` 为 `running/healthy`、`RestartCount=0`、`0.0.0.0:18080 -> 8080`，数据卷仍为 `deploy_sub2api_data`。

验证：`http://127.0.0.1:18080/health`、`/`、`/login`、`/keys` 均 200。运行镜像的前端资源 `assets/index-BoEPtrZE.js` 中 `desktopDownloads` 为 `imageVideo:"SKILL"`、`ccLaunch:"一键安装skill"`；侧边栏“AI生图生视频”仍保留。`pnpm test:run src/views/user/__tests__/KeysView.spec.ts` 26/26 通过。

备份与回滚：旧镜像 `local-deploy/sub2api:home-v3-20261028-1415`（`sha256:08e741b9fa97d1115521407ef6d0d9e529319579a064968d017870527d6e3dce`）保留；容器 `docker inspect` 快照 `.local/sub2api-skills-20261029/sub2api-before.json`；数据卷停写备份 `.local/sub2api-skills-20261029/sub2api-data-before.tgz`。回滚覆盖文件为 `.local/sub2api-skills-20261029/sub2api.rollback.json`。

未执行：未登录本地实例做页面点击验证（本地库中不存在线上测试账号）；未部署到公网；未重建依赖容器与其他卫星。