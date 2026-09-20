# AI 搜索全网营销线索：情报清单（含链接）

> 调研日期：2026-09-17  
> 范围：指定社区（t.me / Telegram、NodeSeek、linux.do、V2EX、Gitee、Nodeloc）+ 社区讨论中的联想关键词（OpenClaw / 养虾 / GEO / 外贸获客 / 一人公司 / AI Hunter / IMAI.WORK / HiveMtk 等）  
> 说明：`site:` 精确检索时，NodeSeek / linux.do / Nodeloc 原生帖常被聚合站、媒体转载覆盖；下列同时收录「社区原生帖」和「可落地的转载/媒体源」。

---

## 0. 先看结论（给项目定位用）

当前「AI 全网搜营销线索」赛道，实际分成 **三条互有交叉的产品线**：

1. **执行型 Agent 框架（OpenClaw / 养虾）**  
   本地部署、操控浏览器/电脑、自动发邮件/跟进。社区热、安装门槛高、安全风险大。适合作为「执行层」，不适合单独当获客 SaaS。
2. **开源获客系统（AI Hunter、IMAI.WORK、HiveMtk）**  
   专门做线索搜索、私域触达、多渠道分发。更接近本项目的产品形态。
3. **GEO / AI 搜索优化 + 商业获客 SaaS**  
   让品牌进入 ChatGPT / 豆包 / DeepSeek 答案；国内已有 NovaSea、Revor、埃米 AIMI、智达明远、惠米聚客等在抢市场。

对本项目最有价值的对标：**AI_Find_Customer（开源搜网）+ OpenOutreach（更成熟的找人+发信）+ YALC（开源 Clay 编排）+ IMAI.WORK / HiveMtk（私域）+ Revor / GEO-SEO outreach**。  
OpenClaw 更适合作为「执行手脚」，而不是获客大脑。

**相对 AI_Find_Customer 谁更强（2026-09-17 核实）：**

| 项目 | Star / Fork | 协议 | 最近推送 | 强在哪 | 弱在哪 |
| --- | --- | --- | --- | --- | --- |
| AI_Find_Customer | 175 / 48 | MIT | 2026-07-17 | 搜公开网页（Google / Maps / B2B）、证据链接、中文外贸 | 社区小、邮件自动化不完整、数据靠爬 |
| **OpenOutreach** | **3015 / 560** | GPL-3.0 | 2026-09-07 | 描述即找人、逐条合格理由、验证邮箱、自己邮箱发信、一键 CLI | 不爬公开网页，绑 BetterContact；GPL 传染 |
| **YALC** | **304 / 90** | MIT | 2026-08-20 | 开源 Clay：在现有 CRM/sequencer 上编排 GTM agent | 不是从零搜网的获客引擎 |
| Clay / Apollo / Instantly | SaaS | 闭源 | — | 产品上限、数据质量、触达规模 | 贵、数据不出自己的库 |

结论：比它「更强」的开源仓有，但不是全方位碾压。成熟度看 OpenOutreach；搜公开网页看 AI Hunter 仍更贴本项目。

---

## ① OpenClaw 官方 / 社区 / 文档

| 资源 | 链接 | 备注 |
| --- | --- | --- |
| 官网 | https://openclaw.ai | 官方入口 |
| GitHub | https://github.com/openclaw/openclaw | 主仓库 |
| 中文文档 | https://docs.openclaw.ai/zh-CN | 部署与技能 |
| Telegram 中文社区 | https://t.me/openclaw_cn | 指定检索目标 `t.me` |
| Discord | https://discord.gg/openclaw | 官方社区 |
| Moltbook | https://moltbook.com | 社区站点 |
| 龙虾公社 | https://longxia.red | 中文社区 |
| 技能市场（镜像） | https://openclawmp.cc | ClawHub 类市场 |
| 技能市场 | https://skill.openclawlog.com | 技能索引 |
| ClawHub | https://clawhub.ai | 官方技能市场；曾有供应链投毒报道 |
| 开源上网指南 | https://openclawlog.com/2026/08/11/awesome-openclaw-上网指南 | 中文导航 |
| 中文安装版 | https://github.com/jiulingyun/openclaw-cn | 社区汉化/安装 |
| 百度百科 · OpenClaw社区 | https://baike.baidu.com/item/OpenClaw%E7%A4%BE%E5%8C%BA/67515584 | 社区词条 |
| 百度百科 · Clawdbot | https://baike.baidu.com/item/Clawdbot/67286566 | 曾用名 |
| 百度百科 · Moltbot AI | https://baike.baidu.com/item/Moltbot%20AI/67391927 | 曾用名 |

