# Docker 本地部署

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

## 启动和检查

在仓库根目录执行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/Start-LocalDocker.ps1
docker ps --format "{{.Names}}\t{{.Status}}\t{{.Ports}}"
docker logs --tail 50 sub2api
```

脚本用于启动本机现有部署，启动 Docker Desktop、依赖和应用，并检查 12 个首页与容器健康状态。它不会删除或重建容器，不适用于尚无镜像和容器的新机器。各容器使用 `unless-stopped`；Docker 引擎须保持运行。

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
