# 调研记录

## 已确认

- Presenton 位于 `presenton/`，Docker Compose 服务包括 `production`、`production-gpu`、`development` 和 `development-gpu`。
- Compose 默认宿主机端口为 5001，可用 `PRESENTON_HTTP_HOST_PORT` 覆盖；当前运行容器使用 `8341:80`。
- 容器内由 `start.js` 启动 Nginx、Next.js、FastAPI 和 MCP 服务；主要内部端口为 80、3000、8000、8001，OAuth 回调使用 1455。
- `servers/fastapi/utils/get_env.py` 暴露多种文本 LLM、图片提供商、搜索服务和推理参数环境变量。
- 当前代码检索到图片生成配置，但没有发现文生视频或图生视频 provider 配置入口。
- Sub2API 仓库在同一工作区，但没有发现 Presenton 的现成跳转/API Key 自动填充实现。

## 待写入文档的对接结论

- 推荐先采用 Sub2API 的 OpenAI 兼容网关作为 Presenton 的 `custom` 或兼容 provider 上游。
- “一键跳转”可通过固定 Presenton URL 或带查询参数的中间页实现；当前仓库没有证据表明前端已支持从 URL 自动填充 API Key。
- 不建议把真实 API Key 放进 URL、日志或 Markdown 示例；自动填充应在受控页面完成，并优先使用短期 token/服务端注入。

---

# Sub2API 模型适配调研

## 已确认需求

- 重点适配图片与视频；未特殊说明的模型使用通用 OpenAI-compatible 格式。
- QA API Hub 的 Grok 系列按 Grok 特殊协议处理；`gemini-3.1-flash-image` 标为 OpenAI 格式。
- 一花 Codex 提供 OpenAI Base URL `https://llm.xxttt.com/v1`；密钥仅作为部署配置，不写入源码或方案示例。
- 计算万物列出 GPT Image、Grok Image、Grok Video、Kling、Seedance 模型，但需求文档尚无实际 API 文档链接。
- 用户已确认计算万物本身是 Sub2API 兼容中转站；其列出的模型统一按 OpenAI-compatible 处理，不能按模型名推断原厂 Kling/Seedance/Grok 协议。
- Superkey 不属于单一分组，需要按请求模型和能力跨用户可用分组选择上游。

## 待核实

- 当前图片 handler 的请求归一化、响应输出和模型映射边界。
- Grok 视频任务的内部归属、状态查询、结果下载及计费路径。
- 计算万物兼容端点的实际 Base URL、模型映射和能力声明（属于配置核对，不是原厂协议取证）。
- 分组、账号调度和 Superkey 解析的精确实现。
- Canvas、QRCode、PPT 的实际字段和端点依赖。