---

## ② 指定社区 / 频道：OpenClaw、养虾、AI 获客讨论

### Telegram（t.me / tg）

- OpenClaw 中文社区：https://t.me/openclaw_cn
- Telegram 获客成本趋近于零分析：https://news.qq.com/rain/a/20260323A07OHX00

### V2EX

- OpenClaw 源码/架构原理（原生帖）：https://www.v2ex.com/t/1191295
- 外链 Skill 自动化（原生帖）：https://www.v2ex.com/t/1201665
- 睡觉找 100 客户（夜雨聆风转述/相关）：https://www.yeyulingfeng.com/594271.html
- 外贸用好 OpenClaw：https://www.yeyulingfeng.com/627851.html

### Gitee

- IMAI.WORK 主仓/镜像：
  - https://gitee.com/shop-sparker/imaiwork
  - https://gitee.com/dilenzhu201911/imaiwork
  - https://gitee.com/chockzhong/imaiwork
  - https://gitee.com/carrieha/imaiwork
  - https://gitee.com/tsinghua-open/imaiwork
- HiveMtk（私域营销 AI OS，社区讨论常指向此仓）：https://gitee.com/xhpmayun/hivemtk.git

### NodeSeek / linux.do / Nodeloc

`site:nodeseek.com` / `site:linux.do` / `site:nodeloc.com` 的精确检索，当前搜索引擎大量返回媒体转载，而不是社区原生 URL。社区讨论主题高度集中在：

- OpenClaw = 本地 Agent，可接管电脑/浏览器做外贸获客
- 「养虾」= 部署 + 调教；上门安装 300–800 元
- 一人公司（OPC）+ 4A 架构
- Token 黑洞、公网暴露、信用卡盗刷、ClawHub 技能投毒

可交叉验证的公开报道 / 转载：

- 养龙虾是什么：https://www.chinanews.com/sh/2026/03-10/10584563.shtml
- 火到两会：https://www.chinanews.com/gsztc/2026/03-09/10583731.shtml
- 「龙虾」爆火别着急尝鲜：https://www.chinanews.com.cn/ll/2026/03-18/10588619.shtml
- 养虾破财 / 信用卡盗刷：https://www.chinanews.com.cn/cj/2026/03-13/10586143.shtml
- linux.do 相关转载 · 炸穿 AI 圈：https://www.toutiao.com/a7624414576452534825
- 5 条变现路径：https://www.toutiao.com/w/1859596828884043/
- 上门安装赚 26 万：https://www.workercn.cn/c/2026-03-10/8752850.shtml
- 央视 100 秒原理：https://news.cctv.com/2026/03/13/ARTIY6v4ATxCLQGUYPbityre260313.shtml
- 工信部风险提示转述：https://www.cnr.cn/mspd/spcy/20260407/t20260407_527575717.shtml
- 无锡高新区补贴 OpenClaw/OPC：https://www.hubpd.com/detail/index.html?contentId=6052837899190957478

---

## ③ 类似开源项目（最接近「AI 搜全网营销线索」）

