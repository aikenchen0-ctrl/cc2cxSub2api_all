# 工作进度

## 2026-09-02 官方增量迁移

- 已完成官方 Anthropic -> Responses 流式 item 生命周期与多文本 `content_index` 修复。
- 已按 TDD 新增回归测试，确认旧实现先失败后通过。
- 已验证：`go test ./internal/pkg/apicompat -count=1`、`go test ./internal/service -run '^TestDetectModelPlatform$' -count=1 -v`。
- 详细边界、项目清单和后续规则见 `增量更新-260902.md`。
- 已完成 WS v2 ingress 容量降载错误码客户端副本改写，保留原始故障判定；相关服务层定向测试通过。

## 2026-09-01 官方优化增量更新

- 新增 `版本更新-260901.md`，记录官方优化范围、定制功能保护边界和后续版本规则。
- 按 TDD 移植数据库初始化瞬时失败重试：PostgreSQL `57P03`/`08xxx` 指数退避，永久错误立即返回。
- 新增 `backend/migrations/231_add_usage_log_native_compaction_v2.sql`，保持幂等，不改变计费金额。
- 定向测试通过：repository 重试测试、migrations 测试、媒体/协议适配器回归测试。
- 完整 repository 测试在当前 Go 386 环境因既有 `http_upstream.go:720` 未对齐 64 位原子操作 panic，已记录，未修改无关代码。

## 2026-08-31

- 完成 Presenton 启动入口、Compose 端口、环境变量和 Sub2API 相关文件检索。
- 确认 Presenton 已在 `http://localhost:8341` 运行并返回首页及 API 状态 200。
- 下一步：完成两份交接文档并执行内容检查。
- 已生成 `presenton/操作手册.md` 和 `presenton/项目架构.md`。
- 已验证文档代码围栏成对、关键配置项齐全，Presenton `8341` 端口仍返回 HTTP 200。

## 2026-09-01：Sub2API 模型适配方案

- 已读取 `模型适配.md`，确认模型清单、平台标注和默认通用协议规则。
- 已确认本轮边界为方案设计，不修改代码或部署。
- 正在核实现有后端及三个子应用调用链，完成后生成 `模型适配实施方案.md`。
- 已完成后端网关、Grok/OpenAI 图片视频 handler、Superkey 解析以及 Canvas/QRCode/PPT 调用链核查。
- 已生成并检查 `模型适配实施方案.md`：包含能力目录、Provider/Profile、Superkey 动态路由、图片/视频 adapter、任务归属、计费、测试矩阵、分阶段实施、风险和验收标准。
- 初版方案曾将 Kling/Seedance 视为待确认特殊协议，后根据用户补充修正为兼容中转模型。
- 用户补充确认：计算万物本身是 Sub2API 兼容中转站。已将方案修正为：计算万物全部模型统一走 OpenAI-compatible profile；后续只核对其 Base URL、模型映射和能力声明，不再要求 Kling/Seedance 原厂协议取证。
- 已在 `sub2api/backend/internal/service/mediaadapter` 新增独立适配模块与本地 HTTP fixture 测试：覆盖全部图片/视频模型目录、计算万物兼容 Profile、图片生成、视频创建/轮询和上游错误传播。
- `go test ./internal/service/mediaadapter` 已通过。
- 用户要求继续实现，已复核适配器专包测试：4 项全部通过；确认真实上游尚未实调，QA Grok 特殊协议仍由现有 `grok_media` 处理。
- 已读取用户补充的计算万物兼容示例，确认 Chat/Responses 使用 Bearer、Claude 使用 `x-api-key`、Gemini 使用 `x-goog-api-key`；媒体示例仍未提供。
- 已为媒体适配器增加兼容中转常见的 `{"data": {...}}` 响应解包，并新增回归测试；`go test ./internal/service/mediaadapter -count=1` 通过。
- 已读取计算万物兼容调用示例；确认网关通过 `NormalizeInboundEndpoint` 统一识别文本、图片、视频入口，但由不同 handler 分派。
- 已修正 `EditImage`：参考图 data URL 解码为 multipart/form-data，兼容 QRCode/Canvas 图生图；新增测试并通过 `go test ./internal/service/mediaadapter -count=1`。
- 针对视频接入补充复合分组模型识别：`kling-*`、`seedance-*` 归入 OpenAI-compatible 平台，使文档列出的计算万物视频模型可进入现有 `forwardCompatibleVideo` 链路；新增回归用例并通过 `go test ./internal/service -run TestDetectModelPlatform -count=1`。
- 视频适配器增加状态和结果字段归一化：支持 `done/succeeded/processing/pending/canceled` 等兼容中转状态，并提取 `video.url`、`data.video_url`、`data.url`；新增测试，`go test ./internal/service/mediaadapter -count=1` 通过。
- 根据目标“文本、图片、视频自动选择上游并自动转换协议”，新增 `internal/service/protocoladapter` 文本协议转换模块：统一请求结构可生成 OpenAI Chat、Anthropic Messages、Gemini generateContent 请求；测试全部通过。
- 当前未部署；下一步将把 Provider Profile 与现有网关调度、Superkey 模型映射连接起来，再做 Canvas/QRCode/PPT 端到端测试。
- 按“兼容姿态”新增账号显式开关 `extra.media_protocol_profile=openai_compatible`。只有配置该值的账号启用新媒体旁路；缺失、空值和未知值均保持原路由、原接口和原计费行为。
- 相关账号开关、模型识别、媒体适配、文本协议适配测试均通过；未部署。
- Canvas 文本 503 根因已确认：`gpt-5.6-sol` 的多个 OpenAI 上游返回 `INSUFFICIENT_BALANCE`，账号被现有限流/故障转移逻辑摘除，最终无可用账号；不是新适配器或计费改动导致。
- 已构建 `deploy-sub2api:latest` 并重建 `sub2api-dev`。因数据库/Redis 旧容器分属不同 Compose 网络，首次 `up` 出现网络解析失败；已在不触碰数据容器的前提下将应用容器接入 `sub2api-pro_sub2api-network`，当前容器 `running healthy`，`GET http://127.0.0.1:18080/health` 返回 200。
# 2026-09-03 线上构建产物

- QRCode 默认 Base URL 改为 `http://api.cc2.cx`，密钥改为环境变量注入；54 项测试通过。
- 构建成功：`sub2api/sub2api:online-http`、`sub2api/canvas:online-http`、`sub2api/qrcode:online-http`。
- PPT 源码构建因 GitHub 依赖下载网络中断失败；使用已验证的 `presenton-local:latest` 标记为 `sub2api/ppt:online-http` 导出。
- `dist-online/` 已导出四个镜像 tar、Compose、HTTP Nginx 配置、环境变量示例和部署说明。
- Compose config 校验通过；QRCode 容器根路径 HTTP 200。
# 2026-09-03 本地化与 Canvas 配置增强

- Sub2API 快捷工具栏固定为 `http://localhost:3522`、`http://localhost:8341`、`http://localhost:5221`，并传入 Base URL `http://localhost:18080`。
- Canvas、QRCode 默认 Base URL 改为 `http://localhost:18080`；Canvas 渠道编辑器新增“本地网关”一键预设。
- 验证通过：Sub2API 前端构建、Canvas `typecheck`/构建、QRCode 54 项测试。
- 本地镜像成功：`sub2api/sub2api:local-http`、`sub2api/canvas:local-http`、`sub2api/qrcode:local-http`；PPT 使用已验证 `presenton-local:latest` 导出为 `ppt-local-http.tar`。
