const state = {
  settings: null,
  dashboard: null,
  refreshTimer: null,
}

const names = {
  modeltrace: '模型真伪',
  token_delta: '输入 token 差值',
  cache: '缓存与费用',
  multiplier: '倍率审计',
}

const statusLabels = {
  ok: '正常',
  alert: '告警',
  unknown: '未知',
  inconclusive: '无法判定',
  unpinned: '未固定',
  error: '错误',
  not_configured: '未配置',
  skipped: '已跳过',
}

function byId(id) {
  return document.getElementById(id)
}

function toast(message) {
  const node = byId('toast')
  node.textContent = message
  node.classList.add('show')
  window.clearTimeout(toast.timer)
  toast.timer = window.setTimeout(() => node.classList.remove('show'), 3200)
}

async function api(path, options = {}) {
  const response = await fetch(path, {
    credentials: 'same-origin',
    cache: 'no-store',
    ...options,
    headers: {
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
      ...(options.headers || {}),
    },
  })
  if (response.status === 401 || response.status === 403) {
    throw Object.assign(new Error('需要从 Sub2API 管理员菜单登录'), { unauthorized: true })
  }
  const body = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(body.detail || ('请求失败 (' + response.status + ')'))
  return body
}

function pill(status, severity) {
  const element = document.createElement('span')
  const tone = severity === 'red' ? 'red'
    : severity === 'yellow' ? 'yellow'
      : severity === 'normal' || status === 'ok' ? 'normal'
        : status === 'unpinned' ? 'blue' : 'neutral'
  element.className = 'status-pill ' + tone
  element.textContent = statusLabels[status] || status || '未知'
  return element
}

function formatTime(raw) {
  if (!raw) return '—'
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date)
}

function formatUsd(value) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '无法估算'
  return '$' + value.toFixed(6)
}

function appendDetails(cell, values) {
  const usable = values.filter(Boolean)
  if (!usable.length) return
  const detail = document.createElement('div')
  detail.className = 'account-meta'
  detail.textContent = usable.join(' · ')
  cell.append(detail)
}

function checkCell(check) {
  const cell = document.createElement('td')
  cell.className = 'check-cell'
  if (!check) {
    cell.append(pill('未扫描', 'none'))
    return cell
  }
  cell.append(pill(check.status, check.severity))
  const summary = document.createElement('div')
  summary.className = 'check-summary'
  summary.textContent = check.summary || names[check.probe] || ''
  cell.append(summary)
  if (check.probe === 'token_delta') {
    const delta = check.metrics?.delta_expected_minus_actual
    appendDetails(cell, [typeof delta === 'number' ? 'expected − actual: ' + delta + ' tokens' : ''])
  }
  if (check.probe === 'modeltrace') {
    const metrics = check.metrics || {}
    const candidate = metrics.observed_candidate
    const reason = check.evidence?.reason
    appendDetails(cell, [
      candidate ? '识别型号 ' + candidate : '',
      check.expected_model ? '预期 ' + check.expected_model : '',
      typeof metrics.estimated_modeltrace_cost_usd === 'number'
        ? 'ModelTrace 本地计价预估 ' + formatUsd(metrics.estimated_modeltrace_cost_usd)
        : '',
      Number.isInteger(metrics.estimated_modeltrace_challenges)
        ? '挑战 ' + metrics.estimated_modeltrace_challenges + ' 次'
        : '',
      metrics.estimated_modeltrace_cost_status === 'partial_estimate' ? '费用估算不完整' : '',
      ['price_or_multiplier_unavailable', 'unavailable'].includes(metrics.estimated_modeltrace_cost_status)
        ? '缺少价目，无法估算挑战费用'
        : '',
      check.status === 'unknown' && reason ? '原因 ' + reason : '',
    ])
  }
  if (check.probe === 'cache') {
    const metrics = check.metrics || {}
    const cache = metrics.cache_check || {}
    const cost = metrics.cost_check || {}
    appendDetails(cell, [
      Number.isInteger(cache.first_cache_write_tokens) ? '写入 ' + cache.first_cache_write_tokens : '',
      Number.isInteger(cache.second_cache_read_tokens) ? '读取 ' + cache.second_cache_read_tokens : '',
      typeof cost.expected_gateway_cost === 'number' ? '参考成本 ' + formatUsd(cost.expected_gateway_cost) : '',
      cost.status === 'inconclusive' ? '实际扣费无法核验' : '',
    ])
  }
  if (check.probe === 'multiplier') {
    const metrics = check.metrics || {}
    const invoice = metrics.invoice_diagnostic || {}
    const details = []
    if (typeof metrics.estimated_probe_cost_usd === 'number') {
      details.push('倍率窗口预估 ' + formatUsd(metrics.estimated_probe_cost_usd))
    }
    if (typeof metrics.estimated_pin_calibration_cost_usd === 'number') {
      details.push('钉号标定预估 ' + formatUsd(metrics.estimated_pin_calibration_cost_usd))
    }
    if (typeof metrics.estimated_total_usage_aware_probe_cost_usd === 'number') {
      details.push('已计价探针合计 ' + formatUsd(metrics.estimated_total_usage_aware_probe_cost_usd))
    }
    if (typeof metrics.observed_usage_multiplier === 'number') {
      details.push('上游 token 配额倍率 ' + metrics.observed_usage_multiplier.toFixed(2) + '×')
    }
    if (typeof invoice.observed_cost_multiplier === 'number') {
      details.push('上游账单倍率 ' + invoice.observed_cost_multiplier.toFixed(2) + '×')
      const labels = {
        configured_account_rate_multiplier: '账号倍率',
        expected_provider_cost_multiplier: '预期倍率',
      }
      for (const [key, label] of Object.entries(labels)) {
        const comparison = invoice.comparisons?.[key]
        if (typeof comparison?.observed_to_expected_ratio === 'number') {
          details.push('对' + label + ' ' + comparison.observed_to_expected_ratio.toFixed(2) + '×')
        }
      }
    }
    appendDetails(cell, details)
  }
  return cell
}

