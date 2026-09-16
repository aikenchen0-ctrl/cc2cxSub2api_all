# Canvas 与 Sub2API 纯 HTTP 部署说明

## 文档状态

本文件已废弃。生产环境必须使用 HTTPS；请参阅 `docs/云端部署约定.md` 和 `docs/云端部署包说明-20260906.md`。

## 部署目标（历史）

- Canvas：`https://canvas.cc2.cx`，由 Nginx 提供静态文件。
- Sub2API：`http://api.cc2.cx`，Nginx 反向代理到本机 `127.0.0.1:18080`。
- 历史版本不配置证书；生产部署不得沿用此配置。

## 1. 准备文件

将以下文件上传到 Linux 服务器同一目录：

- `sub2api-latest.tar`
- `canvas-dist.zip`
- `nginx-canvas-api.conf`

## 2. 启动 Sub2API

导入镜像（镜像内已包含 `weishaw/sub2api:latest` 标签）：

```bash
docker load -i sub2api-latest.tar
```

进入项目的 `deploy` 目录，创建 `.env`（至少设置数据库密码）：

```bash
cd /opt/sub2api/deploy
cp .env.example .env
nano .env
```

确认以下配置适用于纯 HTTP：

```dotenv
POSTGRES_PASSWORD=请替换为强密码
CORS_ALLOWED_ORIGINS=https://canvas.cc2.cx,http://localhost:3522,http://127.0.0.1:3522
SERVER_PORT=18080
BIND_HOST=127.0.0.1
```

启动并检查：

```bash
docker compose up -d
docker compose ps
curl -i http://127.0.0.1:18080/health
```

若 Compose 文件仍使用远程镜像名，`docker load` 导入的同名标签会被直接使用；无需访问 Docker Hub。

## 3. 发布 Canvas 静态文件

```bash
mkdir -p /var/www/canvas
unzip -o canvas-dist.zip -d /var/www/canvas
```

压缩包内若包含顶层 `dist` 目录，请使用下面命令整理到 Nginx 根目录：

```bash
if [ -d /var/www/canvas/dist ]; then
  cp -a /var/www/canvas/dist/. /var/www/canvas/
  rm -rf /var/www/canvas/dist
fi
```

## 4. 配置 Nginx

```bash
cp nginx-canvas-api.conf /etc/nginx/conf.d/canvas-api.conf
nginx -t
systemctl reload nginx
```

确保 DNS 已将 `canvas.cc2.cx` 和 `api.cc2.cx` 都解析到此服务器，并开放 TCP 80 端口。

## 5. 验证跨域与访问

```bash
curl -i -X OPTIONS http://api.cc2.cx/v1/responses \
  -H 'Origin: http://canvas.cc2.cx' \
  -H 'Access-Control-Request-Method: POST' \
  -H 'Access-Control-Request-Headers: authorization,content-type'
```

预期状态为 `204`，并包含 `Access-Control-Allow-Origin: *`。

浏览器访问 `https://canvas.cc2.cx`，Canvas 的 API Base URL 使用 `https://api.cc2.cx`。

## 交付文件校验

```bash
sha256sum sub2api-latest.tar canvas-dist.zip nginx-canvas-api.conf
```
