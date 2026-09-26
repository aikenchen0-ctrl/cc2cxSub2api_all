# Sub2API 上游审计

这是独立的 Sub2API 卫星应用。扫描通过现有管理 HTTP API 读取账号列表、账号详情和账号级 usage 快照；探针只走普通网关 API Key。Anthropic 账号使用 `/v1/messages`，其他账号使用 `/v1/chat/completions`。OAuth/setup-token 探针采用能通过 Sub2API Claude Code 识别的 UA 与 metadata，避免被注入 system prompt。项目不访问账号导出接口，也不调用 `POST /api/v1/admin/accounts/:id/test`。

账号钉号同时使用稳定的 `X-Session-Id` 和 Sub2API 现有识别格式的 `metadata.user_id`（其中 `device_id` 由本地 session secret 派生）。Session 只能让后续请求粘到首次选中的账号，不能指定首次选择哪个账号。因此探针只使用恰好包含一个 active 且 schedulable 上游账号的 group，并在该独占 group 中成功完成校准请求后按路由条件归因。共享 group 不会被用于探测，对应检查记为 `unpinned`；失败的网关请求也不会被归因。请为每个待审账号准备独占 group，并在配置中提供该 group 的普通网关 API Key 和模型档案。状态为 active 但 `schedulable=false` 的账号会记录为 `skipped`，因为网关不会再把请求路由给它。

扫描只使用 `GET /api/v1/admin/accounts`、`GET /api/v1/admin/accounts/:id` 和 `GET /api/v1/admin/accounts/:id/usage` 读取账号数据；不查询 `/api/v1/admin/usage`，也不靠逐请求账单记录归因。SSO 交换和后续管理员会话校验还会通过既有 `GET /api/v1/admin/users/:id` 核验管理员身份，因为 SSO ticket 提供身份但不携带角色。自动停用默认关闭且不预选规则；唯一可选规则是经过独立第二轮同候选确认的高置信 ModelTrace 降档。

四个检查分别记录：

1. ModelTrace 指纹归因；
2. Say hi 的本地 input token 估算与网关 usage 差值；
3. 相同请求体的缓存写入、读取；逐请求费用不可用时标为无法判定；
4. 有可核实的上游 token 配额增量时计算观察倍率，账单金额只作辅助诊断。

报告按检查分别保存。数据库和 JSON 报告不保存原始探针内容、管理 Key、普通 API Key、账号凭据或原始上游响应。探针调用日志保存 request ID、路由归因、状态和网关用量数字；倍率检查把 usage 快照窗口内的 Say hi 与缓存探针价格估算记入 `estimated_probe_cost_usd`，钉号校准估算单列为 `estimated_pin_calibration_cost_usd`，两者可用时合计写入 `estimated_total_usage_aware_probe_cost_usd`。这些是参考价估算，不是 Sub2API 实际扣费；ModelTrace 不读取回包 usage，其本地计价估算单列在 ModelTrace 检查结果中，不计入 usage-aware 总额。未能按独占 group 归因的请求不会用于模型或费用结论。

## 配置

复制 .env.example 为 .env，填入：

- SUB2API_ADMIN_API_KEY：服务端使用的管理 API Key；仅在没有此 Key 时才用 SUB2API_ADMIN_JWT；
- SUB2API_SSO_SECRET：与 Sub2API 完全一致的 SSO secret；
- MODELAUDIT_SESSION_SECRET：独立的本地 Session 签名 secret，至少 32 字节；
- SUB2API_GATEWAY_API_KEYS：按 group ID 映射的普通用户网关 API Key；
- MODELAUDIT_GROUP_PROFILES：每个 group 的探针模型和规则档案。

Group 档案示例。价格需与 Sub2API 对该公开模型使用的价格口径一致；示例数字为零，不代表实际价格。

    {
      "12": {
        "model": "gpt-5.5",
        "expected_model": "gpt-5.5",
        "cache_protocol": "openai",
        "cache_min_prefix_tokens": 1024,
        "pricing_model": "gpt-5.5",
        "gateway_cost_multiplier": 1,
        "expected_provider_cost_multiplier": 1
      }
    }