function renderAccounts(data) {
  const body = byId('accounts-body')
  body.replaceChildren()
  if (!data.length) {
    const row = document.createElement('tr')
    const empty = document.createElement('td')
    empty.colSpan = 7
    empty.className = 'empty'
    empty.textContent = '暂无账号。扫描时会自动读取 Sub2API 中 active 的上游账号。'
    row.append(empty)
    body.append(row)
    return
  }
  for (const account of data) {
    const row = document.createElement('tr')
    const nameCell = document.createElement('td')
    const name = document.createElement('div')
    name.className = 'account-name'
    name.textContent = account.name
    const meta = document.createElement('div')
    meta.className = 'account-meta'
    meta.textContent = '#' + account.id + ' · ' + account.platform + ' · group ' + (account.group_ids.join(', ') || '—')
    nameCell.append(name, meta)
    row.append(nameCell)
    const byProbe = Object.fromEntries((account.checks || []).map((item) => [item.probe, item]))
    for (const key of ['modeltrace', 'token_delta', 'cache', 'multiplier']) {
      const item = byProbe[key] ? { ...byProbe[key], probe: key } : null
      row.append(checkCell(item))
    }
    const accountStatus = document.createElement('td')
    accountStatus.append(pill(account.schedulable ? '可调度' : '已停用', account.schedulable ? 'normal' : 'yellow'))
    const statusMeta = document.createElement('div')
    statusMeta.className = 'account-meta'
    statusMeta.textContent = '\u5e76\u53d1 ' + account.current_concurrency + '/' + (account.concurrency > 0 ? account.concurrency : '\u65e0\u9650') + ' \u00b7 \u500d\u7387 ' + (account.rate_multiplier ?? '\u2014')
    accountStatus.append(statusMeta)
    row.append(accountStatus)
    const actionCell = document.createElement('td')
    if (account.schedulable) {
      const stop = document.createElement('button')
      stop.className = 'button button-quiet'
      stop.type = 'button'
      stop.textContent = '停用'
      stop.addEventListener('click', () => stopAccount(account.id))
      actionCell.append(stop)
    } else {
      actionCell.append(pill('已停用', 'yellow'))
    }
    row.append(actionCell)
    body.append(row)
  }
}

