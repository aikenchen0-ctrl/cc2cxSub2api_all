# AI大模型汇总

本文整理当前影策仓库中可核对的模型目录、调用合同和名称映射，按文本、图片、音频、视频分类。英文名称列保留实际模型标识的大小写、版本和路径前缀；即梦 CLI 的图片标识为版本号。

名称状态说明：**已有映射**表示源码明确配置了用户显示名与调用标识；**未配置中文名**表示保留原名；**说明性中文名**仅用于本文阅读，不代表界面已经使用该名称。本次只整理文档，不修改模型配置。

模型目录会随管理员配置及上游 `/models` 返回结果变化，本文不是所有服务商模型的全集，也不代表所有条目已启用或通过真实生成验证。只读核对本机影策部署时，系统渠道模型表仅有已启用的 `gpt-5.5`，显示名和调用名相同；其他后台或个人渠道设置的历史中文别名未在该表中得到确认。

## 文本

| 中文名称／用户显示名称 | 英文名称／实际调用标识 | 名称状态 | 说明 |
| --- | --- | --- | --- |
| gpt-5.5 | `gpt-5.5` | 未配置中文名 | Sub2API 接入默认文本模型；本机系统渠道已配置 |

文本模型可从渠道动态拉取，仓库没有固定的完整文本模型清单，因此不把任意 Claude、Gemini、千问等版本推定为当前已配置模型。

## 图片

| 中文名称／用户显示名称 | 英文名称／实际调用标识 | 名称状态 | 来源与说明 |
| --- | --- | --- | --- |
| 通义千问图像 3.0 Pro | `qwen-image-3.0-pro` | 已有映射 | 百炼补充模型目录 |
| 万相图像 2.7 Pro | `wan2.7-image-pro` | 已有映射 | 百炼补充模型目录 |
| gpt-image-2 | `gpt-image-2` | 未配置中文名 | 前端历史默认模型标识，不代表当前已启用 |
| gpt-image-1 | `gpt-image-1` | 未配置中文名 | Sub2API 接入测试中的图片模型示例 |
| grok-imagine-image | `grok-imagine-image` | 未配置中文名 | Sub2API 接入测试中的图片模型示例 |
| 即梦图片 3.0 | `3.0` | 说明性中文名 | 官方即梦 CLI，支持文生图 |
| 即梦图片 3.1 | `3.1` | 说明性中文名 | 官方即梦 CLI，支持文生图 |
| 即梦图片 4.0 | `4.0` | 说明性中文名 | 官方即梦 CLI，支持文生图、图生图 |
| 即梦图片 4.1 | `4.1` | 说明性中文名 | 官方即梦 CLI，支持文生图、图生图 |
| 即梦图片 4.5 | `4.5` | 说明性中文名 | 官方即梦 CLI，支持文生图、图生图 |
| 即梦图片 4.6 | `4.6` | 说明性中文名 | 官方即梦 CLI，支持文生图、图生图 |
| 即梦图片 4.7 | `4.7` | 说明性中文名 | 官方即梦 CLI，支持文生图、图生图 |
| 即梦图片 5.0 | `5.0` | 说明性中文名 | 官方即梦 CLI，支持文生图、图生图 |
| 即梦图片 5.0 Pro | `5.0Pro` | 说明性中文名 | 官方即梦 CLI，支持文生图、图生图 |

即梦 CLI 目录当前以版本号作为显示名，提交参数为 `modelVersion`；不能直接把本文中文说明或其他供应商的 Seedream 模型 ID 填入该参数。

## 音频

| 中文名称／用户显示名称 | 英文名称／实际调用标识 | 名称状态 | 说明 |
| --- | --- | --- | --- |
| gpt-4o-mini-tts | `gpt-4o-mini-tts` | 未配置中文名 | 前端历史默认音频模型标识，官方协议测试也使用该标识；用于语音合成 |

音频服务使用所选渠道配置的模型名。当前核对范围内没有固定的中文音频模型映射清单；支持音频协议不等于已经配置了对应模型。

## 视频