`cache_protocol` 可设为 `openai` 或 `anthropic`。Anthropic 模型会在相同请求体中带 `cache_control: ephemeral`。缓存读写 token 检查会继续执行，并根据配置价格估算预期费用；允许的账号接口不提供逐请求网关费用，因此缓存费用核对始终为 `inconclusive`，不能据此声称已验证 Sub2API 是否按缓存价计费。

使用 `grok_token_quota` 的同窗剩余量差值除以已钉号探针回包的 input、output 与 cache token 总量，计算 token 配额倍率。当前只有 Grok 提供可核验的新鲜配额计数；该比值会与名义 1.0、账号 `rate_multiplier` 和配置的 `expected_provider_cost_multiplier` 分别比较。通用配额百分比和 Sub2API 本地 `window_stats` 不充当上游 token 计数；缺少可靠计数时标为 `inconclusive`。
美元账单倍率另行计算：同周期完整供应商账单增量 ÷ LiteLLM 价目估算的已钉号探针成本，再与账号 `rate_multiplier` 和 `expected_provider_cost_multiplier` 分别比较。当前只有 Grok 提供可用的账号级美元计数器；账单不完整或价目缺失时为 `inconclusive`。
默认扫描间隔为 60 分钟，仅接受 30 或 60 分钟；缓存前缀阈值小于 32000 tokens。每个账号的探针串行执行，不同账号默认最多并行 4 个。ModelTrace 从本地闭集指纹库生成三条挑战，不运行网页、不启用 Codex 插件、不附带 Claude Code system prompt。运行时使用同时绑定指纹库和分类器源码 SHA-256 的逐候选相似度阈值；校准文件缺失、过期或置信阈值低于校准基线时一律显示 `unknown`。未过置信门槛时隐藏候选型号；高置信候选与预期不同时告警，候选吻合但证据不足时显示 `unknown`，未知结果不会触发自动停用。

Anthropic 账号通过原生 `/v1/messages` 探针；OAuth/setup-token 请求使用 `claude-cli/2.1.161 (modelaudit probe)` User-Agent 与稳定的 `metadata.user_id`，只发送 user 消息，不含 system 字段。当前 Sub2API Messages 转发按该 UA 与 metadata 识别探针并保留请求体；若识别规则变化，需要重新验证。

隐藏 input 检查发送无 system prompt 的 `Say hi`，本地 tokenizer 估算 expected，再减去回包实际 input usage，保留 `expected − actual` 符号。OpenAI prompt tokens 含 cached tokens，只计一次；Anthropic input、cache creation、cache read 分别处理。绝对差值大于 20 标黄、大于 100 标红、其余视为正常误差；它是筛查信号，不是单次判决。

可用 `python modelaudit/tools/evaluate_modeltrace_thresholds.py` 离线复核阈值，不会发起 API 请求。统一相似度阈值 `0.90` 在原留出法中只放行 5/192 个已知型号样本。当前逐候选阈值采用四折环境隔离：每折分开训练、阈值校准和验证环境；已知型号 186/192 个首选正确，其中 11/192 个达到识别门槛；逐型号移除的库外代理样本 0/192 个通过。运行时阈值文件与指纹库及分类器源码 SHA-256 绑定；任一变化后需用 `--write-calibration modelaudit/vendor/modeltrace/candidate_thresholds.json` 重新生成。逐型号移除仍只是代理未知样本，零次误放行不证明真实库外识别能力，也不能替代新增模型实测。

`MODELAUDIT_MODELTRACE_MIN_PROFILE_SIMILARITY` 是额外的全局相似度下限，默认 `0` 表示使用逐候选校准值；将它设得更高会更保守。运行时概率和候选差距阈值不能低于校准文件基线，否则结果标为 `unknown`。

