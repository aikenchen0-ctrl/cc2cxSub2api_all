export default {
  accountWeights: {
    title: '账号权重',
    description: '查看调度评分快照及选择账号时使用的运行指标。',
    refresh: '刷新',
    searchPlaceholder: '搜索账号名称或平台',
    sortDescending: '从大到小',
    sortAscending: '从小到大',
    allStatuses: '全部状态',
    status: { active: '启用', inactive: '停用', error: '错误' },
    sort: {
      base: '基础分', sticky: '粘性分', priority: '优先级', priorityFactor: '优先级因子',
      loadFactor: '负载因子', queueFactor: '队列因子', errorRateFactor: '错误率因子',
      ttftFactor: 'TTFT 因子', resetFactor: '重置因子', quotaHeadroomFactor: '配额余量因子',
      upstreamCostFactor: '上游成本因子', concurrency: '当前并发', capacity: '容量',
      multiplier: '账号倍率', name: '名称'
    },
    summary: { total: '账号总数', scored: '有评分账号', schedulable: '可调度账号', highestBase: '最高基础分' },
    columns: {
      account: '账号', platformType: '平台 / 类型', baseScore: '基础分', stickyScore: '粘性分',
      priorityRaw: '优先级原值', priorityFactor: '优先级因子', concurrencyCapacity: '并发 / 容量',
      loadFactor: '负载因子', queueFactor: '队列因子', errorRateFactor: '错误率因子', ttftFactor: 'TTFT 因子',
      resetFactor: '重置因子', quotaHeadroomFactor: '配额余量因子', upstreamCostFactor: '上游成本因子',
      multiplier: '账号倍率', status: '状态', groups: '分组'
    },
    baseUrlMissing: '未配置 Base URL',
    groupFallback: '组 {id}',
    empty: '暂无符合条件的账号',
    factorNote: '因子为 0-1 归一化调度得分，越大越有利；优先级原值越小越优先。管理端快照中的错误率与 TTFT 当前采用调度器中性默认值。'
  }
}
