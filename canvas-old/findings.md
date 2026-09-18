# 调研发现

- Web 前端使用 Vite + React + TypeScript，主要代码位于 `web/src`。
- AI 配置由 Zustand persist 保存到浏览器本地存储，键名为 `infinite-canvas:ai_config_store`。
- 配置支持 OpenAI 与 Gemini 两种请求格式；每个模型绑定一个 channel，channel 包含 Base URL、API Key、格式和模型能力。
- 当前模型能力枚举为 `text`、`image`、`video`、`audio`；图片/视频/音频调用均由浏览器直接请求用户配置的 Base URL。
- 已确认端口 8241 已用于 Vite 开发启动和容器运行配置。
- `web/src/components/layout/client-root-init.tsx` 支持从当前页面查询参数读取 `baseUrl`/`baseurl` 与 `apiKey`/`apikey`，写入首个 channel 及旧版顶层配置，随后移除参数并打开配置面板。
- OpenAI 格式原生调用：文本 `/v1/responses`，生图 `/v1/images/generations`，改图 `/v1/images/edits`，视频 `/v1/videos`（创建、轮询、下载），音频 `/v1/audio/speech`。
- Gemini 格式原生支持文本和图片 `generateContent`；视频和音频原生路径会明确拒绝，需使用模型脚本插件。
- 视频参考图最多取前 7 张，图生视频通过视频请求的 `input_reference[]` 上传；脚本模式通过 `images` data URL 数组传入。
- 模型列表按 OpenAI `/v1/models` 或 Gemini `/v1beta/models` 读取，能力默认为按模型名猜测，但可在渠道编辑器中手工覆盖。
- API Key 保存在浏览器 Zustand 持久化状态，并由浏览器直接发送到 Base URL；项目没有后端代理或服务端密钥托管。
