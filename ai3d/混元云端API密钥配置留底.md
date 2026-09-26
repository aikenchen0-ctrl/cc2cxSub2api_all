# 混元云端 API 密钥配置留底

更新日期：2026-09-24  
适用项目：cc2cx AI3D生成（`A-Web2Ai3D`）

## 配置结论

混元云端密钥只需要配置在项目根目录的 `.env.local` 中：

```dotenv
HUNYUAN_CLOUD_API_KEY=replace_with_your_hunyuan_key
```

请将示例值替换为混元云端控制台提供的 API 密钥。不要把真实密钥写入前端代码、截图、日志、文档或公开仓库。

后端会在启动时读取项目根目录的 `.env.local`。前端不会读取密钥；后端请求混元接口时会在服务端加入 Bearer 授权头。修改密钥后需要重启后端。

## Windows 本地配置

在项目根目录打开 PowerShell。若 `.env.local` 尚不存在，可从模板创建；若文件已存在，不要覆盖，以免丢失其他本地配置：

```powershell
if (-not (Test-Path .env.local)) { Copy-Item .env.example .env.local }
notepad .env.local
```

在文件中新增或更新 `HUNYUAN_CLOUD_API_KEY` 这一行，保存后按下面方式启动服务。

## macOS / Linux 本地配置

在项目根目录打开终端。仅当 `.env.local` 不存在时从模板创建：

```bash
if [ ! -f .env.local ]; then cp .env.example .env.local; fi
${EDITOR:-nano} .env.local
```

在文件中新增或更新 `HUNYUAN_CLOUD_API_KEY` 这一行。不要把密钥放进 `VITE_*` 变量；这类变量会进入浏览器端构建。

## 启动项目

先安装依赖（首次运行或依赖更新后执行）：

```bash
npm install
```

分别打开两个终端，并确保当前目录都是项目根目录：

终端一，启动后端：

```bash
npm run dev:api
```

终端二，启动前端：

```bash
npm run dev
```

按 Vite 输出的地址打开页面，通常为 `http://127.0.0.1:5173/`。后端默认地址为 `http://127.0.0.1:8787`。

## 验证配置

浏览器打开后端健康检查地址：

```text
http://127.0.0.1:8787/api/3d/health
```

当混元云端密钥被正确加载时，`providers.hunyuan` 应显示 `configured: true`、`mode: "cloud"`。混元草图模式默认使用 `model: "3.0"` 和 `generateType: "Sketch"`。健康检查只显示是否已配置，不会返回密钥内容。

常见问题：

- `configured` 为 `false`：检查 `.env.local` 是否在项目根目录、变量名是否准确、等号后是否填入有效密钥，然后重启后端。
- 页面可打开但提交报未配置：确认启动的是本项目的 `npm run dev:api`，而不是另一个旧后端进程。
- 请求被拒绝或余额不足：到混元服务控制台检查密钥状态、权限和账户额度；这类问题不能通过更换前端设置解决。

## 部署环境配置

部署到服务器或托管平台时，在平台的后端服务环境变量 / Secrets 页面添加：

```text
HUNYUAN_CLOUD_API_KEY=<在部署平台的安全配置中填写真实值>
```

只添加到后端运行环境，不添加到静态前端构建变量。保存后按平台要求重启或重新部署后端。不要把生产密钥提交到 Git 仓库，也不要把密钥写入构建参数或前端公开变量。

## Git 与密钥安全

- `.env.local` 已由仓库 `.gitignore` 忽略，正常情况下不会进入 Git 提交；`.env.example` 只保留占位值，可安全用于说明变量名。
- 提交前可以在项目根目录执行 `git check-ignore -v .env.local`，确认该文件确实被忽略。
- 不要上传 `.env.local`，即使目标仓库是自己的仓库；公开仓库中的密钥可被任何人读取。
- 本项目此前曾将混元密钥写入公开仓库，因此该旧密钥应视为已暴露：请立即在服务控制台吊销或轮换，并只把新密钥配置到本机 `.env.local` 或部署平台的后端 Secrets。
- 如果真实密钥曾经被提交或上传到公开仓库，仅删除当前文件并不能清除 Git 提交历史，不能把旧密钥继续当作安全凭据。

## 当前项目默认值

仅设置 `HUNYUAN_CLOUD_API_KEY` 即可启用当前混元草图模式。其余云端参数已有默认值：

| 配置 | 默认值 |
| --- | --- |
| `HUNYUAN_CLOUD_API_BASE` | `https://api.ai3d.cloud.tencent.com` |
| `HUNYUAN_CLOUD_SUBMIT_PATH` | `/v1/ai3d/submit` |
| `HUNYUAN_CLOUD_QUERY_PATH` | `/v1/ai3d/query` |
| `HUNYUAN_CLOUD_MODEL` | `3.0` |
| `HUNYUAN_CLOUD_GENERATE_TYPE` | `Sketch` |
| `HUNYUAN_CLOUD_ENABLE_PBR` | `true` |
| `HUNYUAN_CLOUD_FACE_COUNT` | `30000` |