| 中文名称／用户显示名称 | 英文名称／实际调用标识 | 名称状态 | 来源与用途 |
| --- | --- | --- | --- |
| HappyHorse 1.1 文生视频 | `happyhorse-1.1-t2v` | 已有映射 | 百炼，文生视频 |
| HappyHorse 1.1 图生视频 | `happyhorse-1.1-i2v` | 已有映射 | 百炼，图生视频 |
| HappyHorse 1.1 参考视频生成 | `happyhorse-1.1-r2v` | 已有映射 | 百炼，参考生成 |
| HappyHorse 1.0 视频编辑 | `happyhorse-1.0-video-edit` | 已有映射 | 百炼，视频编辑 |
| 万相 2.7 文生视频 | `wan2.7-t2v-2026-06-12` | 已有映射 | 百炼，文生视频 |
| 万相 2.7 图生视频 | `wan2.7-i2v-2026-04-25` | 已有映射 | 百炼，图生视频 |
| 万相 2.7 参考视频生成 | `wan2.7-r2v-2026-06-12` | 已有映射 | 百炼，参考生成 |
| 可灵 V3 Omni 视频生成 | `kling/kling-v3-omni-video-generation` | 已有映射 | 百炼，文生、图生、参考生成、视频编辑 |
| 可灵 V3 视频生成 | `kling/kling-v3-video-generation` | 已有映射 | 百炼，文生、图生视频 |
| 爱诗 C1 文生视频 | `pixverse/pixverse-c1-t2v` | 已有映射 | 百炼，文生视频 |
| 爱诗 V6 文生视频 | `pixverse/pixverse-v6-t2v` | 已有映射 | 百炼，文生视频 |
| 爱诗 V5.6 文生视频 | `pixverse/pixverse-v5.6-t2v` | 已有映射 | 百炼，目录建议升级到 V6 |
| 爱诗 C1 图生视频 | `pixverse/pixverse-c1-it2v` | 已有映射 | 百炼，图生视频 |
| 爱诗 V6 图生视频 | `pixverse/pixverse-v6-it2v` | 已有映射 | 百炼，图生视频 |
| 爱诗 V5.6 图生视频 | `pixverse/pixverse-v5.6-it2v` | 已有映射 | 百炼，图生视频 |
| Vidu Q3 Pro 文生视频 | `vidu/viduq3-pro_text2video` | 已有映射 | 百炼，文生视频 |
| Vidu Q3 Turbo 文生视频 | `vidu/viduq3-turbo_text2video` | 已有映射 | 百炼，文生视频 |
| Vidu Q2 文生视频 | `vidu/viduq2_text2video` | 已有映射 | 百炼，文生视频 |
| MiniMax H3 文生视频 | `minimax_h3_b99_001` | 说明性中文名 | Sub2API 视频合同，无参考图 |
| MiniMax H3 首尾帧视频 | `minimax_h3_b99_002` | 说明性中文名 | Sub2API 视频合同，首尾帧各一张 |
| MiniMax H3 多图参考视频 | `minimax_h3_b99_003_12s` | 说明性中文名 | Sub2API 视频合同，1 至 9 张参考图 |
| grok-imagine-video | `grok-imagine-video` | 未配置中文名 | 前端历史默认模型标识，不代表当前已启用 |
| 即梦视频 Seedance 1.0 Fast | `seedance1.0fast` | 说明性中文名 | 官方即梦 CLI，图生视频 |
| 即梦视频 Seedance 1.5 Pro | `seedance1.5pro` | 说明性中文名 | 官方即梦 CLI，图生、首尾帧视频 |
| 即梦视频 Seedance 2.0 | `seedance2.0` | 说明性中文名 | 官方即梦 CLI |
| 即梦视频 Seedance 2.0 Fast | `seedance2.0fast` | 说明性中文名 | 官方即梦 CLI |
| 即梦视频 Seedance 2.0 VIP | `seedance2.0_vip` | 说明性中文名 | 官方即梦 CLI |
| 即梦视频 Seedance 2.0 Fast VIP | `seedance2.0fast_vip` | 说明性中文名 | 官方即梦 CLI |
| 即梦视频 Seedance 2.0 Mini | `seedance2.0mini` | 说明性中文名 | 官方即梦 CLI |
| 即梦视频 Seedance 2.5 | `seedance2.5` | 说明性中文名 | 官方即梦 CLI |

Sub2API 文生视频还接受 `minimax`、`minimax-h3`、`minimax_h3` 三个英文别名，它们不是三个独立模型。CLI Seedance 2.x 的具体输入、分辨率和时长约束以执行合同及账户权限为准；目录存在不代表账户拥有调用权限。

---

**映射规则与核对依据**

用户看到的名称来自 `displayName`；渠道模型的 `modelKey` 标识可选模型，`providerModelKey` 可指定实际上游模型名。三者应分开维护，不能通过翻译显示名称推导调用标识。前台逻辑模型还可能通过路由选择不同渠道模型。

- [百炼补充模型目录](backend/internal/provider/bailian/discovery.go)：20 条明确的显示名称与英文 ID 对照。
- [模型选择显示名](web/src/lib/model-selection.ts)、[系统渠道模型目录](backend/internal/app/model_catalog.go)：显示名称与模型标识的读取规则。
- [Sub2API 接入配置](backend/internal/app/sub2api_integration.go)、[视频模型合同](backend/internal/app/sub2api_capability.go)、[接入测试](backend/internal/app/sub2api_integration_test.go)：默认文本模型、视频标识及图片示例。
- [前端模型配置](web/src/stores/use-config-store.ts)：历史默认模型标识；这些值不等于当前启用清单。
- [即梦 CLI 执行合同](canvas-agent/src/dreamina-cli-contract.ts)、[即梦目录投影](canvas-agent/src/dreamina-model-catalog.ts)：图片版本、视频版本与实际显示规则。
- [音频协议测试](backend/internal/protocol/official_catalog_test.go)：语音合成模型示例。

本次仅进行源码、配置记录和文档核对，未发起付费模型请求，未运行构建或测试。