function renderAttribution(accounts) {
  const container = byId('attribution-results')
  container.replaceChildren()
  const rows = accounts.flatMap((account) => {
    const check = (account.checks || []).find((item) => item.probe === 'modeltrace')
    const candidates = (check?.metrics?.probabilities || []).slice(0, 3)
    return candidates.length ? [{ account, candidates }] : []
  })
  if (!rows.length) { const empty = document.createElement('p'); empty.className = 'empty'; empty.textContent = '暂无可展示的归因结果。'; container.append(empty); return }
  for (const { account, candidates } of rows) {
    const card = document.createElement('article'); card.className = 'attribution-item'
    const heading = document.createElement('div'); heading.className = 'attribution-heading'
    heading.innerHTML = '<strong></strong><span></span>'
    heading.querySelector('strong').textContent = account.name
    heading.querySelector('span').textContent = '#' + account.id + ' · ' + (account.platform || '')
    card.append(heading)
    for (const candidate of candidates) {
      const probability = Math.max(0, Math.min(1, Number(candidate.probability) || 0))
      const similarity = Math.max(0, Math.min(1, Number(candidate.profile_similarity) || 0))
      const row = document.createElement('div'); row.className = 'attribution-row'
      row.innerHTML = '<div class="attribution-label"><span></span><b></b></div><div class="attribution-track"><i></i></div><small></small>'
      row.querySelector('.attribution-label span').textContent = candidate.model || '未知候选'
      row.querySelector('.attribution-label b').textContent = (similarity * 100).toFixed(1) + '%'
      row.querySelector('i').style.width = (similarity * 100).toFixed(1) + '%'
      row.querySelector('small').textContent = '相似度 · 置信度 ' + (probability * 100).toFixed(1) + '%'
      card.append(row)
    }
    container.append(card)
  }
}

function renderAlerts(alerts) {
  const container = byId('alerts')
  container.replaceChildren()
  if (!alerts.length) {
    const empty = document.createElement('p')
    empty.className = 'empty'
    empty.textContent = '暂无告警'
    container.append(empty)
    return
  }
  for (const alert of alerts.slice(0, 12)) {
    const item = document.createElement('div')
    item.className = 'alert-item'
    const copy = document.createElement('div')
    const title = document.createElement('div')
    title.className = 'alert-title'
    title.textContent = alert.message
    const meta = document.createElement('div')
    meta.className = 'alert-meta'
    meta.textContent = '账号 #' + alert.account_id + ' · ' + alert.rule_id + ' · ' + formatTime(alert.created_at) + (alert.action_taken ? ' · ' + alert.action_taken : '')
    copy.append(title, meta)
    item.append(copy, pill(alert.status === 'warning' ? '未知' : alert.status === 'open' ? '待处理' : alert.status, alert.severity))
    container.append(item)
  }
}

function renderGroups(groups) {
  const container = byId('groups')
  container.replaceChildren()
  if (!groups.length) {
    const empty = document.createElement('p')
    empty.className = 'empty'
    empty.textContent = '请配置 group ID 对应的普通 API Key 和模型档案。'
    container.append(empty)
    return
  }
  for (const group of groups) {
    const item = document.createElement('div')
    item.className = 'group-item'
    const copy = document.createElement('div')
    const name = document.createElement('div')
    name.className = 'group-name'
    name.textContent = 'Group ' + group.group_id + ' · ' + (group.model || '未配置模型')
    const details = document.createElement('div')
    details.className = 'group-details'
    details.textContent = (group.cache_protocol || 'cache 未配置') + ' · 前缀 ' + (group.cache_min_prefix_tokens || '—') + ' tokens · ' + (group.prices_configured ? '价格已配置' : '价格未配置')
    copy.append(name, details)
    item.append(copy, pill(group.key_configured ? 'API Key 已配置' : '缺少 API Key', group.key_configured ? 'normal' : 'yellow'))
    container.append(item)
  }
}

function setSettings(settings) {
  state.settings = settings
  byId('interval').value = String(settings.interval_minutes)
  byId('scheduler-enabled').checked = Boolean(settings.scheduler_enabled)
  byId('auto-stop-enabled').checked = Boolean(settings.auto_stop_enabled)
  const selected = new Set(settings.stop_rules || [])
  document.querySelectorAll('.stop-rule').forEach((input) => { input.checked = selected.has(input.value) })
  const target = byId('target-model')
  if (target) target.value = settings.target_model || ''
}