扫描只读取账号列表、详情和账号 usage，不读取逐请求 usage 账单。供应商计数器是账号级累计值，可能包含并发请求，所以这些结果是筛查信号，不代表单次探针账单，也不会触发自动停用。ModelTrace 不读取回包 `usage`；挑战调用的参考成本单独按本地 tokenizer 统计请求和响应文本，再用配置的 LiteLLM 或手动价目估算，写入 ModelTrace 检查结果。这个数不是实际扣费，也不参与倍率对账；原始挑战和响应文本不会写入数据库或报告。
Docker 镜像包含 ModelTrace 的 fingerprint.py、unified_bank.json 和原 MIT 许可证副本。工作区运行优先读取仓库 ModelTrace/，镜像使用 modelaudit/vendor/modeltrace/。

价格默认按 `pricing_model`（未配置时使用 `model`）从固定 LiteLLM 快照查找，手动 `pricing` 可逐项覆盖，单位为 USD/百万 token。版本、提交和文件哈希见 `vendor/litellm/NOTICE.md`。

## 本地启动

    cd modelaudit
    Copy-Item .env.example .env
    # 编辑 .env：本地 HTTP 时将 MODELAUDIT_COOKIE_SECURE 设为 false
    python -m venv .venv
    .venv\Scripts\Activate.ps1
    pip install -r requirements.txt
    uvicorn app.main:app --host 127.0.0.1 --port 8077

将 Sub2API 的 modelaudit_link 指向 http://localhost:8077，并在浏览器中通过 Sub2API 左侧菜单进入。

面板顶部可选择已配置的上游模型；扫描会逐个检查支持该模型的独占上游账号，并在“上游真实模型归因”区域展示 ModelTrace 相似度前三名。留空表示扫描全部已配置模型。

## Docker

该 compose 文件默认连接本地 Sub2API 的 `deploy_sub2api-network`。若线上主 Compose 使用其它网络名，在 `.env` 中把 `MODELAUDIT_SUB2API_NETWORK` 设为实际网络名；仓库 `docker-compose.online.yml` 当前使用 `sub2api-deploy_sub2api-network`：

    cd modelaudit
    cp .env.example .env
    # 在 .env 中设置服务端 Key、SSO secret 和普通网关 Key；线上部署还要设置 MODELAUDIT_SUB2API_NETWORK
    docker compose up -d --build

容器默认只在宿主机 127.0.0.1:8077 暴露端口，并加入 Sub2API Docker 网络。若反向代理也在该网络内，可将 modelaudit 反向代理到 modelaudit:8077；若反代运行在宿主机，可代理到 127.0.0.1:8077。线上 Sub2API 应将 `MODELAUDIT_SSO_CALLBACK_URL` 设为 `https://modelaudit.cc2.cx/api/auth/sso/callback`；线上 `modelaudit_link` 应配置为 `https://modelaudit.cc2.cx`，ModelAudit 的 `MODELAUDIT_ALLOWED_ORIGINS` 应包含该地址。HTTPS 下保持 `MODELAUDIT_COOKIE_SECURE=true`；本地 HTTP 可设为 false。

报告自动写入 data/reports/<run-id>.json，SQLite 数据库写入 data/modelaudit.sqlite3。备份前停止容器或复制 SQLite WAL 与数据库文件。

账号并发遵循 Sub2API 语义：配置值为 0 时表示不限并发，正数上限由网关槽位控制。审计器对每个账号的探针串行执行。

## 自动停用

默认不预选任何停用规则；管理员需要开启总开关并明确选择 ModelTrace 触发规则。ModelTrace 首轮高置信度不符会再执行一组独立三挑战，第二轮必须以相同候选再次判为高置信度不符，自动停用才具备资格。确认轮只在首轮红色不符时运行，会额外消耗三次探针调用。其他红色检查仅告警。

面板默认关闭自动停用。开启后，只有满足确认条件且勾选 ModelTrace 规则的红色告警才会调用既有的 POST /api/v1/admin/accounts/:id/schedulable，把账号设为不可调度。不会自动恢复账号；管理员可在 Sub2API 中自行重新启用。
