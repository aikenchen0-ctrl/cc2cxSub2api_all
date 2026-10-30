# qrcode 内部原理

> 对象：`qrcode/`（墨莲生成二维码 / QR Code Art Studio）
> 性质：只读逆向梳理，基于 2026-10-24 工作区源码与运行容器
> 状态：**该目录自 2026-09-30 起冻结**，本文档写在 `docs/`，未改动 `qrcode/` 任何文件

---

## 1. 项目定位

Sub2API 的第 11 个卫星应用，独立 Express 进程，无构建步骤（原生 JS + 静态资源），容器 `qrcode`，端口 5221。

产品做一件事：**把普通二维码变成可扫描的艺术二维码，并贴进瓷盘模板做延展。**

边界：

- 只负责二维码业务与本地会话，不持有 SuperKey、不碰计费。
- 调模型时以「当前 Session 用户」身份打 Sub2API `/v1`。
- 一条历史链路（模型 67）直连外部服务，**与 Sub2API 无关**，受冻结规则保护。

---

## 2. 模块地图

```
qrcode/
├── src/
│   ├── server.js             进程入口：createQrApp() + startServer()
│   ├── server-runtime.js     端口占用自动 +1 重试（最多 20 次）
│   ├── qr-app.js             Express 应用：16 个路由、中间件、异步任务队列（1008 行）
│   ├── sso.js                SSO 票据校验、会话 Cookie、三个卫星头、余额地址
│   ├── config.js             APP_CONFIG 全部常量与默认提示词（含模型 67 密钥）
│   ├── qr-generator.js       gpt-image-2 链路：掩膜合成、edits 提交、模块重建
│   ├── mask-processor.js     纯本地图像：白边裁剪、遮罩框计算、盘中合成
│   ├── art-qr.js             模型 67 链路：压缩、提交、轮询、下载
│   ├── url-to-qr.js          URL → 二维码（qrcode 库 + sharp）
│   ├── openai-client.js      /v1/responses 通用调用（当前无路由使用）
│   ├── payload.js            responses 请求体构造，模型 gpt-4.1-mini（当前无路由使用）
│   ├── analyze-chat.js       聊天记录转脚本（当前无路由使用）
│   ├── analyze-chat-file.js  上述的文件入口（当前无路由使用）
│   ├── parser.js             从聊天记录提取图片路径/URL/占位符
│   ├── image-sources.js      本地/远程图片转 dataURL（当前无路由使用）
│   ├── qr-payload.js         edits 请求体构造（当前无路由使用）
│   └── index.js              导出 analyzeChat*
├── public/
│   ├── index.html            单页界面（2231 行 app.js 配套）
│   ├── app.js                全部前端逻辑：裁剪、遮罩、批处理、微信（2231 行）
│   ├── styles.css            样式
│   ├── sub2api-balance-widget.js  余额挂件（Shadow DOM 隔离）
│   ├── blank.jpg             内置空白青花瓷盘模板（614 KB）
│   └── project-icon.jpg      图标
├── tests/                    18 个测试文件，56 项断言
├── Dockerfile                node:24-alpine，npm ci --omit=dev
└── docker-compose.yml        external network deploy_sub2api-network
```

**注意**：`openai-client.js`、`payload.js`、`analyze-chat*.js`、`parser.js`、`image-sources.js`、`qr-payload.js` 这一组共 6 个模块**没有任何路由引用**，只有测试在调，属历史遗留（早期「聊天记录转脚本」功能）。其中 `payload.js` 写死 `gpt-4.1-mini`，与当前公开模型目录不符。

---

## 3. 进程与启动

```
node src/server.js
  ├─ createQrApp()                     qr-app.js
  │    ├─ applyCors(corsOrigin)        QR_CORS_ORIGIN，逗号分隔多域名，* 仅测试
  │    ├─ express.static("public")     静态资源，挂载在路由之前
  │    ├─ express.json()               JSON 体解析
  │    └─ 注册 16 个路由
  └─ startServer({ preferredPort })
       └─ EADDRINUSE 时端口 +1 重试，最多 20 次
```

`server-runtime.js` 的自动换端口在容器里会掩盖端口冲突（容器只映射 5221），排查时需注意日志里的 `端口 x 已被占用，已自动切换到 y`。

---

