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
