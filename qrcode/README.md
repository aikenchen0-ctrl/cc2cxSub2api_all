# 墨莲生成二维码 - QR Code Art Studio

一个强大的二维码艺术化处理系统，支持将普通二维码或URL转换为精美的艺术化二维码。

## ✨ 主要功能

### 🎯 核心功能
- **二维码艺术化**: 将普通二维码转换为艺术风格的二维码
- **URL转二维码**: 直接将URL地址转换为二维码（新功能！）
- **手动框选剪裁**: 精确控制二维码区域
- **模板合成**: 支持空白盘模板合成
- **参考图引导**: 使用参考图指导艺术化风格

### 🆕 最新功能: URL转二维码

现在你可以直接输入URL地址，系统会自动将其转换为二维码！

**使用方式:**
1. 选择"我有URL"选项
2. 输入完整的URL（如 `https://example.com`）
3. 点击"转换为二维码"
4. 继续后续的艺术化处理流程

详见 [快速开始指南](QUICKSTART.md)

## 🚀 快速开始

### 安装依赖
```bash
npm install
```

### 启动服务
```bash
npm start
```

### 访问应用
打开浏览器访问: http://127.0.0.1:3100

## 部署上线检查

上线时不能只上传 `public` 静态目录，必须运行 Node 服务：

```bash
npm install
npm start
```

如果部署平台分配了端口，请设置 `PORT`，服务会优先使用该端口：

```bash
PORT=3100 npm start
```

上线后先访问：

```text
https://你的域名/api/health
```

正常应返回：

```json
{ "ok": true }
```

如果前端和后端使用同一个域名，建议把 `/api` 反向代理到 Node 服务，并保持页面中的：

```html
<meta name="qr-api-base-path" content="/api" />
```

如果前端和后端分开部署，需要把页面 API 地址改为后端地址：

```html
<script>
  window.QR_API_BASE_PATH = "https://api.example.com/api";
</script>
```

或改为：

```html
<meta name="qr-api-base-path" content="https://api.example.com/api" />
```

后端不同域名时，还需要设置允许跨域的前端域名：

```bash
QR_CORS_ORIGIN=https://www.example.com npm start
```

多个域名可用英文逗号分隔。仅测试环境才建议使用 `QR_CORS_ORIGIN=*`。

微信 `access_token` 相关接口由服务端访问 `api.weixin.qq.com`。如果线上获取失败并提示 IP 白名单，请把接口返回的服务器出口 IP 加入微信公众号后台“基本配置”的 IP 白名单。云服务器、容器、Serverless、代理环境要填写真实出站 IP，不是浏览器用户 IP。

## 📖 使用文档

- [🚀 快速开始](QUICKSTART.md) - 5分钟上手指南
- [📝 URL转二维码功能说明](URL转二维码功能说明.md) - 详细使用文档
- [🎨 功能演示页面](URL转二维码演示.html) - 可视化演示
- [📊 实现总结](IMPLEMENTATION_SUMMARY.md) - 技术实现细节

## 🛠️ 技术栈

### 后端
- **Node.js** - 运行时环境
- **Express** - Web框架
- **Sharp** - 图片处理
- **QRCode** - 二维码生成
- **Multer** - 文件上传

### 前端
- **原生JavaScript** - 无框架依赖
- **Canvas API** - 图像处理和框选
- **Fetch API** - HTTP请求

## 📁 项目结构

```
├── public/                 # 前端文件
│   ├── index.html         # 主页面
│   ├── app.js             # 前端逻辑
│   └── styles.css         # 样式文件
├── src/                   # 后端源码
│   ├── server.js          # 服务器入口
│   ├── qr-app.js          # Express应用
│   ├── qr-generator.js    # 二维码生成
│   ├── url-to-qr.js       # URL转二维码 ⭐新增
│   ├── art-qr.js          # 艺术化处理
│   ├── mask-processor.js  # 遮罩处理
│   └── config.js          # 配置文件
├── tests/                 # 测试文件
├── output/                # 输出目录
└── docs/                  # 文档
    ├── QUICKSTART.md
    ├── URL转二维码功能说明.md
    └── IMPLEMENTATION_SUMMARY.md
```

## 🔧 API接口

### URL转二维码
```http
POST /api/url-to-qr
Content-Type: application/json

{
  "url": "https://example.com"
}
```

### 艺术化二维码
```http
POST /api/stylize-qr
Content-Type: multipart/form-data

qrImage: <file>
```

### 生成最终二维码
```http
POST /api/generate
Content-Type: multipart/form-data

qrImage: <file>
templateImage: <file>
referenceImages: <files>
positivePrompt: <text>
negativePrompt: <text>
```

## 🧪 测试

### 运行单元测试
```bash
npm test
```

### URL转二维码测试
```bash
node test-url-qr.js
```

### API接口测试
```bash
node test-api.js
```

## 📸 功能截图

- [普通二维码输入](普通二维码输入.png)
- [可以扫的二维码](可以扫的二维码.png)
- [艺术可扫二维码](艺术可扫二维码.png)
- [应该避免的样式](应该避免的样式.png)

## 💡 使用提示

### URL转二维码
1. ✅ URL必须包含协议（http:// 或 https://）
2. ✅ 建议使用短URL以获得更好的效果
3. ✅ 生成的二维码具有高容错率，可扫描
4. ❌ 不要使用过长的URL

### 艺术化处理
1. ✅ 选择合适的模板图片
2. ✅ 使用参考图引导风格
3. ✅ 调整遮罩位置以匹配模板
4. ✅ 编写清晰的prompt描述

### 框选技巧
1. ✅ 精确框选二维码区域
2. ✅ 保留足够的边距
3. ✅ 避免包含过多空白
4. ✅ 注意二维码的定位点

## 🔍 配置说明

编辑 `src/config.js` 可以修改：
- API密钥和端点
- 图片尺寸限制
- 超时设置
- 默认prompt

## 🤝 贡献

欢迎提交Issue和Pull Request！

## 📄 许可证

本项目仅供学习和研究使用。

## 🙏 致谢

感谢所有为这个项目做出贡献的开发者！

---

**Made with ❤️ by QR Code Art Studio Team**
