# 格知 · AI Excel Copilot

一个本地优先的 AI Excel 智能操作助手。上传 `.xlsx` 或 `.xls` 文件后，可以通过自然语言完成公式生成、批量修改、排序筛选、条件着色、跨表关联、数据分析和报表制作。

Excel 文件在浏览器中解析和修改，不会上传到业务服务器。AI 仅接收工作表结构、表头、行列数量和少量数据样例。

## 功能特性

### Excel 文件处理

- 支持点击选择和拖拽上传
- 支持 `.xlsx` 与 `.xls`
- 解析多个 Sheet、公式、日期、样式和单元格格式
- 在线表格预览、Sheet 切换、搜索、缩放
- 保留公式、字体、颜色、边框和条件格式
- 一键导出修改后的 `.xlsx`

### AI 自动化操作

- 新增公式列并批量填充
- 修改、清空和批量更新单元格
- 插入、删除一行或多行
- 新增、删除一列或多列
- 批量填充指定列中的全部空白单元格
- 跨 Sheet 生成 `VLOOKUP`、`XLOOKUP` 等公式
- 查找与替换
- 工作表自动美化
- 根据字段值为整行设置指定颜色

### 排序与筛选

- 为任意表头行启用筛选按钮
- 按指定列升序或降序排列完整数据
- 支持等于、不等于、包含、大于、小于筛选
- 支持筛选空白或非空白单元格
- 在线预览中的筛选按钮可直接点击操作
- 清除筛选并恢复全部数据

### 数据分析与报表

- 按分类字段和数值字段自动汇总
- 计算样本数、平均值、最低值和最高值
- 在新 Sheet 中生成研究报表
- 生成可导出到 Excel 的柱状图
- 在线交互式图表支持：
  - 指标切换
  - 升降序切换
  - 悬停查看数值
  - 点击查看区域明细
  - 研究选项与样本可信度分析

### 操作安全

- AI 生成操作计划后由用户确认执行
- 支持撤销、重做和操作历史
- 未识别的 AI 操作不会静默执行
- 只有实际执行有效操作后才提示成功
- API Key 不写入工作簿和操作记录

## 技术栈

- React 19
- TypeScript
- Vinext / Vite
- ExcelJS
- SheetJS
- Lucide React
- DeepSeek API

## 环境要求

- Node.js `>= 22.13.0`
- npm

## 安装与启动

```bash
git clone https://github.com/ns2250225/ai-excel.git
cd ai-excel
npm install
npm run dev
```

启动后访问：

```text
http://localhost:3000
```

## Sub2API 配置

从 Sub2API 左侧的“AI表格”入口打开应用。登录通过短期一次性 SSO 票据完成，模型请求由 AIExcel 服务端转发到 Sub2API，浏览器不会接触 API Key。

服务端配置 `.env.local`：

```env
SUB2API_SSO_SECRET=与 Sub2API 相同的至少 32 位密钥
SUB2API_RELAY_BASE_URL=http://localhost:18080/v1
SUB2API_APP_CREDENTIAL=Sub2API 为卫星签发的应用凭据
SUB2API_RELAY_MODEL=gpt-5.5
AIEXCEL_SSO_CALLBACK_URL=http://localhost:4173/api/auth/sso/callback
```

`.env.local` 已被 Git 忽略，请勿将真实密钥提交到仓库。

## 构建

### 应用构建

```bash
npm run build
```

构建结果位于 `dist`，适用于 Vinext 运行环境。

### 纯静态网站（仅本地预览）

```bash
npm run build:static
```

构建结果位于：

```text
out/
├── index.html
├── assets/
└── favicon.svg
```

可以使用任意静态服务器运行，但静态服务器不提供 SSO 回调和 `/api/ai`，不能用于 Sub2API 集成环境：

```bash
python3 -m http.server 4173 --directory out
```

然后访问：

```text
http://localhost:4173
```

由于浏览器安全策略限制，不建议直接双击 `out/index.html` 以 `file://` 方式打开。

## 隐私说明

Excel 原始文件不会上传到 DeepSeek。发送给 AI 的内容仅包括：

- Sheet 名称
- 表头候选行
- 行列数量
- 少量样例数据
- 用户的自然语言指令

具体的全表扫描、公式填充、空白处理、排序筛选和文件导出均在浏览器本地完成。

## 项目结构

```text
app/
├── api/ai/route.ts     # 本地开发模式的 AI 代理
├── globals.css         # 页面和工作区样式
├── layout.tsx
└── page.tsx            # Excel、AI 与交互式报表核心功能

static/
├── index.html          # 静态版本入口
└── main.tsx

public/                 # 公共资源
out/                    # 静态构建输出，默认不提交
vite.static.config.ts   # 静态构建配置
prd.md                  # 产品需求文档
```

## 常用命令

```bash
npm run dev          # 启动本地开发服务
npm run build        # 构建完整应用
npm run build:static # 构建纯静态网站到 out
npm run lint         # 代码检查
```

## 注意事项

- 大型工作簿的解析和导出速度取决于浏览器性能。
- ExcelJS 无法生成原生可交互 Excel 图表，因此导出文件中保存静态图表；网页预览提供完整交互图表。
- 生产环境必须使用完整的 Vinext 服务端版本，以提供 SSO 回调、会话和 Sub2API 中转请求。
- 使用 AI 修改重要文件前，建议保留原文件备份。

## License

当前仓库未指定开源许可证。如需公开分发或商业使用，请先补充合适的 License。
