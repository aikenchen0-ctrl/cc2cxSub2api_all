# AI 获客（ai-hunter）

本目录是「AI 获客」工作区快照，作为 `cc2cxSub2api_all` 的一个项目文件夹上传。产品界面名是 **AI 获客**，内部代码目录仍叫 `ai-hunter-root`。

## 目录

| 路径 | 用途 |
| --- | --- |
| `ai-hunter-root/` | 可运行的工作树。只改这里。 |
| `refs/` | 只读参考仓（`AI_Find_Customer`、`OpenOutreach`、`ai-outreach-engine`、`maigret`、`spiderfoot`）。不要改，不要往主树拷 GPL 代码。 |
| `工作留档-失忆复现.md` | 失忆复现入口：硬规则、已落地模块、搜索/LLM 现状、禁止重试的 hunt。 |
| `根项目选定与模块吸收方案.md` | 根项目选定与吸收边界。 |
| `AI全网营销线索情报清单.md` | 早期情报与对标。 |

## 不要提交的内容

真实密钥在本机 `ai-hunter-root/backend/.env`，不进本仓库。部署时从 `ai-hunter-root/backend/.env.example` 复制。

`node_modules`、数据库、`uploads`、`.workbuddy` 缓存也不在本目录里。

## 本地启动

```bash
cd ai-hunter-root/backend
cp .env.example .env
# 填 LLM / 搜索 Key
# 后端默认 http://127.0.0.1:8000

cd ../frontend
npm install
npm run dev
```

OpenOutreach 是 GPL-3.0：可参考思路，禁止拷代码进 `ai-hunter-root`。P5（发信执行层）按留档不要动。
