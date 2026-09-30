export default {
  upstreamAudit: {
    title: '上游审计',
    description: '扫描上游账号的可用模型，并用 ModelTrace 指纹库核验模型身份。',
    eyebrow: 'MODELTRACE / 上游模型身份核验',
    heroTitle: '确认上游返回的模型，是否与声明一致',
    heroDescription: '系统会读取 Sub2API 内全部上游账号及其模型，仅对 ModelTrace 已收录的模型发起探针，再比较输出指纹与参考模型的相似度。',
    startScan: '扫描账号模型',
    running: '审计进行中',
    loadFailed: '上游审计数据加载失败',
    refreshAccounts: '刷新账号数',
    metrics: {
      accounts: '上游账号',
      supported: '指纹模型',
      matched: '待检测模型',
      lastRun: '最近审计',
      loading: '读取中',
      pending: '扫描后生成',
      never: '尚未运行'
    },
    flowTitle: '审计工作流',
    flowDescription: '每次审计展示账号、声明模型、候选模型、相似度、有效输出数和探针次数。',
    flow: {
      syncTitle: '同步账号与模型',
      syncDescription: '从 Sub2API 读取全部账号，并刷新每个账号的上游模型列表。',
      matchTitle: '匹配指纹库',
      matchDescription: '只保留 ModelTrace 已支持的 GPT 与 Claude 模型，其他模型标记为暂不支持。',
      probeTitle: '发送探针请求',
      probeDescription: '使用账号自身凭据调用对应模型，解析数字序列并仅保留诊断统计。',
      scoreTitle: '计算相似度',
      scoreDescription: '对比统一指纹库，给出第一候选、闭集权重和证据强度。'
    },
    tabs: {
      queue: '检测队列',
      library: '指纹模型库',
      settings: '审计策略'
    },
    queue: {
      title: '账号模型检测队列',
      description: '扫描完成后，所有命中的账号模型会在这里排队检测。',
      emptyTitle: '还没有检测任务',
      emptyDescription: '点击“扫描账号模型”，系统会同步全部账号模型并自动检测指纹库已支持的模型。',
      scanErrors: '{count} 个账号未能读取模型列表',
      columns: {
        account: '上游账号',
        declared: '声明模型',
        prediction: '指纹候选',
        similarity: '相似度',
        status: '状态',
        action: '操作'
      }
    },
    progress: {
      title: '本次审计进度',
      accounts: '账号 {done}/{total}',
      models: '模型 {done}/{total}',
      running: '运行中',
      completed: '已完成'
    },
    status: {
      pending: '等待',
      probing: '探测中',
      verified: '一致',
      mismatch: '不一致',
      failed: '失败'
    },
    library: {
      title: 'ModelTrace 指纹模型库',
      description: '来自 ModelTrace 统一指纹库快照，构建时间 2026-09-23，共 16 个候选模型。',
      gpt: 'GPT 系列',
      claude: 'Claude 系列',
      models: '个模型',
      snapshot: '指纹库 SHA-256',
      calibration: '当前快照尚未完成同上下文及多语言独立校准。'
    },
    settings: {
      title: '默认审计策略',
      description: '当前审计使用以下固定策略执行，并保留每个模型的探针次数与评分结果。',
      scopeTitle: '扫描范围',
      scopeDescription: '扫描 Sub2API 中全部账号，并对每个账号的全部上游模型取交集。',
      retriesTitle: '异常复测',
      retriesDescription: '每个模型最多请求 6 次，目标收集 3 个有效数字序列；不足 3 个时按现有有效序列校准评分。',
      languagesTitle: '探针语言',
      languagesDescription: '使用 ModelTrace 数字选择探针；当前指纹库尚未完成多语言独立校准。',
      thresholdTitle: '结果表达',
      thresholdDescription: '展示候选权重与证据强度，不把实验性相似度直接表述为模型替换证明。'
    },
    noticeTitle: '结果边界',
    notice: 'ModelTrace 是闭集指纹比较：高相似度表示输出更接近库内某个候选模型，不等同于确认供应商替换、降级或欺诈。正式告警应结合复测、账号日志与上游响应头。'
  }
}
