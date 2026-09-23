# BidMonitor 内部服务

此目录承载从 `BidMonitor-AI` 搬运的采集、匹配、通知和调度模块，并由融合项目的 Python 内部服务统一调度。

运行入口是 `service/app.py`。浏览器和外部反向代理不应直接访问此服务；它只接受 OpenBidKit Fastify 发出的内部请求，并使用 `X-BidMonitor-Service-Token` 校验调用方。

用户数据按 `BID_MONITOR_DATA_ROOT/<user_id>/` 隔离。真实数据目录、服务令牌、通知凭据和模型密钥不能提交到 Git。

来源项目：

- https://github.com/zhiqianzheng/BidMonitor-AI
- 搬运基线：`63a0e13`（`v1.7`）

模块边界、启动方式和 OpenBidKit 接口见 `../docs/yibiao-bidmonitor-sub2api-integration.md`。