## 4. 认证与会话

### 4.1 SSO 票据格式

与公共层一致：

```
base64url(JSON payload) . base64url(HMAC-SHA256(payload, SUB2API_SSO_SECRET))
```

`verifyQrcodeSSOTicket()` 逐项校验：

| 校验 | 规则 | 失败信息 |
| --- | --- | --- |
| 结构 | 恰好 2 段，总长 ≤ 16384 | `invalid SSO ticket` |
| 签名 | HMAC-SHA256 + `timingSafeEqual` | `invalid SSO ticket signature` |
| 可解析 | base64url → JSON | `invalid SSO ticket payload` |
| `iss` | 必须等于 `sub2api` | `invalid SSO ticket claims` |
| `aud` | 必须等于 `qrcode` | 同上 |
| `sub` | 必须是纯数字 | 同上 |
| `jti` | 必须存在 | 同上 |
| 时间 | `iat`/`exp` 为安全整数；`exp > now`；`iat ≤ now+30`；`exp-iat ≤ 180` | `expired SSO ticket` |
| 一次性 | `jti` 进入内存 Map，重复即拒 | `SSO ticket already used` |

票据 TTL 由主站签发时决定（默认约 2 分钟），这里只做上限约束 `≤180s`。

### 4.2 会话 Cookie

```
qrcode_session = base64url({sub, exp}) . HMAC-SHA256(·, QR_SESSION_SECRET || SUB2API_SSO_SECRET)
Path=/; HttpOnly; SameSite=Lax; Max-Age=259200
```

- `259200 = 3 × 86400`，符合公共层「卫星 Session 统一 3 天」硬约束。
- `Secure` 仅在 `QR_SECURE_COOKIE=1` 时追加。
- 会话密钥与 SSO 密钥可分离；`QR_SESSION_SECRET` 未设时回退到 `SUB2API_SSO_SECRET`，长度不足 32 字节直接抛错。

### 4.3 两种运行模式

`requireQrcodeUser()` 的判定树：

```
ssoConfigured()?  (SUB2API_SSO_SECRET 长度 ≥ 32)
├─ 否 ─┬─ NODE_ENV=production → 503 "QRCode SSO is not configured"
│      └─ 其他                → 返回 "local"（开发放行）
└─ 是 ─┬─ 无有效 Session → 401 "请先通过 Sub2API 登录"
       └─ 有效           → 返回 userId
```

这个分叉直接决定了测试结果：`NODE_ENV=development` 且未配密钥时 56 项全过；`NODE_ENV=production` 且配了密钥时 9 项因 401 失败——**测试夹具没有签发 Cookie，不是产品缺陷**。

### 4.4 三个卫星请求头

`satelliteHeaders(userId)`：

```http
Authorization: Bearer <SUB2API_APP_CREDENTIAL>
X-Sub2API-On-Behalf-Of: <userId>
X-Sub2API-Satellite: qrcode
```

`qrcodeRequestHeaders()` 在其上加了「托管模式」判定：

- 配了 `SUB2API_APP_CREDENTIAL` 或 `SUB2API_SSO_SECRET` → 视为托管部署，缺头直接抛错，**禁止回退共享 Key**。
- 未托管且 `NODE_ENV=production` → 抛 `QRCode Sub2API satellite is not configured`。
- 未托管且非生产 → 允许空头（本地管线测试用）。

---

## 5. 地址解析：两条互相矛盾的路径

这是当前最关键的实现细节，因为**同一份配置下余额能通、生图不通**。

| 函数 | 取值优先级 | 使用者 |
| --- | --- | --- |
| `sub2apiV1Base()` | `SUB2API_RELAY_BASE_URL` → `LINK` → 补 `/v1` | `/api/sub2api/balance` |
| `APP_CONFIG.apiBaseUrl` | `QR_API_BASE_URL` → `LINK` → 默认 `http://localhost:18080` | `qr-generator.js` 的 edits 提交 |

`qr-app.js` 的 `resolveSessionAiConfig()` 把 `APP_CONFIG.apiBaseUrl` 补成 `…/v1` 后传给生成器。

**后果**：容器内 `LINK=localhost:18080` 时，

- 余额走 `SUB2API_RELAY_BASE_URL=http://sub2api:8080/v1` → 正常；
- 生图走 `http://localhost:18080/v1/images/edits` → 指向容器自身 → `ECONNREFUSED`。