async function loadModels() {
  const data = await api('/api/models')
  const select = byId('target-model')
  select.replaceChildren()
  const all = document.createElement('option'); all.value = ''; all.textContent = '全部已配置模型'; select.append(all)
  for (const model of data.models || []) { const option = document.createElement('option'); option.value = model; option.textContent = model; select.append(option) }
}

function renderDashboard(data) {
  state.dashboard = data
  byId('account-count').textContent = String(data.accounts.length)
  byId('run-time').textContent = formatTime(data.latest_run?.finished_at || data.latest_run?.started_at)
  byId('alert-count').textContent = String(data.alerts.filter((item) => item.status === 'open').length)
  byId('scan-state').textContent = data.scan_running ? '扫描中' : data.settings.scheduler_enabled ? '定时开启' : '定时关闭'
  byId('run-status').replaceChildren(pill(data.latest_run?.status || '等待数据', data.latest_run?.status?.startsWith('completed') ? 'normal' : 'none'))
  renderAccounts(data.accounts)
  renderAttribution(data.accounts)
  renderAlerts(data.alerts)
  renderGroups(data.groups)
  setSettings(data.settings)
  const report = byId('download-report')
  if (data.latest_run?.id) {
    report.href = '/api/report/' + encodeURIComponent(data.latest_run.id)
    report.classList.remove('disabled')
    report.setAttribute('aria-disabled', 'false')
  } else {
    report.href = '#'
    report.classList.add('disabled')
    report.setAttribute('aria-disabled', 'true')
  }
  byId('run-now').disabled = data.scan_running
  if (!data.configured) toast('服务端尚未完成必要的 SSO、管理 Key 或网关 Key 配置。')
}

async function refresh() {
  try {
    const data = await api('/api/dashboard')
    renderDashboard(data)
  } catch (error) {
    if (error.unauthorized) showLogin()
    else toast(error.message)
  }
}

function showLogin() {
  byId('app').classList.add('hidden')
  byId('login-gate').classList.remove('hidden')
  byId('identity').textContent = ''
  byId('logout').classList.add('hidden')
}

async function saveSettings() {
  const stopRules = [...document.querySelectorAll('.stop-rule:checked')].map((input) => input.value)
  try {
    const value = await api('/api/settings', {
      method: 'POST',
      body: JSON.stringify({
        interval_minutes: Number(byId('interval').value),
        scheduler_enabled: byId('scheduler-enabled').checked,
        auto_stop_enabled: byId('auto-stop-enabled').checked,
        stop_rules: stopRules,
        target_model: byId('target-model').value,
      }),
    })
    setSettings(value)
    toast('设置已保存')
    await refresh()
  } catch (error) {
    toast(error.message)
  }
}

async function stopAccount(accountId) {
  if (!window.confirm('确认把上游账号 #' + accountId + ' 设为不可调度？此操作不会自动恢复。')) return
  try {
    await api('/api/accounts/' + accountId + '/stop', { method: 'POST', body: '{}' })
    toast('账号已设为不可调度')
    await refresh()
  } catch (error) {
    toast(error.message)
  }
}

async function runNow() {
  try {
    await api('/api/scan', { method: 'POST', body: JSON.stringify({ target_model: byId('target-model').value }) })
    toast('扫描已加入队列')
    await refresh()
    await loadModels()
  } catch (error) {
    toast(error.message)
  }
}

async function boot() {
  try {
    const session = await api('/api/session')
    byId('identity').textContent = session.display_name || session.email || '管理员'
    byId('logout').classList.remove('hidden')
    byId('app').classList.remove('hidden')
    byId('login-gate').classList.add('hidden')
    await refresh()
    await loadModels()
    state.refreshTimer = window.setInterval(refresh, 8000)
  } catch (error) {
    if (error.unauthorized) showLogin()
    else toast(error.message)
  }
}

byId('save-settings').addEventListener('click', saveSettings)
byId('run-now').addEventListener('click', runNow)
byId('logout').addEventListener('click', async () => {
  try {
    await api('/api/logout', { method: 'POST', body: '{}' })
  } finally {
    showLogin()
  }
})
boot()
