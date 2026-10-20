# cc2cx Helicone 审计链路

cc2cx 自托管的 Helicone 审计项目源码快照。生产链路把 Sub2API 的请求正文与响应正文旁路采集到自建 Helicone（all-in-one）+ MinIO + ClickHouse，用于提示词审计与破甲排查。

导出于 2026-10-21，来源于生产服务器 `38.175.201.140` 的三处目录：

| 本仓库路径 | 服务器原路径 | 说明 |
| --- | --- | --- |
| `gateway/` | `/opt/cc2cx-audit-gateway-src` | Helicone `ai-gateway`（Rust），承担透传 + 审计落盘 |
| `web/` | `/opt/cc2cx-helicone-src` | Helicone `helicone` web / worker（上游原样） |
| `ops/` | `/opt/cc2cx-audit-validation` | 构建、路由同步、保留期、并发回归脚本与排障文档 |

## 上游基线

- `gateway/`：`Helicone/ai-gateway` @ `9649b27`，本地改动 8 个文件（+1143 / −805），完整 diff 见 `gateway/CC2CX_CHANGES.patch`。
- `web/`：`Helicone/helicone` @ `067d929`，工作区干净、无本地改动。
- 运行时镜像：`cc2cx/helicone-passthrough:20261009-eof-v3`（网关）、`helicone/helicone-all-in-one`（审计后端）。

## 部署拓扑

```
客户端 → Nginx (api.cc2.cx / cc2.cx)
        └─ /v1/responses, /v1/chat/completions → 127.0.0.1:18488 (ai-gateway, 审计代理)
                                                  ├─ 原样透传 → 127.0.0.1:8080 (Sub2API)
                                                  └─ 旁路采集 → 127.0.0.1:18585 (Helicone)
                                                                  ├─ ClickHouse  请求索引/属性
                                                                  ├─ PostgreSQL  关联元数据
                                                                  └─ MinIO :9080  请求/响应正文对象
```

- 只采集 `POST /v1/responses` 与 `POST /v1/chat/completions`，WebSocket、其他域名、直连 8080 不在范围内。
- 身份不依赖客户端声称：Nginx 注入独立审计密钥，`helicone-property-sub2api-user-id` 与 `helicone-property-sub2api-api-key-id` 由服务器数据库映射生成，覆盖同名请求头。
- Helicone 顶层 `user_id` 是审计管理员身份，**不能**当作 Sub2API 用户；多轮会话自动聚合尚未实现。

## 网关自定义改动（`gateway/CC2CX_CHANGES.patch`）

1. **持久化 spool 队列**：请求正文先写 `CC2_AUDIT_SPOOL` 指定目录（`0600`、`create_new`、原子 `rename` 提交），再由独立上传进程消费，业务不等待审计上传。
2. **采集与业务解耦**：响应侧队列满或已关闭时记录 `response_queue_full_or_closed` 并跳过本条审计，原始字节流继续转发——故障时优先保业务。
3. **gzip 非流式响应修复**：修复压缩链路重复轮询已结束 unfold 流导致的 `Unfold must not be polled after it returned Poll::Ready(None)`，该缺陷会让 Nginx 返回真实 502（见 `ops/NewAPI测试502-排查与修复.md`）。
4. **MinIO 正文读写**：对象前 32 字节为 erasure header，其后才是 gzip 数据（与 `web/` 的 `s3Client.ts` 对齐），补写失败会重试而非静默丢弃。
5. **日志与鉴权**：审计日志服务改造为可注入/可降级，`forward-client-authorization` 保留原始 Authorization 透传。

## 保留策略（`ops/审计保留策略.md`）

- Sub2API 用户 `11`（`super@sub.com`）：正文与索引不自动过期。
- 其他用户：超过 2×24 小时后由每小时清理任务删除（随机错峰 ≤120s）。
- 三层清理顺序：PostgreSQL 关联数据 → ClickHouse 索引 → MinIO `requests/<uuid>/` 对象；已移除上游默认的三个月正文清空规则。
- 只对已知临时备份设置 2 天清理，不做桶级统一过期，避免误删永久用户数据。

## `ops/` 内容

- `sync_audit_route.py` / `.v2.py` / `.all-users.py`：从 Sub2API 数据库同步用户与密钥映射（约 30s 刷新）。
- `audit_queue_worker.py`、`audit_retention.py`、`retention_sweep.py`：队列上传与保留期清理。
- `provision.py`、`bootstrap.py`、`run_gateway.py`、`start_*.py`、`probe*.py`、`verify_*.py`、`test_*.py`：环境准备、启动与验证脚本。
- `Dockerfile.passthrough`、`Dockerfile.memory-incremental`、`config-*.yaml`：镜像与网关配置样例。
- `cc2-audit-*.service|timer`、`cc2-audit-*.conf`：systemd 与 Nginx 片段。
- `*.patch`：网关改动补丁与 `helicone-audit-consolidated.patch` 合并补丁。
- 文档：`生产接入说明.md`、`审计保留策略.md`、`数百并发审计-HMM改造.md`、`NewAPI测试502-排查与修复.md`、`审计代理异常-HMM分析与修复.md`、`验证结论.md`。

## 复现步骤

```bash
# 1. 构建网关
cd gateway && docker build -t cc2cx/helicone-passthrough:local .

# 2. 起审计后端（all-in-one 自带 ClickHouse + PostgreSQL + MinIO）
docker run -d --name cc2cx-helicone-audit -p 127.0.0.1:18300:3000 \
  -p 127.0.0.1:18585:8585 -p 127.0.0.1:9080:9080 helicone/helicone-all-in-one

# 3. 生成审计凭据并部署路由/保留期定时任务
python3 ops/provision.py
python3 ops/sync_audit_route.py
cp ops/cc2-audit-*.service ops/cc2-audit-*.timer /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now cc2-audit-route.timer cc2-audit-retention.timer

# 4. Nginx 把两个业务路径指向 18488
```

生产参数：16 核 / 16 GiB 主机；`CC2_AUDIT_SPOOL` 指向独立磁盘目录；500 并发突发下业务与审计均完整（见 `ops/数百并发审计-HMM改造.md` 与 `ops/scale-*-results.json`）。

## 未包含内容

以下文件留在服务器，未入库：

- `dashboard-credentials.json`、审计 API key、数据库口令等凭据。
- `retention-state.sqlite3`、`queue/`、`queue-test/` 等运行时状态与待上传正文（含真实用户提示词）。
- `*.log` 构建/运行日志、`*.tar.gz` 中间产物、`production-*` 实时状态文件。

仓库为 public，任何复现都需要自行生成审计密钥，不要提交生产凭据。