| 项目 | 链接 | 要点 |
| --- | --- | --- |
| AI_Find_Customer / AI Hunter | https://github.com/xiongQvQ/AI_Find_Customer | FastAPI + LangGraph + React；多智能体：Insight→KeywordGen→Search→LeadExtract→Evaluate |
| 商业版 B2B Insights | https://b2binsights.io/ | 同一团队商业化 |
| 演示视频 | https://www.bilibili.com/video/BV1AzwYzXEGD/ | 产品演示 |
| IMAI.WORK | https://gitee.com/shop-sparker/imaiwork | PHP7.4 + Swoole + Laravel + MySQL + Redis；IM/私域获客 |
| HiveMtk | https://gitee.com/xhpmayun/hivemtk.git | Go + Vue3 + pgvector；开源私域营销 AI OS（AGPL-3.0） |
| HiveMtk 技术文 | https://juejin.cn/post/7677775118559461426 | 94 模块 / 41 AI 工具 / 13 触达渠道；Demo：http://hiveuser.xapptool.cn/ |
| GEO-SEO / outreach-skill | https://github.com/GEO-SEO/outreach-skill | 外联/outreach 技能 |
| GEO-SEO / seo-geo-content-engine | https://github.com/GEO-SEO/seo-geo-content-engine | GEO 内容引擎 |
| OpenClaw Outreach Skill | https://clawbot.ai/skills/outreach.html | 官方 outreach 技能页 |
| Toolify 技能页 | https://www.toolify.ai/zh/openclaw-skills/outreach-12845 | 技能目录 |

### ③-b 第二轮扩搜（2026-09-17，GitHub API 核实）

用户要求「再找、一定能找到、大量搜索」。本轮结论先说清楚：**没有出现第二个 AI Hunter 级「搜全网公开网页」引擎**；扩出来的是四层东西——找人、触达、编排、爬虫底座。星标以 GitHub API 当日快照为准。

#### A. 找人 / 获客系统（可直接对标产品）

| 项目 | 链接 | Star / Fork | 协议 | 最近推送 | 定位 |
| --- | --- | --- | --- | --- | --- |
| OpenOutreach | https://github.com/eracle/OpenOutreach | 3015 / 560 | GPL-3.0 | 2026-09-07 | 描述即找人；持牌源 BetterContact；导出 CSV 给 Instantly/Smartlead。官网 https://openoutreach.app |
| YALC（开源 Clay） | https://github.com/Othmane-Khadri/YALC-the-GTM-operating-system | 304 / 90 | MIT | 2026-08-20 | CLI-first GTM OS；在现有 CRM/sequencer 上编排。官网 https://www.yalc.ai |
| b2b-sdr-agent-template | https://github.com/iPythoning/b2b-sdr-agent-template | 181 / 48 | MIT | 2026-08-20 | 基于 OpenClaw 的外贸 SDR 模板：10 阶段管道、WhatsApp+Telegram+Email。主页 https://pulseagent.io |
| Linki | https://github.com/moaljumaa/linki | 142 / 46 | Other | 2026-08-01 | 自托管 AI SDR：LinkedIn 序列 + 冷邮件，OpenRouter 接任意模型，Docker/SQLite |
| ai-outreach-engine | https://github.com/rohitmalhotra1420/ai-outreach-engine | 21 / 7 | MIT | 2026-08-06 | 爬官网 → 建受众 → 找联系人 → 起草邮件 → 人工审核后发送。最接近「搜网获客」的新仓 |
| OpenSales | https://github.com/siddartha19/OpenSales | 16 / 2 | 未声明 | 2026-04-20 | LangGraph 多智能体：VP 规划 / SDR 找公司(Exa) / AE 写邮件；停更约 5 个月 |
| Tanchi（探知） | https://github.com/tanchihq/tanchi | 12 / 1 | AGPL-3.0 | 2026-08-25 | 邮件优先的自主获客引擎；每晚 sourcing+研究+起草，人审队列。官网 https://tanchi.io |
| GTM Grid | https://github.com/badapplesdotdev/gtm-grid | 11 / 5 | FSL-1.1 | 2026-09-08 | 本地优先的可编程 GTM 表格（列=函数），接 Apollo/HubSpot/LeadMagic。演示 https://gtm-grid-web.vercel.app |