对比 canvas / aiexcel / yibiao 三个卫星（同样是 `LINK=localhost:18080`）不受影响，因为它们统一读 `SUB2API_RELAY_BASE_URL`。**qrcode 是唯一读反了顺序的项目。**

---

## 6. 两条生图链路

### 链路 A：艺术化二维码（模型 67，外部服务）

```
POST /api/stylize-qr   [requireQrcodeUser]
  └─ stylizeQrImage()                         art-qr.js
       ├─ trimQrWhiteBorder(imageBuffer, qrTrim)     按框选坐标裁边
       ├─ buildQrImageBase64()                       jpeg 质量 95→88→80→72，压到 ≤5MB
       ├─ submitArtQrTask()                          最多 3 次，仅对 timeout 类文本重试
       │    POST {artQr.apiBaseUrl}/api/v1/images/generate
       │      { prompt, negative_prompt, model:"67", callback_url, qr_image:<base64> }
       │    成功判据：HTTP ok && code===0 && data.img_uuid
       ├─ pollArtQrResult()                          每 3s，最多 60 次（≈3 分钟）
       │    GET {artQr.apiBaseUrl}/api/v1/images/detail?img_uuid=…
       │    status===1 && urls[0] → 完成；status===-1 → 失败
       ├─ downloadArtQrImage()                       下载结果图
       └─ trimQrWhiteBorder(threshold=245, padding=0) 去输出白边
```

返回结构：`{ imageDataUrl, mimeType, filename, task:{imgUuid,status,duration,cost,url}, input:{…} }`。

提示词是**两套并存**：

- `artQr.defaultPrompt` / `artQr.negativePrompt`：瓷盘+花卉风格，带 `--v 3 --iw 0.95 --shape tiny-plus` 参数（疑似 Midjourney 语法）。
- `artQr.sub2apiPrompt` / `sub2apiNegativePrompt`：**供 `gpt-image-2` 用**，强调「整张二维码重绘、保持模块布局、不要嵌进插图」。`config.js` 有注释明确说明两者必须分开。

`callbackUrl` 指向 `https://shorts-locally-larry-refers.trycloudflare.com/api/v1/callback`，是历史临时隧道；实际完成判定走轮询，该字段目前只是随请求发送。

### 链路 B：上盘延展（gpt-image-2，经 Sub2API）

```
POST /api/preview-mask   （不调 AI，纯本地）
  └─ buildSquareQrPlatePreview()              mask-processor.js
       ├─ 模板小于掩膜框 → 等比放大到刚好容纳
       ├─ computeFixedMaskBox()              绝对坐标，默认 x250 y270 size950
       │   内置空白盘预设 x575 y670 size950（qr-app.js）
       ├─ 二维码 fit:contain 贴入；shape=circle 时用 SVG dest-in 裁圆
       ├─ composedBuffer = 模板 + 二维码
       ├─ debugOverlayBuffer = composedBuffer  ← 刻意与提交图一致，不叠红框
       └─ editMaskBuffer = 中心不透明矩形/圆形，外侧透明
  → 返回 { debugOverlayDataUrl, squareBox, templateSize, qrTrim }

POST /api/generate       [requireQrcodeUser]
  ├─ async=true → 入队，立即返回 202 + jobId
  └─ 同步       → withGenerateTimeout(180s) 后直接返回
       └─ generateQrArtwork()                 qr-generator.js
            ├─ 有模板 → buildSquareQrPlatePreview() 得 context + mask
            ├─ 无模板 → buildSingleContextImage()   主图 96px + 参考图 48px 拼 128px
            │            或 artisticQr → buildArtisticQrCanvas(1024, quietZone 0.08)
            ├─ POST {apiBaseUrl}/v1/images/edits   multipart
            │    model=gpt-image-2, size=1024x1024,
            │    prompt, negative_prompt, image=context.png, mask=mask.png,
            │    input_fidelity=high（仅艺术化模式）
            ├─ 解析 b64_json 或 url（url 则再下载）
            └─ 艺术化模式额外 rebuildArtisticQrFromModules()
```

### 6.1 `rebuildArtisticQrFromModules` 的作用

