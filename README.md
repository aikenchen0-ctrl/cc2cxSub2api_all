# sub2api-pro

这是一个以 `sub2api` 为主项目的开发工作区，包含 API 网关源码、前端管理界面、部署配置，以及若干设计、测试和辅助工具目录。

## 主项目

`sub2api/` 是一个基于 Go + Vue 3 的 AI API 网关与订阅配额管理平台，支持统一接入多种模型服务、渠道路由、用量统计、用户与密钥管理等能力。

- [主项目中文文档](sub2api/README_CN.md)
- [主项目英文文档](sub2api/README.md)
- [开发指南](sub2api/DEV_GUIDE.md)
- [产品说明](sub2api/PRODUCT.md)
- [部署目录](sub2api/deploy/)
- [许可证](sub2api/LICENSE)

## 快速启动（Docker Compose）

环境要求：Docker 24+ 与 Docker Compose v2。

```bash
cd sub2api/deploy
cp .env.example .env
# 编辑 .env，至少设置数据库密码和服务端口
docker compose up -d
docker compose ps
```

启动后访问 `http://localhost:8080`。首次启动请根据页面向导完成数据库、Redis 和管理员账号配置。生产环境请使用强随机密码，并通过 HTTPS 或反向代理暴露服务。

停止服务：

```bash
cd sub2api/deploy
docker compose down
```

## 本地开发

后端依赖 Go 1.21+、PostgreSQL 15+ 和 Redis 7+；前端依赖 Node.js 18+ 与 pnpm。

```bash
# 前端
cd sub2api/frontend
pnpm install
pnpm run dev

# 后端（另开终端）
cd sub2api/backend
go run ./cmd/server
```

常用检查命令：

```bash
cd sub2api
make build     # 构建前后端
make test      # 运行后端与前端测试
```

## 工作区目录

| 目录 | 用途 |
| --- | --- |
| `sub2api/` | 主项目源码、部署文件和正式文档 |
| `canvas/` | 画布/交互相关原型与说明 |
| `home/` | 首页或视觉素材实验 |
| `ppt/` | 演示文稿与相关测试 |
| `qrcode/` | 二维码工具与素材 |

## 文档索引

根目录中的中文 Markdown 文件记录了模型适配、数据导入和阶段性实施方案；主项目的详细专题文档位于 `sub2api/docs/`。涉及支付、合规、插件和安全配置时，请优先阅读对应专题文档。

## 安全与合规

请勿将真实密钥、密码、生产数据或本地配置提交到 Git。部署前检查 `sub2api/deploy/.env.example`，为 JWT、TOTP 和数据库设置独立的强随机值，并遵守所接入模型服务商的服务条款及当地法律法规。

## 贡献

提交改动前建议运行 `make test` 和 `make build`，并在 Pull Request 中说明影响范围、配置变更及验证方式。

## 许可证

主项目遵循 [GNU AGPL v3](sub2api/LICENSE)。工作区内其他目录如有单独许可证或说明，以其目录中的文件为准。