#### B. Claude Code / Agent 技能包（热，但不是搜网引擎）

| 项目 | 链接 | Star | 协议 | 最近推送 | 定位 |
| --- | --- | --- | --- | --- | --- |
| ai-sales-team-claude | https://github.com/zubair-trabzada/ai-sales-team-claude | **1356** | MIT | 2026-03-27 | 14 skills + 5 并行 agent：研究、BANT/MEDDIC、找决策人、5 封外联序列、会议准备、提案。社区最大，但停更半年 |
| Lead Hunter skill | https://aiskill.market/skills/lead-hunter | — | skill | — | OpenClaw 获客 skill：X / LinkedIn / GitHub / Product Hunt / Moltbook |
| firecrawl-lead-gen | https://github.com/firecrawl/firecrawl-workflows（skill 名 firecrawl-lead-gen） | — | skill | — | 从公开名录抽出 CRM 可用名单；拒绝绕过验证码 |
| clay-search skill | https://github.com/matteotitta/genesys-skills | — | skill | — | 用 Clay + Apollo 凭证批量搜人，去重后导出 |
| open-sales-stack | https://github.com/ekas-io/open-sales-stack | 2 / 2 | MIT | 2026-04-09 | B2B 研究 MCP：官网/技术栈/社媒/招聘/广告情报 |

#### C. 基础设施（真正能「大量搜索」的底座）

