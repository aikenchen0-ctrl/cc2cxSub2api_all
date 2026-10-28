# 审计持久化与保留方案

## 目标

- 用户 11 的规范化请求和响应正文永久保存。
- 用户 11 的原始 SSE 调试副本只保留两天。
- 其他用户的元数据和对象均保留两天。
- 异步上传队列和清理器统一使用两天保留期，避免入口提前丢弃待上传记录。
- 元数据先消失时，真实 MinIO 对象仍可被识别和清理。
- 每次清理均可重入，并保留失败任务和永久请求编号。

## 存储层次

1. `permanent_requests` 保存永久请求编号。该表只在 SQLite 锁内更新，不随元数据删除。
2. `request_response_body` 是规范化正文，包含请求和解析后的响应，永久保存。
3. `raw_request_response_body` 是原始 SSE 调试副本，按两天保留，避免永久账号持续写入大对象。
4. `expired_pending` 和 `raw_cleanup_pending` 保存已开始但尚未完成的任务，进程中断后可继续。
5. `cleanup_runs` 保存每次运行的摘要，`retention-status.json` 保存最近一次机器可读状态。
6. `state-backups` 每天生成一次 SQLite 一致性备份，默认保留 14 天。

## 安全边界

- 脚本默认只预检，只有 `--apply` 才执行删除。
- 永久保护由数据库当前记录和 SQLite 永久台账共同决定，任一来源命中都禁止删除。
- MinIO 清理以实际 UUID 目录为准，非法目录、软链接和越界路径一律跳过或中止。
- raw 清理前要求规范化对象同时存在 `xl.meta` 和正文分片。
- 元数据删除按小批次执行，避免 ClickHouse mutation 同时占用过大空间。
- 运行时使用文件锁，定时任务不会并发重叠。

## 上线顺序

```bash
python3 audit_retention.py --mode all
python3 audit_retention.py --apply --mode ledger
python3 audit_retention.py --mode raw --max-raw 1
python3 audit_retention.py --apply --mode raw --max-raw 1
python3 audit_retention.py --apply --mode raw --max-raw 50000
python3 audit_retention.py --apply --mode all
```

前三条均为预检，第四条只处理一条 raw 作为验收样本。确认规范化正文仍可读后，再执行历史 raw 回收，最后启用 systemd 定时任务。

## 校验

- 永久台账条数应覆盖 ClickHouse 和 PostgreSQL 中用户 11 的全部请求编号。
- 清理后的永久请求仍能通过 Audit UI 读取规范化请求和响应。
- 连续执行两次 `--apply`，第二轮不得重复删除或重复累计字节。
- 两天内对象必须保持不变。

## 2026-10-28 上线结果

- 永久台账：40,327 条，与 ClickHouse、PostgreSQL 中的用户 11 请求编号一致。
- 有完整规范化正文的永久记录：34,568 条。
- 仅保留 raw 的永久记录：5,759 条。缺少规范化正文时禁止删除 raw。
- 历史 raw 已删除：34,548 条，逻辑大小 50,626,631,425 字节。
- 两天内 raw 等待到期：20 条。
- 连续复跑清理器时，删除候选和待处理任务均为 0。
- 清理无容器审计构建镜像和过量 journal 后，文件系统可用空间从约 9GB 提升到约 72GB。
