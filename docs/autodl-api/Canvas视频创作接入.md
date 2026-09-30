# Canvas 视频创作台接入 AutoDL

## 中文展示、英文调用

“AI生图生视频” → “视频创作台”底部“模型”与展开选项显示中文名称。底层选项值、历史配置、请求中的 `model` 以及 AutoDL 创建端点始终保存英文 workflow UUID。完整对应表见 `../卫星公开模型.md`。

托管 Relay 默认补齐 16 个视频工作流；不加入 TTS 音频工作流。前端不请求 AutoDL 获取默认列表，不接触 AutoDL Token 或 SuperKey。前端与网关使用相同的公开参数快照，目录一致性由测试检查。

## 调用链路

1. 浏览器通过 Canvas HttpOnly Session 发起视频任务。
2. Canvas 服务端按公共层协议携带应用凭据、用户身份和卫星标识调用 Sub2API `POST /v1/videos`。
3. Sub2API 按英文 workflow UUID 选择 AutoDL 能力账号。SSO 内部 SuperKey 沿用跨分组调度；任务创建后绑定用户、API Key 和上游账号，查询及下载不换号。
4. 创建请求转换为 `POST /api/v1/comfyui/comfyui_workflow/{workflowUUID}`，上游 `Authorization` 使用服务端保存的原始 ComfyUI Token。
5. 网关返回带 `autodl_` 前缀的任务 ID。Canvas 后台轮询 `/v1/videos/{id}`；网关访问 `/api/v1/comfyui/comfyui_workflow/result/{taskID}` 并规范化状态。
6. 成片地址改写为授权 `/content`，下载 CDN 不携带上游凭据，限制公网 HTTPS 且不跟随重定向。

普通 OpenAI 文本模型通配映射不影响 AutoDL workflow UUID。HTTP 200 中的业务失败会作为错误处理，不伪装成成功任务。

## 参数与素材

- 中文名称来自仓库预设表；输入能力取自 AutoDL 公开工作流规则快照。
- 每个工作流按实际规则显示参考图、音频、视频、首尾帧、时长范围和完整分辨率选项；缺少必填素材、超量素材或非法参数会明确报错。
- 素材沿用 Canvas 原有上传/云存储能力，必须取得 AutoDL 可访问的公开 URL。仅本机或受登录保护的素材地址不能直接用于 AutoDL。
- 动作迁移不发送提示词或时长字段，分辨率发送其原生像素枚举；音频同步工作流使用 `audio_duration`。
- 可指定时长的工作流在创建时保存计费快照；768p、1088p、1440p 保留真实档位，不折算为 480p。
- 动作迁移按成片 MP4 时长向上取整到秒，测量结果写回快照；不套用 Grok 默认 8 秒或 15 秒上限。测量限制 45 秒和 128 MiB，失败不猜测时长、不占用一次性计费标记，后续授权查询可重试。实际单价仍由既有定价配置决定。
- 成功完成后沿用既有一次性计费与持久化去重，重复轮询/下载不会重复计费。

## 账号与验收条件

上游使用已有 OpenAI/API Key 账号，`base_url` 为 AutoDL 官方域名，`api_key` 必须是可调用 ComfyUI 的 Token。运营配置的镜像可用 `extra.video_protocol=autodl` 明确标记。仍需账号启用、可调度、所属分组有效及上游额度/工作流权限；SuperKey 不会把被停用的账号变成可用账号。

部署后只读核对的本地账号 `autodl`（ID 29）存在，但仍为 `schedulable=false`。本次未修改该账号状态或凭据，真实成片尚未验收。

已覆盖部署到本地 `canvas-app`（3522）与 `sub2api`（18080），保留原环境变量和数据卷；两端健康检查及 `/video` 均返回 200。运行时 Relay 已包含 16 个 AutoDL ID，页面实际加载的 JavaScript 资源包含全部 16 个英文 ID 及对应中文名称；未登录视频创建请求在两端均返回 401。当前无可用浏览器自动化会话，未声称完成登录后下拉框的视觉验收。

回滚镜像：`canvas:pre-autodl-20260929-131310`、`sub2api:pre-autodl-20260929-131310`。数据库、Canvas 数据卷、网关配置卷备份及运行配置保存在 `.local/autodl-deploy-20260929-131310/`；该目录含服务端敏感配置，不应提交或公开。需要回滚应用时，在 `sub2api/deploy` 使用现有 Compose 文件，并依次附加该目录的 `runtime.json`、`rollback.json`，执行 `up -d --no-deps --no-build sub2api canvas`，不删除数据卷。

自动化回归覆盖目录一致性、16 个工作流入参、中文显示与英文 ID 分离、SSO 三个头、后台轮询、任务归属、授权下载、跨分组选择、原始 Token 认证、一次性计费与动作迁移时长。模拟上游回归通过不等同于真实 AutoDL 生成验收；发布后需用有效账号验证文生视频、图/音频参考、首尾帧和动作迁移成片。