这些仓不是获客产品，但是搜网获客的产能上限。星标来自 2026-08-19 公开核对清单（[30 Open-Source Repositories](https://threadnavigator.com/thread/2090090583649861708)），落地前再核一次。

| 项目 | 链接 | 约 Star | 用来干什么 |
| --- | --- | --- | --- |
| Firecrawl | https://github.com/mendableai/firecrawl | ~16.9 万 | 网页 → Markdown/结构化，给 LLM 吃 |
| Browser Use | https://github.com/browser-use/browser-use | ~10.9 万 | Agent 操控真浏览器，翻没有 API 的名录 |
| Crawl4AI | https://github.com/unclecode/crawl4ai | ~7.8 万 | 为 RAG/AI 准备的爬虫 |
| ScrapeGraphAI | https://github.com/ScrapeGraphAI/Scrapegraph-ai | ~3.0 万 | 自然语言描述字段，直接出结构 |
| Crawlee | https://github.com/apify/crawlee | ~2.5 万 | 队列/重试/会话/代理，适合定期大批量 |
| GPT Researcher | https://github.com/assafelovic/gpt-researcher | ~2.9 万 | 多步网络研究，出报告 |
| Twenty CRM | https://github.com/twentyhq/twenty | ~5.5 万 | 开源 CRM，线索落地处 |

配套教程（不是产品，但是可抄的流水线）：

- Firecrawl + Claude 做 prospect brief：https://dev.to/dimanegodiuk/i-built-a-prospect-research-agent-with-firecrawl-and-claude-code-49b5
- Quora 线索 + Firecrawl + Google Sheets：https://github.com/GURPREETKAURJETHRA/AI-Lead-Generation-Agent
- Indie Hackers 介绍 ai-outreach-engine：https://www.indiehackers.com:8443/post/built-an-open-source-ai-outreach-engine-to-find-customers-jobs-investors-3aab68b36b

#### D. 本轮刻意不抬进主表的仓

| 仓 | 原因 |
| --- | --- |
| withoneai/gtm-lead-agent | 0★，2026-07-29 一次性提交 |
| NinjaPear-Shares/agentic-lead-generation | 0★，starter 空壳 |
| Dvbxtreme/ai-sales-agent | 4★，Next.js+Ollama demo |
| mguozhen/SolveaSDR | 营销文强、仓库弱，且偏语音外呼，不贴搜网 |

---

## ④ 国内商业竞品 / 选型对象

| 产品 | 链接 | 定位 |
| --- | --- | --- |
| NovaSea AI（启明海创，暨南大学团队） | https://cnews.chinadaily.com.cn/a/202609/14/WS6aa76934e4b09a165c789c63.html | AI 获客 / 出海 |
| Revor AI | https://revor.ai/zh | AI 外联 / 获客 |
| Revor Outreach Skill 文档 | https://revor.ai/zh/docs/skill/revor-outreach | 可对标本项目 skill 形态 |
| Revor 定价 | https://revor.ai/zh/pricing | 商业模式 |
| Revor 介绍 | https://www.tbadc.com/hao/revor-ai.html | 第三方评测 |
| 埃米 AIMI | https://baike.baidu.com/item/%E5%9F%83%E7%B1%B3AIMI/67823251 | 国内 AI 获客 |
| 智达明远 AI | https://big5.china.com.cn/gate/big5/business.china.com.cn/2026-08/04/content_43471609.shtml | 企业 AI 获客 |
| 智达明远 · 新京报 | https://www.bjnews.com.cn/detail/1785892004129000.html | 媒体报道 |
| 智达明远 · 中国日报财经 | https://caijing.chinadaily.com.cn/a/202606/29/WS6a421fd7a310d709c2fbab66.html | 媒体报道 |
| 惠米聚客 / 慧米云 | https://m.11467.com/product/d56637912.htm | AI 获客系统招商，含 GEO |
| 慧米云 · 工厂品牌引流 | https://m.11467.com/product/d56881901.htm | 一站式内容+GEO |
| 惠米聚客份额分析 | https://www.toutiao.com/article/7635138823827800582 | 本地生活 AI 私域 |
| 微盟 / 瞬维 AI / 无界 AI / 聚视推 选型 | https://www.163.com/dy/article/L6F4SA320556PD28.html | 公域矩阵获客对比 |
| 通用「3 天搭 AI 获客系统」文 | https://www.toutiao.com/article/7633760784841032244 | 方法论（非产品） |
| AB客 GEO | https://www.cnabke.com | 外贸 B2B GEO（社区文章中高频露出） |

---

## ⑤ 海外 AI 获客工具栈（对标上限）

社区和评测里反复出现的组合：**Clay（数据编排）+ Apollo.io（联系人）+ Instantly / Smartlead / Lemlist（冷邮件）+ ZoomInfo / Cognism / 6sense（意图数据）**。

- Best AI tools for lead generation：https://startupik.com/best-ai-tools-for-lead-generation-and-prospecting
- Best AI outreach tools：https://toolsbyrole.com/best-ai-outreach-tools-for-sales
- Best AI BDR tools 2026：https://instantly.ai/blog/best-ai-bdr-tools-platforms-2026
- AI B2B sales leadgen 2026：https://en.ai-pedias.com/blog/ai-b2b-sales-leadgen-2026
- Automated lead generation with AI：https://www.masternodeai.com/en/systems/automated-lead-generation-with-ai

补充常见单品（评测文中反复出现，官网可直接跟）：Clay、Apollo.io、Instantly.ai、Smartlead、Lemlist、ZoomInfo、Cognism、6sense、Tidio、Lavender。

---

## ⑥ 关键技术文章（OpenClaw 获客 + GEO）

### OpenClaw / 养虾 / 外贸获客

- 百科式拆解：https://www.ai-indeed.com/encyclopedia/19464.html
- 53AI 专题：https://www.53ai.com/news/Openclaw/2026041013850.html
- 博客园实操：https://www.cnblogs.com/wgwyanfs/p/19962884
- 51wheatsearch 转载：https://www.51wheatsearch.com/4/2/2231491
- Cocoloop 讨论：https://www.cocoloop.cn/t/topic/2508
- Cocoloop 续帖：https://www.cocoloop.cn/t/topic/296/4
- 阿里云开发者：https://developer.aliyun.com/article/1711718
- 养虾运营：https://www.163.com/dy/article/KNTIK1JP0511DBV1.html
- 搞钱实操：https://post.smzdm.com/p/am9eqw24/
- 记忆可插拔：https://m.thepaper.cn/newsDetail_forward_32732273
- 责任承担：https://wap.ce.cn/xwzx/gnsz/gdxw/202603/t20260315_2829263.shtml
- 为什么养龙虾：http://www.jnxc.gov.cn/show-19-8645-1.html
- 马化腾没想到：https://www.toutiao.com/article/7614858908926870054
- 副业赚钱：https://www.toutiao.com/w/1859153399369736/
- 一天一个 AI 工具 · OpenClaw：https://www.toutiao.com/a7643382793104065034

### GEO（Generative Engine Optimization）

GEO 是社区从「搜营销线索」自然联想到的第二条腿：不只把线索搜出来，还要让自己的品牌被 AI 搜索推荐出去。

- 商业新知：https://www.shangyexinzhi.com/article/34024129.html
- What is GEO：https://seo.whoops.com.tw/what-is-geo/
- 中国发展网：https://m.chinadevelopment.com.cn/checklist.php?s=index/detail/id/2003893
- 课程向介绍：https://www.hdcourse.com/seo/geo/
- Yotpo 英文定义：https://www.yotpo.com/blog/what-is-geo/

---

## ⑦ 联想搜索：社区里反复出现、值得跟进的关键词

后续如果继续挖，优先用这些词，而不是只搜「AI 获客」：

| 关键词 | 为什么出现 |
| --- | --- |
| OpenClaw / Clawdbot / Moltbot / 养虾 / 养龙虾 | 执行层 Agent，外贸圈爆款 |
| GEO / Generative Engine Optimization / AI 推荐池 | 让品牌进入 AI 答案 |
| AI Hunter / AI_Find_Customer / 全网搜客户 | 最接近本项目的开源实现 |
| IMAI.WORK / 私域获客 / 企微 SCRM | 国内 IM 获客 |
| HiveMtk / 私域营销操作系统 | Go 技术栈开源 OS |
| 一人公司 / OPC / 4A 架构 | OpenClaw 叙事 |
| Clay + Apollo + Instantly | 海外标准获客栈 |
| outreach-skill / ClawHub | 技能市场化 |
| Token 黑洞 / ClawHavoc / 技能投毒 | 安全与成本，产品必须处理 |
| 上门安装 OpenClaw | 现成获客/变现场景（300–800 元/次） |

---

## ⑧ 对本项目的可执行建议

1. **产品形态不要做成「第二个 OpenClaw」。** OpenClaw 是手脚，本项目应做「搜 → 抽线索 → 评分 → 触达」的大脑。执行可以接 OpenClaw Skill 或浏览器。
2. **开源对标优先抄 AI_Find_Customer 的多智能体流水线**（Insight→KeywordGen→Search→LeadExtract→Evaluate），搜网底座接 Firecrawl / Browser Use / Crawl4AI，找人可旁路接 OpenOutreach，触达抄 Linki 或交给 Instantly，编排层看 YALC。再用 IMAI.WORK / HiveMtk 补私域。
3. **搜索源不要只盯 7 个社区。** 社区自己已经在用：GitHub/Gitee、海关数据、B2B 平台、Telegram 频道、X/LinkedIn、官网联系页。本清单里的联想词就是下一轮爬取字典。
4. **必须内置 GEO 视角。** 国内竞品（惠米、AB客、NovaSea）都在卖「让 AI 推荐你」；纯「搜别人」会很快同质化。
5. **安全与合规是卖点不是附录。** 养虾翻车（公网暴露、信用卡盗刷、ClawHub 投毒、Token 烧钱）已经上了主流媒体。如果做 Agent 执行，默认本地、默认低权限、默认二次确认。

---

*本清单为 2026-09-17 检索快照。部分 `site:` 结果被聚合站改写，落地前建议再点开一次确认是否仍可访问。*
