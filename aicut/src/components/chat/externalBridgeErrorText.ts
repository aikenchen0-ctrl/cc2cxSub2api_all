type Translate = (key: string, params?: Record<string, string | number>) => string;

/** Render transport diagnostics separately from workflow/task failures. */
export function externalBridgeErrorText(message: string, t: Translate): string {
  const match = /^(registration|poll|cancellation poll|result) failed: (network|HTTP (\d+))$/.exec(message);
  const stages: Record<string, string> = {
    registration: '注册编辑器连接',
    poll: '接收编辑任务',
    'cancellation poll': '同步任务取消状态',
    result: '回传编辑结果',
  };
  const stage = t(stages[match?.[1] ?? ''] ?? '同步编辑器连接');
  const status = Number(match?.[3]);
  if (status === 401) return t('编辑器连接登录已失效，请从左侧菜单重新进入 AI剪辑。');
  if (status === 403) return t('编辑器连接无访问权限，请从左侧菜单重新进入 AI剪辑。');
  if (status === 409) return t('编辑器连接正在恢复，请稍候；若持续失败，请关闭同一工程的其他窗口。');
  if (match?.[2] === 'network' || /^(Failed to fetch|fetch failed|Load failed|NetworkError.*)$/i.test(message)) {
    return t('{stage}失败：无法连接服务，正在自动重连。请检查网络；若持续失败，请稍后重试。', { stage });
  }
  if (match) return t('{stage}失败（HTTP {status}），正在自动重连；若持续失败，请联系管理员。', { stage, status });
  // Do not expose arbitrary browser or server diagnostics as the alert itself.
  // Existing localized action/proposal errors remain actionable.
  if (/[\u3400-\u9fff]/u.test(message)) return t(message);
  return t('编辑器同步失败，请稍后重试；若持续失败，请联系管理员。');
}
