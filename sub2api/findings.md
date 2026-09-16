# sub2api 构建发现

## 当前发现

- 工作区包含 `sub2api`、`infinite-canvas`、`presenton` 三个目录。
- `sub2api` 包含 Go 后端、独立前端、Dockerfile 与多套 Compose 配置。
- 本次只处理 `sub2api`。
- 本机已安装 Docker 29.4.1、Docker Compose 5.1.3、Go 1.27.0、Node 24.15.0、pnpm 10.12.3。
- `deploy/docker-compose.dev.yml` 明确用于从本地源码构建，包含应用、PostgreSQL 18、Redis 8。
- 应用默认映射到 `127.0.0.1:8080`，健康检查路径为 `/health`。
- 开发 Compose 至少要求 `POSTGRES_PASSWORD`，并支持固定 `JWT_SECRET`、`TOTP_ENCRYPTION_KEY` 与管理员密码。
- 根 Dockerfile 是多阶段构建：pnpm 构建前端、Go 构建嵌入式后端、Alpine 运行镜像。
- Docker 守护进程正常。
- 现有 `C:\WorkSpace\sub2api` Compose 项目正在运行，并占用 `127.0.0.1:8080`；不能覆盖。
- 当前开发 Compose 的固定容器名为 `sub2api-dev`、`sub2api-postgres-dev`、`sub2api-redis-dev`，与旧部署不冲突。
- 当前项目将使用端口 18080 和 Compose 项目名 `sub2api-pro`。
- `deploy/.env` 已创建且被 `.gitignore` 排除，包含独立随机凭证。
- Compose 配置静态校验通过，18080 与开发容器名均无冲突。
- 当前源码镜像 `sub2api-pro-sub2api:latest` 构建成功。
- 前端构建存在 Vite 的动态导入和大 chunk 警告，但类型检查与构建成功；不阻塞运行。
- 三个当前项目容器已启动并报告 healthy，`/health` 初次请求返回 200/ok。
- 应用日志有 GitHub API 403 的后台版本同步告警，不影响健康状态。
- 最终核验脚本首次失败于 PowerShell 对 Docker Go 模板的引号传递，不是应用故障。
- JSON 解析修复已单独验证，能够正确读取 Compose 项目标签。
- 第二次核验确认配置、容器、当前构建镜像、健康接口与首页均通过；管理员登录请求未报 HTTP 错误，但顶层字段断言不匹配，需确认响应封装。
- 登录响应确认为 `code/message/data` 封装，管理员登录返回 200，令牌和邮箱验证成功。
- Redis 返回 `PONG`；空 `REDISCLI_AUTH` 仅导致 CLI 警告，取消该空变量后检查输出干净。
- 单体核验脚本再次因 `$home` 与 PowerShell 只读 `$HOME` 冲突而中止；最终核验改为独立短命令。

## 待确认

- 完成源码镜像构建并验证运行状态。
