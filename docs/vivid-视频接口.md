









# 接口文档
OpenAI 兼容接口 + 旧项目兼容入口 — 改个 `base_url` 和 `api_key` 即可调用。图像 / 视频 / 图生图 / 商品套图均有对应接口，具体差异见下方说明。

## 基础信息
Base URL

https://aigc.easysu.cn/v1

鉴权

Authorization: Bearer YOUR_API_KEY

你的 Key

YOUR_API_KEY

标准 `/v1` 只接受用户 API Key；`/api/v1/open` 的生成和余额接口还支持 account/password。还没有 Key?去 [设置 → API Key](https://aigc.easysu.cn/settings) 生成。

## 端点
+ GET `/v1/models`
+ GET `/v1/user/balance`（查余额）
+ POST `/v1/images/generations`（文生图）
+ POST `/v1/images/edits`（图生图，multipart）
+ POST `/v1/videos`（创建视频任务）
+ GET `/v1/videos/{id}`（查状态）
+ GET `/v1/videos/{id}/content`（下载 mp4）
+ POST `/v1/product-image-suites/analyze`（商品分析）
+ POST `/v1/product-image-suites`（商品套图）
+ GET `/v1/product-image-suites/{id}`（套图状态）
+ POST `/api/v1/open/generations`（旧项目兼容任务入口）

## 可用模型
| **model** | **类型** | **定价 / 积分** | **能力** |
| --- | --- | --- | --- |
| seedance2.0-900-720p | 视频 | 720p **20**（10s **+10**，15s **+10**） | 720p；10s–15s；参考图 9；16×9；9×16 |
| gpt-image-2 | 图像 | 1K **0.6**；2K **0.6**；4K **1.2** | 1K；2K；4K；参考图 1；1×1；16×9；9×16；4×3；3×4 |
| minimax-h3-933-图文 | 视频 | 768p **1**（5s **+8**，10s **+18**，15s **+28**） | 768p；5s–15s；参考图 9；音频 3；16×9；9×16 |
| sd-2.5-30秒 | 视频 | 720p **1**（30s **+29**） | 720p；30s；参考图 9；16×9；9×16 |
| gpt-image-2-原生 | 图像 | 1K**0.8**2K**1.2**4K**1.4** | 1K2K4K参考图 61×116×99×164×33×4 |
| minimax-h3 | 视频 | 768p**2**2K**4**5s**+8**10s**+16**15s**+20** | 768p2K5s–15s参考图 9音频 316×99×16 |
| grok-video-1.5 | 视频 | 720p**1**5s**+70**6s**+72**7s**+74**8s**+76**9s**+78**10s**+80**11s**+82**12s**+84**13s**+86**14s**+88**15s**+90** | 720p5s–15s参考图 916×99×16 |
| seedance2.5-9图 | 视频 | 720p**10**30s**+40** | 720p30s参考图 916×99×16 |
| nano-banana-pro | 图像 | 1K**0.6**2K**0.6**4K**1.2** | 1K2K4K参考图 61×116×99×164×33×4 |
| nano-banana-2 | 图像 | 1K**0.6**2K**0.6**4K**1.2** | 1K2K4K参考图 61×116×99×164×33×41×81×44×18×1 |


## 文生图参数 /v1/images/generations
| **参数** | **类型** | **必填** | **说明** |
| --- | --- | --- | --- |
| model | string | 必填 | 模型名(别名优先),见上表(图像) |
| prompt | string | 必填 | 文字描述 |
| size | string | 可选 | 宽x高,如 "1024x1024"。同时决定「比例」+「分辨率档」(按长边)；当前 gpt-image-2 为 1K/2K/4K = 0.6/0.6/1.2 积分，4K 最大 4096px / 16777216 像素。留空 = 1:1 · 2K |
| background | string | 可选 | 透明/不透明/自动：文生图、图生图都可传 transparent；仅 gpt-image-2 / gpt-image-1 生效，banana 不支持 |
| output_format | string | 可选 | png / webp / jpeg；透明背景会自动按 png 输出，图生图同样适用 |
| response_format | string | 可选 | url 或 b64_json；指定 b64_json 时由当前服务下载产物后以内联 base64 返回，省略时 URL 优先 |


透明背景适用于文生图和图生图中的 `gpt-image-2` / `gpt-image-1`： 把 `background` 设为 `transparent`， 并把 `output_format` 设为 `png`；`banana` 不支持。

## 图生图参数 /v1/images/edits · multipart
| **参数** | **类型** | **必填** | **说明** |
| --- | --- | --- | --- |
| image | file | 必填 | 输入图;多张参考图重复 image[] 字段(multipart 文件上传) |
| prompt | string | 必填 | 编辑/参考描述 |
| model | string | 必填 | 模型名(别名优先,需支持图生图) |
| size | string | 可选 | 同图像:决定比例 + 分辨率档(见下方对照表)；当前 gpt-image-2 为 1K/2K/4K = 0.6/0.6/1.2 积分，4K 最大 4096px / 16777216 像素 |
| background | string | 可选 | 透明/不透明/自动：图生图也可传 transparent；仅 gpt-image-2 / gpt-image-1 生效，banana 不支持 |
| output_format | string | 可选 | png / webp / jpeg；透明背景会自动按 png 输出 |
| response_format | string | 可选 | url 或 b64_json；指定 b64_json 时由当前服务下载产物后以内联 base64 返回 |
| mask | file/base64 | 可选 | multipart 传文件，JSON 传 base64/data URI；仅 OpenAI-compatible/custom 和旧 Banana 上游真正执行，其他 provider 返回不支持 |


图生图里的透明背景同样可用：把 `background` 设为 `transparent`， 并把 `output_format` 设为 `png`；但只对 `gpt-image-2` / `gpt-image-1` 生效，`banana` 不支持。

## 视频参数 /v1/videos · 异步
| **参数** | **类型** | **必填** | **说明** |
| --- | --- | --- | --- |
| model / model_id | string | 必填 | 使用 GET /v1/models 返回的公开 id；配置了 alias 时优先使用 alias。不要传内部 upstream_model |
| prompt | string | 必填 | 文字描述 |
| seconds / duration | string|int | 必填 | 时长秒数或带 s 的时长,如 "5"、"30s"(取决于模型支持) |
| size | string | 可选 | OpenAI 兼容写法,如 "1280x720" / "1344x768"；用于推导比例与分辨率，显式 ratio/resolution 优先 |
| resolution | string | 可选 | 直接指定模型支持的分辨率,如 "720p"、"768p"；优先于 size 推导值 |
| ratio / aspect_ratio | string | 可选 | 直接指定模型支持的比例,如 "16:9" 或 "9:16"；优先于 size 推导值 |
| input_reference / reference_images / image | file|string|string[] | 可选 | 图生视频参考图。multipart 传文件；JSON 传纯 base64 或 data URI，单值和数组均可。不要传公网 URL |
| input_audio / reference_audios / audio | file|string|string[] | 可选 | 参考音频。multipart 传文件；JSON 传纯 base64 或 data URI，单值和数组均可 |
| reference_mode | string | 可选 | 参考图用途:"frame"=首尾帧(最多 2 张,第 1 张为首帧、第 2 张为尾帧,只传 1 张即仅首帧),"asset"=普通参考图。留空用模型默认;仅支持单一模式的模型不可传 |


**与页面调用的关系：**页面会从模型配置中自动选择合法的时长、比例和分辨率，并把已选素材转换为 `reference_images` base64。接口调用使用同一模型配置和同一 `upstream_model` 映射，但调用方必须自行传入这些字段。先请求 `GET /v1/models` 获取公开模型名和能力，不要猜测模型名，也不要直接使用内部上游模型名。

## 商品套图 AI 分析 /v1/product-image-suites/analyze
+ 输入 1-5 张商品参考图，支持 JSON base64 / data URI，也支持 multipart 的 `image[]`、`reference_images[]`。
+ 会返回并回填 `product_info`、`brand_name`、`brand_colors`、`font_style`、`sales_region`、`platform`、`language` 等字段。
+ 浏览器页同一能力对应 `/admin/api/product-image-suite/analyze`，session 调用免费。
+ Bearer API Key 调用按 usage 计费；account/password 仅用于 OpenAPI 生成任务，不适用于这个直接分析入口。

## 商品套图生成 /v1/product-image-suites
+ 输入参考图和 `params`；常用字段是 `model_id`、`product_info`、`a_plus_*`、`sell_point_count`、`white_bg_count`（默认 0）、`scene_count`、`dimension_count`、`text_required`。
+ `text_required` 只增强卖点图 / A+ 的可见文案，`scene` 和 `white_bg` 仍然偏无字。
+ 返回 `request_id` / `query_url`，完成后结果在 `items` 和 `urls` 里。
+ 浏览器页同一能力对应 `/admin/api/product-image-suite`。
+ OpenAPI 兼容入口也支持 `task_type=product_image_suite`。

## 旧项目调用迁移对照
| **旧调用** | **当前调用** | **兼容说明** |
| --- | --- | --- |
| /api/v1/generation/product-image-suite/analyze | 同路径 | 需要当前项目 session，响应仍为 `code/message/data`。 |
| /api/v1/generation/product-image-suite | 同路径 | 用 `GET /api/v1/tasks/{task_id}` 轮询，结果读取 `result_urls`。 |
| /api/v1/open/generations | 同路径 | 支持 account/password 或 Bearer API Key；先取外层 `data`，再读取 `request_id`。 |
| /v1/images/edits + JSON 图片 | multipart 或 JSON image/images/reference_images | JSON 图片会归一化为当前引擎的参考图；`mask` 会传给支持它的 custom/OpenAI-compatible 与旧 Banana 上游，其余 provider 明确返回不支持。 |
| 全局 API_KEY/shared key | 用户 API Key 或绑定后的 shared key | 配置 `LEGACY_SHARED_API_KEY` 与 `LEGACY_SHARED_API_ACCOUNT` 后，旧 shared key 会归属到指定用户并沿用当前积分、并发、日志和审计链路。 |


**字段变化**：旧项目的 `model_id/model`、`images/reference_images`、`quality` 别名在开放兼容入口会做映射；标准 `/v1` 主要使用 OpenAI 字段。

**结果字段**：标准图像接口可能返回 `data[0].url` 或 `data[0].b64_json`，不能只读取其中一个。旧套图接口返回 `task_id/result_urls`，开放兼容接口返回 `request_id/result/items/urls`，两套 ID 和轮询路径不能混用。

**模型边界**：旧 Banana 名称只在兼容别名已登记、分辨率为 1K/2K/4K、比例受支持且旧 Banana 上游已配置时可用；否则会收到未知模型、未定价或上游不可用错误。

## 图像分辨率对照表 · `size` 该传什么
左边选比例,上面选分辨率档,交叉格里就是 `size` 要传的值(直接复制)。 没有 `quality` 参数,图像分辨率只看 `size` 的**长边**。 档位必须是该模型支持的(见上方「可用模型」的分辨率列),不支持会自动回退到该模型最低档。

| **比例** | **1K** | **2K** | **4K** |
| --- | --- | --- | --- |
| 1:1 · 方 | 1024x1024 | 2048x2048 | 4096x4096 |
| 5:4 · 横 | 1280x1024 | 2560x2048 | 3840x3072 |
| 4:3 · 横 | 1024x768 | 2048x1536 | 4096x3072 |
| 3:2 · 横 | 1200x800 | 2400x1600 | 3600x2400 |
| 16:9 · 横 | 1280x720 | 2048x1152 | 4096x2304 |
| 2:1 · 横 | 1440x720 | 2880x1440 | 4096x2048 |
| 21:9 · 超宽 | 1680x720 | 2520x1080 | 5040x2160 |
| 3:1 · 超宽 | 1536x512 | 2304x768 | 3840x1280 |
| 4:1 · 超宽 | 1728x432 | 2880x720 | 4096x1024 |
| 8:1 · 超宽 | 1728x216 | 2880x360 | 4096x512 |
| 4:5 · 竖 | 1024x1280 | 2048x2560 | 3072x3840 |
| 3:4 · 竖 | 768x1024 | 1536x2048 | 3072x4096 |
| 2:3 · 竖 | 800x1200 | 1600x2400 | 2400x3600 |
| 9:16 · 竖 | 720x1280 | 1152x2048 | 2304x4096 |
| 1:3 · 竖 | 512x1536 | 768x2304 | 1280x3840 |
| 1:4 · 竖 | 432x1728 | 720x2880 | 1024x4096 |
| 1:8 · 竖 | 216x1728 | 360x2880 | 512x4096 |


例:想要 **2K 的 16:9 横图** → `"size": "2048x1152"`。 留空 size = 默认 **1:1 · 2K**。

## 视频分辨率对照表 · `size` 该传什么
视频用 `720p` / `768p` / `1080p` 档位,只看 `size` 的**短边**(短边 <768 = 720p, 768–1079 = 768p, ≥1080 = 1080p)。 档位必须是该视频模型支持的(如 grok-video 仅 720p),不支持会被拒。

| **比例** | **720p** | **768p** | **1080p** |
| --- | --- | --- | --- |
| 16:9 · 横 | 1280x720 | 1344x768 | 1920x1080 |
| 9:16 · 竖 | 720x1280 | 768x1365 | 1080x1920 |
| 1:1 · 方 | 720x720 | 768x768 | 1080x1080 |
| 4:3 · 横 | 960x720 | 1024x768 | 1440x1080 |
| 3:4 · 竖 | 720x960 | 768x1024 | 1080x1440 |
| 3:2 · 横 | 1080x720 | 1152x768 | 1620x1080 |
| 2:3 · 竖 | 720x1080 | 768x1152 | 1080x1620 |


例:想要 **720p 的 16:9 横版视频** → `"size": "1280x720"`; 竖版 9:16 → `"720x1280"`。

## 调用示例
文生图 · curl

```plain
curl https://aigc.easysu.cn/v1/images/generations \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "a corgi running in a golden wheat field, cinematic",
    "size": "2048x2048"
  }'
```

文生图 · Python (openai SDK)

```plain
import urllib.request
from openai import OpenAI

client = OpenAI(api_key="YOUR_API_KEY", base_url="https://aigc.easysu.cn/v1")

resp = client.images.generate(
    model="gpt-image-2",
    prompt="a corgi running in a golden wheat field, cinematic",
    size="2048x2048",   # 2K · 1:1,见下方对照表
)
# 默认返回 URL；需要内联图片时传 response_format="b64_json"。
item = resp.data[0]
if getattr(item, "url", None):
    urllib.request.urlretrieve(item.url, "out.png")
else:
    import base64
    with open("out.png", "wb") as f:
        f.write(base64.b64decode(item.b64_json))
```

图生图 / 参考图 · curl (multipart)

```plain
curl https://aigc.easysu.cn/v1/images/edits \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -F model="gpt-image-2" \
  -F prompt="把这张图改成赛博朋克风格" \
  -F size="2048x2048" \
  -F image=@input.png
# 多张参考图:重复 -F image=@a.png -F image=@b.png
```

图生图 · 透明背景 · curl

```plain
curl https://aigc.easysu.cn/v1/images/edits   -H "Authorization: Bearer YOUR_API_KEY"   -F model="gpt-image-2"   -F prompt="把这张图改成透明贴纸风格"   -F size="2048x2048"   -F background="transparent"   -F output_format="png"   -F image=@input.png
# 透明背景只对 gpt-image-2 / gpt-image-1 生效；banana 系列不支持
```

图生图 · Python (openai SDK)

```plain
import urllib.request
from openai import OpenAI

client = OpenAI(api_key="YOUR_API_KEY", base_url="https://aigc.easysu.cn/v1")

resp = client.images.edit(
    model="gpt-image-2",
    image=open("input.png", "rb"),     # 多张:image=[open("a.png","rb"), open("b.png","rb")]
    prompt="把这张图改成赛博朋克风格",
)
# 默认返回 URL；需要内联图片时传 response_format="b64_json"。
item = resp.data[0]
if getattr(item, "url", None):
    urllib.request.urlretrieve(item.url, "out.png")
else:
    import base64
    with open("out.png", "wb") as f:
        f.write(base64.b64decode(item.b64_json))
```

文生视频 · curl（创建 → 轮询 → 下载）

```plain
# 1) 创建任务 → 立即返回 {"id": "...", "status": "queued"}
curl https://aigc.easysu.cn/v1/videos \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax-h3-933-图文",
    "prompt": "a paper boat sailing down a rainy street, cinematic",
    "seconds": "5",
    "size": "1344x768"
  }'

# 2) 轮询状态,直到 status=completed
curl https://aigc.easysu.cn/v1/videos/<VIDEO_ID> \
  -H "Authorization: Bearer YOUR_API_KEY"

# 3) 下载 mp4(完成后)
curl https://aigc.easysu.cn/v1/videos/<VIDEO_ID>/content \
  -H "Authorization: Bearer YOUR_API_KEY" -o out.mp4
```

图生视频 · curl（multipart 参考图）

```plain
# input_reference 必须作为文件上传；多张参考图重复该字段
curl https://aigc.easysu.cn/v1/videos \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -F model="seedance2.0-900-720p" \
  -F prompt="让画面中的人物自然运动，镜头缓慢推进" \
  -F seconds="10" \
  -F size="1280x720" \
  -F input_reference=@input.jpg

# 返回 id 后，继续使用下方状态接口轮询；不要等待 POST 直接返回视频文件。
```

视频 · Python (requests, 轮询)

```plain
import time, requests

base = "https://aigc.easysu.cn/v1"
h = {"Authorization": "Bearer YOUR_API_KEY"}

# 1) 创建
job = requests.post(f"{base}/videos", headers=h, json={
    "model": "minimax-h3-933-图文",
    "prompt": "a paper boat sailing down a rainy street",
    "seconds": "5",
    "size": "1344x768",
}).json()
vid = job["id"]

# 2) 最长轮询 30 分钟
deadline = time.time() + 30 * 60
while time.time() < deadline:
    s = requests.get(f"{base}/videos/{vid}", headers=h).json()
    if s["status"] in ("completed", "failed"):
        break
    time.sleep(5)
else:
    raise TimeoutError("video generation timed out after 30 minutes")

# 3) 下载
if s["status"] == "completed":
    mp4 = requests.get(f"{base}/videos/{vid}/content", headers=h).content
    open("out.mp4", "wb").write(mp4)
else:
    raise RuntimeError(s.get("error", {}).get("message", "video generation failed"))
```

列出模型 · curl

```plain
curl https://aigc.easysu.cn/v1/models \
  -H "Authorization: Bearer YOUR_API_KEY"
```

查询余额 · curl

```plain
curl https://aigc.easysu.cn/v1/user/balance \
  -H "Authorization: Bearer YOUR_API_KEY"

# => {"object":"user.balance","balance":12000,"used":680,"total":12680}
```

商品套图分析 · curl

```plain
curl https://aigc.easysu.cn/v1/product-image-suites/analyze \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model_id": "gpt-image-2",
    "product_info": "户外灯，防水耐晒",
    "images": ["<base64-1>", "<base64-2>"],
    "params": {
      "brand_name": "示例品牌",
      "platform": "Amazon",
      "language": "英文"
    }
  }'
# 浏览器页同一接口：/admin/api/product-image-suite/analyze
# session 调用免费；Bearer API Key 调用按 usage 计费
# account/password 仅用于 /api/v1/open/generations 的生成任务
```

商品套图生成 · curl

```plain
curl https://aigc.easysu.cn/v1/product-image-suites \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model_id": "gpt-image-2",
    "product_info": "户外灯，防水耐晒",
    "images": ["<base64-1>"],
    "params": {
      "scene_count": 2,
      "white_bg_count": 1,
      "language": "英文"
    }
  }'
# 浏览器页同一接口：/admin/api/product-image-suite
# 兼容入口也支持 task_type=product_image_suite
```

OpenAPI 套图 · curl

```plain
curl https://aigc.easysu.cn/api/v1/open/generations \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "task_type": "product_image_suite",
    "model_id": "gpt-image-2",
    "prompt": "户外灯，防水耐晒",
    "reference_images": ["<base64-1>"],
    "params": {
      "scene_count": 2,
      "white_bg_count": 1
    }
  }'
# 创建后轮询 /api/v1/open/generations/<request_id> 或 /api/v1/open/results/<request_id>
```

## 响应 & 计费
**图像**(generations / edits)返回 OpenAI 图片格式，`data[0]` 可能包含 `url` 或 `b64_json`。多数模型返回上游**原始直链**；少数上游返回需要本站鉴权的转发链 `/v1/images/{id}/content`。URL 可能过期，请**尽快下载或转存到你自己的存储**。

**视频**(异步,Sora 风格三步):

1. `POST /v1/videos` 立即返回任务对象 `{ "id": "...", "object": "video", "status": "queued", ... }`
2. 轮询 `GET /v1/videos/{id}`,`status` 从 `queued → in_progress → completed`(或 `failed`)
3. 完成后 `GET /v1/videos/{id}/content` 返回 **mp4 原始二进制**(非 base64、非 URL)

**计费(预扣)**:生成**前**按上表价格从你的 Key 账号预扣积分;图像或视频上游失败会自动退回 —— 失败不扣费。商品套图按实际生成的子图数量逐项计费；AI 分析仅 API Key / account 调用按 usage 计费，浏览器 session 调用免费。

**参数映射**:图像 `size` 按长边映射 1K/2K/4K；视频 `size` 按短边映射 720p/768p/1080p。视频显式传入的 `ratio/aspect_ratio` 与 `resolution` 优先于 `size` 推导值，`seconds` 映射视频时长。所有值必须位于该模型展示的能力和定价范围内，否则返回 400。

**兼容边界**:标准 `/v1/images/*` 支持 `n=1-4`，每张图都是独立的当前引擎任务，并按总数量预检查余额；`response_format=url/b64_json` 会决定返回字段。`/v1/images/edits` 同时接受 multipart 与 JSON base64 图片，`mask` 按 provider 能力执行或明确报不支持。需要旧项目的异步任务、account/password、套图结果 `result_urls` 格式，请使用 `/api/v1/open/*` 或旧版套图别名。

401Key 无效 / 上游需重新授权

404未知 model / 视频任务不存在

400参数缺失 / 不支持或未定价

402积分不足

409视频尚未完成(content 未就绪)

429账号并发已满,请重试

503上游繁忙,请重试