模型重绘后容易把二维码画糊、对比度降低。这一步用**源二维码自身的模块几何**做掩膜强制修复：

1. 源图与生成图都归一到 1024×1024。
2. 源图转灰度，`< 245` 的像素判为「模块」（含抗锯齿边缘），生成 alpha 掩膜。
3. 生成图 `multiply` 到深蓝底 `rgb(28,96,126)` 上——即使模型输出偏白，模块仍有足够对比度。
4. 用掩膜 `dest-in` 裁出「艺术化模块」。
5. 合成到纯白画布。

效果：模块位置、定位角、编码内容严格来自源图，模型只贡献纹理。

---

## 7. 异步任务队列

`createGenerateJobScheduler(generateJobs, concurrency=1)`：

```
submit → generateJobs.set(id, job)  → queue.push → runNextGenerateJob()
                                                    ↓ activeCount < concurrency
                                              并发执行，完成后再拉下一个
```

- job 结构：`{id: "gen_<uuid>", ownerUserId, status, createdAt, updatedAt, result, error, pipeline}`
- 状态机：`queued → running → succeeded | failed`
- **所有权校验**：`GET /api/generate/jobs/:jobId` 会比对 `job.ownerUserId !== userId` → 404，防止跨用户读取。
- **TTL 30 分钟**：每次创建任务和查询时顺带 `cleanupGenerateJobs()`。
- **进程内存态**：任务表、队列、jti 去重 Map 全部在内存。**重启即丢，多副本不共享。**

前端侧：

- 默认一次生成 3 张（`DEFAULT_GENERATE_COUNT`），上限 12。
- 每张独立提交 `async=true`，`Promise.allSettled` 并行，完成一张显示一张。
- 轮询间隔 2s，单张前端超时 900s，整批 `900s × 张数`。
- 部分失败不影响其余结果，状态栏显示「已完成 x/y 张」。

---

## 8. 前端核心机制

### 8.1 框选裁剪

```
initCropCanvas(file, target)
  ├─ FileReader → Image → 计算 cropDrawBox（等比缩放居中，最大 1000×720）
  ├─ Canvas 按 devicePixelRatio 放大，ctx.scale(dpr,dpr)
  └─ 默认 currentRect = 整图
拖拽 → canvasRectToImageRect()
  ├─ 先夹取到 cropDrawBox 边界
  ├─ 按 scaleX/scaleY 换算回原图坐标
  └─ 提交为 qrCropX/Y/Width/Height + qrTrimMode=manual
```

服务端 `trimQrWhiteBorder` 的 manual 分支会再做一次边界夹取，越界返回 `out-of-bounds` 不报错。

`cropTarget` 在 `normal` / `artistic` 之间切换——艺术化完成后自动切到艺术图继续框选。

### 8.2 遮罩实时预览

`scheduleMaskPreview()` 150ms 防抖 → `previewMaskNow()` → `/api/preview-mask`。移动掩膜（步长 10px）或换模板都会触发。**纯本地合成，不消耗模型额度**，这是设计上的关键优化。

### 8.3 模板加载

`loadDefaultTemplate()` 异步拉 `./blank.jpg`，并在 `await` 前后各检查一次 `templateInput.files`——防止预设覆盖用户刚选的自定义模板。这是并发竞态的显式处理。

### 8.4 微信素材流程

```
getValidWechatAccessToken()   本地缓存，过期前复用
  └─ POST /api/wechat/token → 转发 api.weixin.qq.com/cgi-bin/token
getGeneratedMediaFile()       dataURL → File
  ├─ POST /api/wechat/material/prepare   超 10MB 时 PNG 压缩 + 等比缩到 ≥1024
  └─ POST /api/wechat/material/upload    转发 add_material
查询：POST /api/wechat/material/list → batchget_material
查看：POST /api/wechat/material/get    → get_material
```

服务端对微信错误做了细分诊断（`buildWechatFailurePayload`）：40164/IP 白名单、40013/AppID 无效、40125+40001/凭据错误，各自给出中文处置建议。这是本项目体验最好的一块。

---

## 9. 配置全表

### 9.1 服务端常量（config.js）

| 项 | 值 | 说明 |
| --- | --- | --- |
| `port` | `PORT` 或 5221 | |
| `maxImages` | 6 | |
| `maxInputImageSize` | 128 | 拼贴主图边长 |
| `maxReferenceImages` | 3 | |
| `qrOutputSize` | `1024x1024` | |
| `model` | `QR_IMAGE_MODEL` 或 `gpt-image-2` | |
| `imageEditEndpoint` | `/v1/images/edits` | |
| `taskPollIntervalMs` / `taskPollMaxAttempts` | 2000 / 90 | |
| `generateTimeoutMs` | `QR_GENERATE_TIMEOUT_MS` 或 180000 | |
| `generateConcurrency` | `QR_GENERATE_CONCURRENCY` 或 1 | |
| `artQr.apiBaseUrl` | `QR_ART_API_BASE_URL` 或 `https://open-qr.mewx.art` | |
| `artQr.model` | `67`（写死） | |
| `artQr.maxImageBytes` | 5 MB | |
| `artQr.submitMaxAttempts` / `submitRetryDelayMs` | 3 / 1500 | |
| `artQr.pollIntervalMs` / `pollMaxAttempts` | 3000 / 60 | |

### 9.2 环境变量

| 变量 | 必需 | 作用 |
| --- | --- | --- |
| `PORT` | 否 | 默认 5221 |
| `NODE_ENV` | 是 | `production` 触发托管模式强校验 |
| `LINK` | 是 | 公开网关地址，生成 `/purchase` 充值链接 |
| `SUB2API_RELAY_BASE_URL` | 推荐 | 容器内实际网关地址 |
| `QR_API_BASE_URL` | 否 | 覆盖 `LINK`（**当前是生图失败的直接原因**） |
| `SUB2API_SSO_SECRET` | 是 | ≥32 字节，验票 + 默认会话密钥 |
| `SUB2API_APP_CREDENTIAL` | 是 | 卫星应用凭证 |
| `QR_SESSION_SECRET` | 否 | 与会话密钥分离用 |
| `QR_SECURE_COOKIE` | 否 | `1` 时 Cookie 加 `Secure` |
| `QR_CORS_ORIGIN` | 否 | 逗号分隔多域名 |
| `QR_IMAGE_MODEL` | 否 | 默认 `gpt-image-2` |
| `QR_GENERATE_TIMEOUT_MS` / `QR_GENERATE_CONCURRENCY` | 否 | 默认 180000 / 1 |
| `QR_ART_API_BASE_URL` / `QR_ART_SUBMIT_MAX_ATTEMPTS` / `QR_ART_SUBMIT_RETRY_DELAY_MS` | 否 | 模型 67 链路 |

### 9.3 上传与尺寸限制

| 限制 | 值 | 位置 |
| --- | --- | --- |
| 前端单图校验 | 15 MB | `MAX_UPLOAD_IMAGE_BYTES` |
| multer `/api/generate`、`/api/preview-mask`、`/api/stylize-qr` | **8 MB** | `upload` |
| multer 微信上传 | 10 MB + 字段 64 KB | `wechatMaterialUpload` |
| multer 微信压缩 | 30 MB | `wechatMaterialPrepareUpload` |
| 微信永久素材上限 | 10 MB | `WECHAT_IMAGE_MAX_BYTES` |
| 艺术化上游 jpg | 5 MB | `artQr.maxImageBytes` |

⚠️ **前端 15 MB 与 multer 8 MB 不一致**：8–15 MB 的图片能通过前端校验，但会在服务端被 multer 拒绝，且 `/api/generate` 未挂 multer 错误处理，会落到通用 500。

---

## 10. 错误与超时

| 场景 | 行为 |
| --- | --- |
| 生成超时 | `withGenerateTimeout` 抛「生成超时：单次生图最多等待 180 秒」 |
| 上游 edits 失败 | `GenerationSubmitError` 携带 `submitAttempts`，HTTP 状态透传 |
| 艺术化提交超时 | 仅当响应文本含 timeout 才重试，最多 3 次 |
| 艺术化轮询超时 | 60 次 × 3s ≈ 3 分钟 |
| 余额失败 | 上游 401→401、503→503、其余→502，统一「余额暂时不可用」 |
| SSO 失败 | 浏览器只看到 `SSO login failed`，真实原因写 `console.error` |
| 生产模式 pipeline 精简 | `slimGeneratePipeline` 去掉调试图片数据，只留计数与错误 |

`upstreamStatusFromGenerationError()` 会把上游 4xx/5xx 透传给前端，便于区分「模型拒绝」与「服务端故障」。

---

## 11. 测试结构

18 个文件 / 56 项：

| 分组 | 文件 | 项数 |
| --- | --- | --- |
| 服务端路由 | `qr-server.test.js` | 16 |
| 图像处理 | `art-qr`、`mask-processor`、`qr-generator`、`qr-payload` | 10 |
| 配置与提示词 | `config`、`default-prompt` | 6 |
| 前端行为 | `public-batch-generate`、`public-crop-shape`、`public-download`、`public-generate-guard`、`public-wechat-material` | 15 |
| 遗留链路 | `analyze-chat*`、`parser`、`payload`、`image-sources` | 8 |
| 其他 | `server-runtime` | 1 |

运行方式（不写项目目录）：

```powershell
docker run --rm -e NODE_ENV=development `
  -v "${PWD}/qrcode/tests:/app/tests:ro" -w /app `
  local-deploy/qrcode-qrcode:modelplaza-platinum-20261023 `
  sh -c 'node --test tests/*.test.js'
```

结果：**56/56 通过**。换成 `NODE_ENV=production` + 密钥则 47/56，9 项因缺少 Session 失败（见 §4.3）。

---

## 12. 已知问题

| # | 问题 | 影响 | 修复需改动 |
| --- | --- | --- | --- |
| 1 | 生图读 `APP_CONFIG.apiBaseUrl` 而非 `sub2apiV1Base()`，容器内 `LINK=localhost:18080` → `ECONNREFUSED` | **`gpt-image-2` 扩图全链路不可用** | 容器环境变量，或 `config.js`/`qr-generator.js` |
| 2 | 微信 5 个接口未挂 `requireQrcodeUser` | 任何人可借服务端出口 IP 调微信 API | `qr-app.js` |
| 3 | 前端 15 MB vs multer 8 MB | 8–15 MB 图片静默 500 | 任一侧对齐 |
| 4 | `artQr.callbackUrl` 指向失效的 trycloudflare 隧道 | 若上游依赖回调则任务不结束（当前走轮询，未触发） | `config.js` |
| 5 | 6 个模块无路由引用（含 `gpt-4.1-mini`） | 死代码，模型名与公开目录不符 | `payload.js` 等 |
| 6 | 任务表 / jti 去重 / 队列全在内存 | 重启丢失；多副本不共享 | 架构级 |
| 7 | `artQr.apiKey` 硬编码在 `config.js` | 明文密钥进仓库 | 冻结规则明确禁止迁移 |
| 8 | 容器无 healthcheck | 进程假死无法自动重启 | `docker-compose.yml` |

第 1、7 项涉及冻结规则，**必须由用户明确解除保护并指定文件**才能处理。

---

## 13. 冻结边界

`qrcode/AGENTS.md` 规定：

- 禁止修改该目录任何代码、配置、测试、提示词、页面、部署文件。
- 禁止改动艺术化链路（URL、模型 67、`config.js` 密钥、`stylizeQrImage` 默认调用）。
- 禁止把旧链路替换为 `gpt-image-2` / `/v1/images/edits`。
- 允许只读检查、测试、诊断，但不得写入或生成项目文件。
- 仅当用户明确说「解除 qrcode 冻结保护」或点名具体文件与内容时才可修改；即使解除，也须先读根 `AGENTS.md`、`docs/卫星应用接入公共层.md`、`docs/卫星公开模型.md`，说明改动文件与原因，只做最小修改，改后回归并确认旧链路未被替换。

本文档因此写在 `docs/`，未进入冻结目录。

---

## 14. 一句话总结

qrcode 是「本地图像处理 + 两次模型调用」的组合：先用 sharp 把二维码精确放进瓷盘并生成掩膜（全本地、零成本、可预览），再把「上下文图 + 掩膜 + 中文提示词」交给 `gpt-image-2` 做一次 edits；艺术化按钮则走另一条直连外部服务的旧链路（模型 67）。身份靠 3 天 HttpOnly Session，模型调用靠三个卫星头，SuperKey 全程不出现。当前部署下**余额链路正常、`gpt-image-2` 生图链路因地址解析顺序错误而中断**。
