(() => {
  const PROMPT = 'Write an AK47 SVG at the same scale as reality'
  const DEFAULT_MODELS = ['gpt-6.1-sol', 'gpt-6-astra']
  const state = {
    config: { prompt: PROMPT, prompt_id: 'ak47', prompts: [{ id: 'ak47', label: 'AK47', text: PROMPT }], max_parallel_jobs: 6, models: DEFAULT_MODELS, reasoning_effort: 'low', reasoning_efforts: ['low', 'medium', 'high', 'max'], csrf_token: '', management_required: true, authenticated: false },
    promptIDs: ['ak47'],
    promptEditor: { mode: 'edit', id: '', version: 0, submitting: false },
    defaultJobName: 'AK47 SVG 双模型对比',
    activeJobs: 0,
    activeModels: 0,
    authPanelOpen: false,
    accounts: [],
    sources: [],
    jobs: [],
    results: [],
    accountError: '',
    runningJobs: new Set(),
    parseTimer: null,
    parseRevision: 0,
    artifact: { id: null, raw: '', dimensions: null },
    connection: { base_url: '', key_configured: false },
    parallelDraftDirty: false,
    parsed: { url: '', key_detected: false, error: '' },
    lastParseError: '',
    billing: { file: null, fileName: '', fileSize: 0, result: null, loading: false },
    response: { id: null, raw: '', revision: 0 },
  }

  const $ = (selector) => document.querySelector(selector)
  const els = {
    modeBadge: $('#modeBadge'),
    authToggle: $('#authToggle'),
    authPanel: $('#authPanel'),
    authForm: $('#authForm'),
    accessToken: $('#accessToken'),
    managementArea: $('#managementArea'),
    connectionForm: $('#connectionForm'),
    connectionBaseURL: $('#connectionBaseURL'),
    connectionAdminKey: $('#connectionAdminKey'),
    connectionState: $('#connectionState'),
    notice: $('#notice'),
    effort: $('#effortValue'),
    effortSelect: $('#effortSelect'),
    promptSelect: $('#promptSelect'),
    quickPromptSelect: $('#quickPromptSelect'),
    prompt: $('#promptField'),
    promptEditButton: $('#promptEditButton'),
    promptCreateButton: $('#promptCreateButton'),
    promptEditor: $('#promptEditor'),
    promptEditorForm: $('#promptEditorForm'),
    promptEditorTitle: $('#promptEditorTitle'),
    promptEditorSelect: $('#promptEditorSelect'),
    promptLabelInput: $('#promptLabelInput'),
    promptTextInput: $('#promptTextInput'),
    promptSaveButton: $('#promptSaveButton'),
    promptCancelButton: $('#promptCancelButton'),
    promptCancelButtonSecondary: $('#promptCancelButtonSecondary'),
    promptEditorError: $('#promptEditorError'),
    promptEditorStatus: $('#promptEditorStatus'),
    modelA: $('#modelA'),
    modelB: $('#modelB'),
    interval: $('#intervalField'),
    jobName: $('#jobName'),
    paste: $('#pasteField'),
    sourceGroup: $('#sourceGroup'),
    quickSource: $('#quickSourceSelect'),
    quickSourceJob: $('#quickSourceJobSelect'),
    parseButton: $('#parseButton'),
    parseStatus: $('#parseStatus'),
    parsedUrl: $('#parsedUrl'),
    parsedKey: $('#parsedKey'),
    accounts: $('#accountsList'),
    accountsBody: $('#accountsBody'),
    accountsToggle: $('#accountsToggle'),
    accountCount: $('#accountCount'),
    accountsError: $('#accountsError'),
    jobs: $('#jobsList'),
    parallelHint: $('#parallelHint'),
    results: $('#resultsList'),
    resultCount: $('#resultCount'),
    resultSearch: $('#resultSearch'),
    refresh: $('#refreshButton'),
    createJob: $('#createJobButton'),
    createRunJob: $('#createRunJobButton'),
    parallelForm: $('#parallelForm'),
    parallelLimit: $('#parallelLimit'),
    dialog: $('#artifactDialog'),
    closeArtifact: $('#closeArtifact'),
    artifactTitle: $('#artifactTitle'),
    artifactMeta: $('#artifactMeta'),
    artifactFrame: $('#artifactFrame'),
    artifactLoading: $('#artifactLoading'),
    artifactError: $('#artifactError'),
    download: $('#downloadArtifact'),
    sizeOriginal: $('#sizeOriginal'),
    sizeFit: $('#sizeFit'),
    artifactStage: $('.artifact-stage'),
    responseDialog: $('#responseDialog'),
    responseTitle: $('#responseTitle'),
    responseMeta: $('#responseMeta'),
    responseHint: $('#responseHint'),
    responseDiagnostics: $('#responseDiagnostics'),
    responseText: $('#responseText'),
    responseLoading: $('#responseLoading'),
    responseEmpty: $('#responseEmpty'),
    responseError: $('#responseError'),
    closeResponse: $('#closeResponse'),
    downloadResponse: $('#downloadResponse'),
    billingCsvFile: $('#billingCsvFile'),
    billingFileMeta: $('#billingFileMeta'),
    billingStatus: $('#billingStatus'),
    billingAnalyze: $('#billingAnalyzeButton'),
    billingDownload: $('#billingDownloadButton'),
    billingClear: $('#billingClearButton'),
    billingSummary: $('#billingSummary'),
    billingModelSection: $('#billingModelSection'),
    billingModelTable: $('#billingModelTable'),
    billingAnomalySection: $('#billingAnomalySection'),
    billingAnomalies: $('#billingAnomalies'),
    billingEmpty: $('#billingEmpty'),
  }
  state.accountsExpanded = false
  state.resultSearch = ''
  const resultCards = new Map()
  const resultsEmpty = document.createElement('div')
  resultsEmpty.className = 'empty-state'
  resultsEmpty.textContent = '暂无结果，运行任务后会出现在这里。'

  function setNotice(message, error = false) {
    els.notice.hidden = !message
    els.notice.textContent = message || ''
    els.notice.classList.toggle('error', error)
  }

  function errorText(error, fallback = '请求失败') {
    return error?.message || fallback
  }

  function escapeHTML(value) {
    return String(value ?? '').replace(/[&<>"']/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char]))
  }

  function formatDate(value) {
    if (!value) return '—'
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString('zh-CN', { hour12: false })
  }

  function duration(value) {
    if (value == null || value === '') return '—'
    const milliseconds = Number(value)
    return Number.isFinite(milliseconds) ? `${(milliseconds / 1000).toFixed(2)} 秒` : String(value)
  }

  function liveDuration(result) {
    if (result?.live || result?.status === 'running') {
      const started = Date.parse(result.started_at || '')
      if (Number.isFinite(started)) return duration(Math.max(0, Date.now() - started))
    }
    return duration(result?.duration_ms ?? result?.duration)
  }

  function modelList() {
    return [els.modelA.value.trim(), els.modelB.value.trim()]
      .filter((model, index, all) => model && all.indexOf(model) === index)
  }

  function promptOptions() {
    const configured = state.config.prompts
    return Array.isArray(configured) && configured.length
      ? configured
      : [{ id: 'ak47', label: 'AK47', text: state.config.prompt || PROMPT }]
  }

  function promptLabel(item) {
    const id = item?.prompt_id || 'ak47'
    return item?.prompt_label || promptOptions().find(prompt => prompt.id === id)?.label || (id === 'composite' ? '复合提示词' : 'AK47')
  }

  function promptByID(id) {
    return promptOptions().find(prompt => String(prompt.id) === String(id)) || null
  }

  function setPromptEditorError(message = '') {
    els.promptEditorError.hidden = !message
    els.promptEditorError.textContent = message
  }

  function updatePromptEditorFields() {
    const prompt = promptByID(els.promptEditorSelect.value)
    if (!prompt) {
      els.promptLabelInput.value = ''
      els.promptTextInput.value = ''
      state.promptEditor.id = ''
      state.promptEditor.version = 0
      return
    }
    state.promptEditor.id = String(prompt.id)
    state.promptEditor.version = Number.isInteger(Number(prompt.version)) ? Number(prompt.version) : 1
    els.promptLabelInput.value = prompt.label || ''
    els.promptTextInput.value = prompt.text || ''
    els.promptEditorStatus.textContent = `版本 ${state.promptEditor.version}`
  }

  function renderPromptEditorSelect(selectedID = state.promptIDs[0]) {
    const prompts = promptOptions()
    els.promptEditorSelect.innerHTML = prompts.map(prompt => `
      <option value="${escapeHTML(prompt.id)}">${escapeHTML(prompt.label || prompt.id)}</option>
    `).join('')
    if (prompts.some(prompt => String(prompt.id) === String(selectedID))) {
      els.promptEditorSelect.value = selectedID
    } else if (prompts[0]) {
      els.promptEditorSelect.value = prompts[0].id
    }
    updatePromptEditorFields()
  }

  function openPromptEditor(mode = 'edit', id = state.promptIDs[0]) {
    state.promptEditor.mode = mode
    setPromptEditorError('')
    els.promptEditorTitle.textContent = mode === 'create' ? '新建提示词' : '编辑提示词'
    els.promptEditorSelect.disabled = mode === 'create'
    els.promptEditorSelect.hidden = mode === 'create'
    els.promptEditorSelect.previousElementSibling.hidden = mode === 'create'
    if (mode === 'create') {
      state.promptEditor.id = ''
      state.promptEditor.version = 0
      els.promptLabelInput.value = ''
      els.promptTextInput.value = ''
      els.promptEditorStatus.textContent = '新建提示词'
    } else {
      renderPromptEditorSelect(id)
    }
    els.promptEditor.showModal()
    els.promptLabelInput.focus()
  }

  function closePromptEditor() {
    if (els.promptEditor.open) els.promptEditor.close()
    state.promptEditor.submitting = false
    els.promptSaveButton.disabled = false
    setPromptEditorError('')
  }

  async function refreshPromptAfterConflict() {
    try {
      await refreshConfig()
      const latest = promptByID(state.promptEditor.id)
      if (latest) {
        state.promptEditor.version = Number(latest.version) || 1
        els.promptEditorStatus.textContent = `版本已刷新到 ${state.promptEditor.version}；当前编辑内容未覆盖，请确认后重试`
      }
    } catch (error) {
      setPromptEditorError(`刷新提示词失败：${errorText(error, '请稍后重试')}`)
    }
  }

  async function savePromptEditor(event) {
    event.preventDefault()
    if (state.promptEditor.submitting) return
    const label = els.promptLabelInput.value.trim()
    const text = els.promptTextInput.value
    if (!label) {
      setPromptEditorError('提示词名称不能为空')
      els.promptLabelInput.focus()
      return
    }
    if (label.length > 60) {
      setPromptEditorError('提示词名称不能超过 60 个字符')
      els.promptLabelInput.focus()
      return
    }
    if (!text.trim()) {
      setPromptEditorError('原文提示词不能为空')
      els.promptTextInput.focus()
      return
    }
    if (text.length > 32000) {
      setPromptEditorError('原文提示词不能超过 32000 个字符')
      els.promptTextInput.focus()
      return
    }
    state.promptEditor.submitting = true
    els.promptSaveButton.disabled = true
    setPromptEditorError('')
    els.promptEditorStatus.textContent = '保存中…'
    try {
      const mode = state.promptEditor.mode
      const payload = mode === 'create'
        ? { label, text }
        : { label, text, version: state.promptEditor.version }
      const path = mode === 'create'
        ? '/api/prompts'
        : `/api/prompts/${encodeURIComponent(state.promptEditor.id)}`
      const saved = await request(path, { method: mode === 'create' ? 'POST' : 'PATCH', body: payload })
      const preset = saved?.preset || saved
      await refreshConfig()
      if (preset?.id) {
        state.promptIDs = mode === 'create' ? [String(preset.id)] : state.promptIDs
      }
      syncPromptUI()
      closePromptEditor()
      setNotice(mode === 'create' ? '提示词已创建，可用于新建任务' : '提示词已更新，可用于新建任务')
    } catch (error) {
      if (error?.status === 409 || /版本冲突|冲突|version/i.test(error?.message || '')) {
        setPromptEditorError('提示词版本已变化，已刷新最新内容，请确认后重试')
        await refreshPromptAfterConflict()
      } else {
        setPromptEditorError(errorText(error, '保存提示词失败'))
      }
    } finally {
      state.promptEditor.submitting = false
      els.promptSaveButton.disabled = false
    }
  }

  function syncPromptUI() {
    const prompts = promptOptions()
    const selectedIDs = state.promptIDs?.length ? state.promptIDs.map(String) : [String(state.config.prompt_id || 'ak47')]
    const selectedList = selectedIDs.map(id => prompts.find(prompt => String(prompt.id) === id)).filter(Boolean)
    const selected = selectedList[0]
      || prompts.find(prompt => String(prompt.id) === String(state.config.prompt_id))
      || prompts[0]
    if (!selected) return
    state.promptIDs = selectedList.length ? selectedList.map(prompt => prompt.id) : [selected.id]
    const activeIDs = new Set(state.promptIDs.map(String))
    els.promptSelect.innerHTML = prompts.map(prompt => `
      <label class="prompt-option">
        <input type="checkbox" value="${escapeHTML(prompt.id)}" data-prompt-id="${escapeHTML(prompt.id)}" ${activeIDs.has(String(prompt.id)) ? 'checked' : ''}>
        <span>${escapeHTML(prompt.label)}</span>
      </label>
    `).join('')
    els.prompt.value = selectedList.length > 1
      ? selectedList.map(prompt => `【${prompt.label}】\n${prompt.text}`).join('\n\n')
      : selected.text || ''
    els.prompt.classList.toggle('long-prompt', els.prompt.value.length > 500)
    const nextDefault = selectedList.length > 1
      ? `${selectedList.map(prompt => prompt.label).join(' + ')} SVG 双模型对比`
      : `${selected.label} SVG 双模型对比`
    // 只同步自动生成的任务名称，保留用户自行输入的名称。
    if (!els.jobName.value.trim() || els.jobName.value === state.defaultJobName) els.jobName.value = nextDefault
    state.defaultJobName = nextDefault
    syncQuickPromptUI()
  }

  function syncQuickPromptUI() {
    if (!els.quickPromptSelect) return
    const prompts = promptOptions()
    const selected = new Set((state.promptIDs || []).map(String))
    els.quickPromptSelect.innerHTML = prompts.map(prompt => `
      <label class="prompt-option">
        <input type="checkbox" value="${escapeHTML(prompt.id)}" data-quick-prompt-id="${escapeHTML(prompt.id)}" ${selected.has(String(prompt.id)) ? 'checked' : ''}>
        <span>${escapeHTML(prompt.label)}</span>
      </label>
    `).join('')
  }

  function updateJobs(payload) {
    state.jobs = payload?.items || []
    state.activeJobs = Number(payload?.active_count) || 0
    state.activeModels = Number(payload?.active_model_count) || 0
  }

  function renderQuickSources() {
    const current = els.quickSource.value
    const currentJob = els.quickSourceJob.value
    els.quickSource.innerHTML = '<option value="">请选择已保存站点（无需再填 Key）</option>' + state.sources.map(source => {
      const sourceRef = source.source_url_ref || source.source_url
      const count = Array.isArray(source.jobs) ? source.jobs.length : 0
      const group = source.group ? ` · 分组：${source.group}` : ''
      return `<option value="${escapeHTML(sourceRef)}">${escapeHTML(source.source_url || sourceRef)}${escapeHTML(group)} · ${count || 1} 个历史任务</option>`
    }).join('')
    if (state.sources.some(source => (source.source_url_ref || source.source_url) === current)) {
      els.quickSource.value = current
      renderQuickSourceJobs(current, currentJob)
    } else {
      renderQuickSourceJobs('', '')
    }
  }

  function renderQuickSourceJobs(sourceRef = els.quickSource.value, selectedJob = els.quickSourceJob.value) {
    const source = state.sources.find(item => (item.source_url_ref || item.source_url) === sourceRef)
    const jobs = Array.isArray(source?.jobs) ? source.jobs : []
    // 站点引用使用最近候选；摘要可能指向其他密钥，仅匹配时展示其指纹。
    const defaultJob = jobs[0]
    const defaultFingerprint = defaultJob && defaultJob.id === source?.source_job_id ? source.key_fingerprint : ''
    const defaultBinding = defaultJob
      ? `${defaultJob.name || `任务 ${defaultJob.id.slice(0, 8)}`}${defaultFingerprint ? ` · Key ${defaultFingerprint}` : defaultJob.key_configured ? ' · 已配置 Key' : ' · Sub2API 账号'}`
      : ''
    const defaultOption = source
      ? `使用站点默认凭据（无需选择任务）${defaultBinding ? ` · ${defaultBinding}` : ''}`
      : '可选：先选择站点，再指定历史凭据'
    els.quickSourceJob.innerHTML = `<option value="">${escapeHTML(defaultOption)}</option>` + jobs.map(job => {
      const prompt = job.prompt_label || job.prompt_id || '提示词'
      const date = job.last_test_at ? ` · ${formatDate(job.last_test_at)}` : ''
      const binding = job.key_configured ? '服务器加密 Key' : 'Sub2API 账号'
      const group = job.group ? ` · 分组：${job.group}` : ''
      return `<option value="${escapeHTML(job.id)}">${escapeHTML(job.name || `任务 ${job.id.slice(0, 8)}`)}${escapeHTML(group)} · ${escapeHTML(prompt)} · ${escapeHTML(binding)}${escapeHTML(date)}</option>`
    }).join('')
    els.quickSourceJob.disabled = !source || !jobs.length
    if (jobs.some(job => job.id === selectedJob)) els.quickSourceJob.value = selectedJob
  }

  async function refreshSources() {
    const sources = await request('/api/sources')
    state.sources = sources?.items || []
    renderQuickSources()
  }

  function resultSourceURL(result) {
    return result?.source_url || result?.request_url || ''
  }

  function apiURL(path) {
    const relative = path.startsWith('/') ? `.${path}` : `./${path}`
    return new URL(relative, document.baseURI).toString()
  }

  function storedAccessToken() {
    try { return sessionStorage.getItem('LAB_ACCESS_TOKEN') || '' } catch { return '' }
  }

  function saveAccessToken(value) {
    try {
      if (value) sessionStorage.setItem('LAB_ACCESS_TOKEN', value)
      else sessionStorage.removeItem('LAB_ACCESS_TOKEN')
    } catch {}
  }

  function syncAuthUI() {
    const open = !state.config.management_required || state.config.authenticated
    els.managementArea.hidden = !open
    els.authPanel.hidden = open || !state.authPanelOpen
    els.authToggle.hidden = !state.config.management_required
    els.authToggle.textContent = open && state.config.authenticated ? '退出管理' : '管理登录'
    for (const button of els.results.querySelectorAll('[data-response-id]')) button.hidden = !open
    if (!open && els.responseDialog.open) els.responseDialog.close()
  }

  async function refreshConfig() {
    const headers = {}
    const accessToken = storedAccessToken()
    if (accessToken) headers['X-Lab-Access'] = accessToken
    const response = await fetch(apiURL('/api/config'), { cache: 'no-store', headers })
    let payload = {}
    try { payload = await response.json() } catch {}
    if (!response.ok) throw new Error(payload.error || `配置请求失败（${response.status}）`)
    state.config = { ...state.config, ...payload }
    syncPromptUI()
    syncAuthUI()
    return state.config
  }

  async function request(path, options = {}) {
    const retryCSRF = options.retryCSRF !== false
    const method = options.method || 'GET'
    const headers = { 'Content-Type': 'application/json', ...(options.headers || {}) }
    const accessToken = storedAccessToken()
    if (accessToken) headers['X-Lab-Access'] = accessToken
    if (method !== 'GET' && method !== 'HEAD') {
      if (!state.config.csrf_token) await refreshConfig()
      if (!state.config.csrf_token) throw new Error('请先登录管理区')
      headers['x-lab-token'] = state.config.csrf_token
    }
    const response = await fetch(apiURL(path), {
      ...options,
      headers,
      cache: 'no-store',
      body: options.body && typeof options.body !== 'string' ? JSON.stringify(options.body) : options.body,
    })
    if (!response.ok) {
      let payload = {}
      try { payload = await response.json() } catch {}
      if (response.status === 401) {
        saveAccessToken('')
        state.config.authenticated = false
        state.config.csrf_token = ''
        syncAuthUI()
        const authError = new Error('管理口令不正确或已过期')
        authError.status = response.status
        throw authError
      }
      let csrfExpired = response.status === 403 && (
        payload.code === 'CSRF_EXPIRED'
        || payload.error === '页面会话已过期，请刷新后重试'
      )
      if (csrfExpired && retryCSRF && method !== 'GET' && method !== 'HEAD') {
        state.config.csrf_token = ''
        await refreshConfig()
        return request(path, { ...options, retryCSRF: false })
      }
      const requestError = new Error(payload.error || `请求失败（${response.status}）`)
      requestError.status = response.status
      requestError.payload = payload
      throw requestError
    }
    if (response.status === 204) return null
    const type = response.headers.get('content-type') || ''
    return type.includes('application/json') ? response.json() : response.text()
  }

  async function uploadBillingFile(file, retryCSRF = true) {
    if (!state.config.csrf_token) await refreshConfig()
    if (!state.config.csrf_token) throw new Error('请先登录管理区')
    const headers = { 'x-lab-token': state.config.csrf_token }
    const accessToken = storedAccessToken()
    if (accessToken) headers['X-Lab-Access'] = accessToken
    const form = new FormData()
    form.append('file', file, file.name)
    const response = await fetch(apiURL('/api/billing-audits'), {
      method: 'POST',
      headers,
      cache: 'no-store',
      body: form,
    })
    let payload = {}
    try { payload = await response.json() } catch {}
    if (!response.ok) {
      if (response.status === 401) {
        saveAccessToken('')
        state.config.authenticated = false
        state.config.csrf_token = ''
        syncAuthUI()
        throw new Error('管理口令不正确或已过期')
      }
      const csrfExpired = response.status === 403 && (
        payload.code === 'CSRF_EXPIRED'
        || payload.error === '页面会话已过期，请刷新后重试'
      )
      if (csrfExpired && retryCSRF) {
        state.config.csrf_token = ''
        await refreshConfig()
        return uploadBillingFile(file, false)
      }
      throw new Error(payload.error || `计费分析失败（${response.status}）`)
    }
    return payload
  }

  async function loadAll() {
    setNotice('')
    const failures = []
    try {
      await refreshConfig()
      const effort = state.config.reasoning_effort || 'low'
      els.effort.textContent = effort
      if (els.effortSelect) els.effortSelect.value = effort
      syncAuthUI()
    } catch (error) { failures.push(errorText(error, '配置加载失败')) }
    const results = await Promise.allSettled([request('/api/results')])
    if (results[0].status === 'fulfilled') state.results = results[0].value?.items || []
    else failures.push(errorText(results[0].reason, '结果列表加载失败'))
    const canManage = state.config.authenticated || !state.config.management_required
    if (canManage) {
      const [accounts, jobs, sources, settings] = await Promise.allSettled([request('/api/accounts'), request('/api/jobs'), request('/api/sources'), request('/api/settings')])
      if (accounts.status === 'fulfilled') {
        state.accounts = accounts.value?.items || []
        state.accountError = ''
      } else {
        state.accounts = []
        state.accountError = errorText(accounts.reason, '管理员账号列表加载失败；仍可使用外部粘贴来源')
      }
      if (jobs.status === 'fulfilled') updateJobs(jobs.value)
      else failures.push(errorText(jobs.reason, '任务列表加载失败'))
      if (sources.status === 'fulfilled') state.sources = sources.value?.items || []
      else state.sources = []
      if (settings.status === 'fulfilled') applySettings(settings.value)
      else failures.push(errorText(settings.reason, '并行设置加载失败'))
      renderQuickSources()
      await loadConnection()
    } else {
      state.accounts = []
      state.jobs = []
      state.activeJobs = 0
      state.activeModels = 0
      state.accountError = ''
      resetConnectionUI()
    }
    els.modeBadge.textContent = state.accountError ? '部分连接' : failures.length ? '连接失败' : '已连接'
    renderAll()
    if (failures.length) setNotice(failures.join('；'), true)
  }

  function applySettings(payload) {
    const max = Number(payload?.max_parallel_jobs)
    if (Number.isInteger(max) && max >= 1 && max <= 16) {
      state.config.max_parallel_jobs = max
      if (!state.parallelDraftDirty) els.parallelLimit.value = String(max)
    }
    if (Number.isInteger(Number(payload?.active_count))) state.activeJobs = Number(payload.active_count)
    if (Number.isInteger(Number(payload?.active_model_count))) state.activeModels = Number(payload.active_model_count)
  }

  function resetConnectionUI() {
    state.connection = { base_url: '', key_configured: false }
    els.connectionBaseURL.value = ''
    els.connectionAdminKey.value = ''
    els.connectionState.textContent = '尚未连接'
  }

  async function loadConnection() {
    try {
      const connection = await request('/api/connection')
      state.connection = { base_url: connection?.base_url || '', key_configured: Boolean(connection?.key_configured) }
      els.connectionBaseURL.value = state.connection.base_url
      els.connectionAdminKey.value = ''
      els.connectionState.textContent = state.connection.key_configured ? '已验证' : '尚未连接'
    } catch (error) {
      if (state.config.authenticated) {
        els.connectionState.textContent = '读取失败'
        setNotice(errorText(error, '读取连接配置失败'), true)
      }
    }
  }

  async function loadJobsResults({ quiet = false } = {}) {
    const canManage = state.config.authenticated || !state.config.management_required
    const requests = [request('/api/results')]
    if (canManage) requests.unshift(request('/api/jobs'), request('/api/settings'))
    const responses = await Promise.allSettled(requests)
    const failures = []
    const results = canManage ? responses[2] : responses[0]
    if (canManage) {
      const jobs = responses[0]
      if (jobs.status === 'fulfilled') updateJobs(jobs.value)
      else failures.push(errorText(jobs.reason, '任务列表加载失败'))
      const settings = responses[1]
      if (settings.status === 'fulfilled') applySettings(settings.value)
      else failures.push(errorText(settings.reason, '并行设置加载失败'))
    }
    if (results.status === 'fulfilled') state.results = results.value?.items || []
    else failures.push(errorText(results.reason, '结果列表加载失败'))
    renderJobs()
    renderResults()
    if (!quiet && failures.length) setNotice(failures.join('；'), true)
  }

  function renderAll() {
    renderAccounts()
    renderJobs()
    renderResults()
  }

  function selectedAccountIDs() {
    return [...document.querySelectorAll('[data-account-id]:checked')]
      .map((input) => Number(input.value))
      .filter(Number.isSafeInteger)
  }

  function renderAccounts() {
    const selected = new Set(selectedAccountIDs().map(String))
    els.accountCount.textContent = `${state.accounts.length} 个来源`
    if (els.accountsError) {
      els.accountsError.hidden = !state.accountError
      els.accountsError.textContent = state.accountError
    }
    if (!state.accounts.length) {
      els.accounts.innerHTML = '<div class="empty-state">暂无可测试来源，可在上方粘贴外部来源。</div>'
      return
    }
    els.accounts.innerHTML = state.accounts.map((account) => `
      <label class="account-row">
        <span class="account-pick">
          <input type="checkbox" value="${escapeHTML(account.id)}" ${account.eligible ? '' : 'disabled'} ${selected.has(String(account.id)) ? 'checked' : ''} data-account-id="${escapeHTML(account.id)}">
          <span>
            <strong class="row-title">${escapeHTML(account.name || `来源 #${account.id}`)}</strong>
            <span class="source-url">${escapeHTML(account.source_url || '未提供来源 URL')}</span>
            ${account.group ? `<span class="source-url">分组：${escapeHTML(account.group)}</span>` : ''}
            <span class="source-url">API Key：${account.eligible ? '已配置（出于安全不显示明文）' : '不可用'}</span>
            ${account.reason ? `<span class="source-url">${escapeHTML(account.reason)}</span>` : ''}
          </span>
        </span>
        <span class="eligibility ${account.eligible ? 'eligible' : 'ineligible'}">${account.eligible ? '可测试' : '不可用'}</span>
      </label>
    `).join('')
  }

  function taskSourceSummary(job) {
    const urls = []
    if (job?.source_url) urls.push(job.source_url)
    for (const source of Object.values(job?.sources || {})) {
      if (source?.url && !urls.includes(source.url)) urls.push(source.url)
    }
    return urls
  }

  function jobBindingSummary(job) {
    const accountIds = job?.account_ids || []
    if (accountIds.length) {
      return accountIds.map(id => {
        const source = job.sources?.[id]
        return `Sub2API 账号 #${id}${source?.name ? ` · ${source.name}` : ''}`
      })
    }
    return [job?.key_configured ? `粘贴来源 · API Key 已加密保存${job.key_fingerprint ? ` · 指纹 ${job.key_fingerprint}` : ''}` : '粘贴来源 · 未配置 API Key']
  }

  function intervalOptions(value) {
    const options = [
      [0, '仅手动'],
      [5, '5 分钟'],
      [15, '15 分钟'],
      [30, '30 分钟'],
      [60, '60 分钟'],
      [360, '6 小时'],
      [1440, '24 小时'],
    ]
    const normalized = Number(value)
    if (!options.some(([minutes]) => minutes === normalized)) options.push([normalized, `${normalized} 分钟`])
    return options.map(([minutes, label]) => `<option value="${minutes}" ${minutes === normalized ? 'selected' : ''}>${label}</option>`).join('')
  }

  function renderJobs() {
    const maxParallel = Number(state.config.max_parallel_jobs) || 6
    const activeCount = Math.max(state.activeJobs, state.jobs.filter(job => job.running || state.runningJobs.has(String(job.id))).length)
    els.parallelHint.textContent = `任务 ${activeCount} / ${maxParallel} · 模型运行 ${state.activeModels} 路。同一任务 A、B 同时请求；每任务选 2 个模型时，最多 ${maxParallel * 2} 路。`
    if (!state.jobs.length) {
      els.jobs.innerHTML = '<div class="empty-state">还没有定时任务。</div>'
      return
    }
    els.jobs.innerHTML = state.jobs.map((job) => {
      const running = Boolean(job.running) || state.runningJobs.has(String(job.id))
      const schedule = job.enabled ? `每 ${escapeHTML(job.interval_minutes || 30)} 分钟` : '仅手动运行'
      return `
        <article class="job-row">
          <div class="job-info">
            <strong class="row-title">${escapeHTML(job.name || `任务 #${job.id}`)}</strong>
            <div class="job-meta">
              ${job.group ? `<span class="prompt-tag">分组：${escapeHTML(job.group)}</span>` : ''}
              <span class="prompt-tag">${escapeHTML(promptLabel(job))}</span>
              <span>${(job.models || []).map(escapeHTML).join(' · ') || '未设置模型'}</span>
              <span>思考 ${escapeHTML(job.reasoning_effort || 'low')}</span>
              <span>${schedule}</span>
              <span>计划已保存</span>
              <span class="result-status ${running ? 'status-running' : job.enabled ? 'status-success' : 'status-running'}">${running ? '运行中' : job.enabled ? '已启用' : '已暂停'}</span>
            </div>
            ${taskSourceSummary(job).map(url => `<div class="job-source" title="${escapeHTML(url)}">上游：${escapeHTML(url)}</div>`).join('')}
            ${jobBindingSummary(job).map(binding => `<div class="job-binding">${escapeHTML(binding)}</div>`).join('')}
            ${job.next_run_at && job.enabled ? `<div class="job-meta">下次运行：${escapeHTML(formatDate(job.next_run_at))}</div>` : ''}
          </div>
          <div class="job-actions">
            <button class="small-button run" type="button" data-run-job="${escapeHTML(job.id)}" ${running ? 'disabled' : ''}>${running ? '运行中…' : '立即运行'}</button>
            <select class="job-interval" data-interval-job="${escapeHTML(job.id)}" aria-label="调整任务间隔" ${running ? 'disabled' : ''}>${intervalOptions(job.interval_minutes)}</select>
            <select class="job-effort" data-effort-job="${escapeHTML(job.id)}" aria-label="调整任务思考程度" ${running ? 'disabled' : ''}>${reasoningEffortOptions(job.reasoning_effort)}</select>
            <button class="small-button pause" type="button" data-toggle-job="${escapeHTML(job.id)}" ${running || !job.interval_minutes ? 'disabled' : ''}>${job.enabled ? '暂停' : '启用'}</button>
          </div>
        </article>
      `
    }).join('')
  }

  function resultInfoHTML(result, live, hasSVG) {
    if (live) {
      return `
            <div class="result-live-info">
              <span class="result-name">${escapeHTML(result.model || '未标注模型')}</span>
              <span class="result-job-name">${escapeHTML(result.job_name || '未命名任务')}</span>
              <div class="result-meta">
                ${result.group ? `<span class="prompt-tag">分组：${escapeHTML(result.group)}</span>` : ''}
                <span class="prompt-tag">${escapeHTML(promptLabel(result))}</span>
                <span class="result-status status-running">运行中</span>
                <span>已用 <span class="result-elapsed" data-started-at="${escapeHTML(result.started_at || '')}" data-duration-ms="${Number(result.duration_ms) || 0}"></span></span>
                <span>${escapeHTML(result.reasoning_effort || result.effort || 'low')}</span>
              </div>
              <div class="result-meta">
                <span>${escapeHTML(resultSourceURL(result) || '来源未返回')}</span>
                <span>开始于 ${escapeHTML(formatDate(result.started_at))}</span>
              </div>
            </div>
      `
    }
    const success = result.status === 'success' || result.status === 'passed'
    return `
          ${hasSVG ? `<button class="result-open" type="button" data-result-id="${escapeHTML(result.id)}">` : '<div class="result-info">'}
            <span class="result-name">${escapeHTML(result.model || '未标注模型')}</span>
            <span class="result-job-name">${escapeHTML(result.job_name || '未命名任务')}</span>
            <div class="result-meta">
              ${result.group ? `<span class="prompt-tag">分组：${escapeHTML(result.group)}</span>` : ''}
              <span class="prompt-tag">${escapeHTML(promptLabel(result))}</span>
              <span class="result-status ${success ? 'status-success' : 'status-failed'}">${escapeHTML(result.status || '未知')}</span>
              <span>${escapeHTML(duration(result.duration_ms ?? result.duration))}</span>
              <span>${escapeHTML(result.reasoning_effort || result.effort || 'low')}</span>
              ${result.http_status ? `<span>HTTP ${escapeHTML(result.http_status)}</span>` : ''}
            </div>
            <div class="result-meta"><span>${escapeHTML(resultSourceURL(result) || '来源未返回')}</span><span>${escapeHTML(formatDate(result.created_at || result.started_at))}</span></div>
            ${result.failure_kind ? `<div class="result-diagnostic">失败类别：${escapeHTML(result.failure_kind)}${result.upstream_code ? ` · ${escapeHTML(result.upstream_code)}` : ''}</div>` : ''}
            ${result.upstream_event_type ? `<div class="result-diagnostic">上游事件：${escapeHTML(result.upstream_event_type)}</div>` : ''}
            ${result.upstream_code === 'max_output_tokens' ? `<div class="result-diagnostic">上游报告输出预算耗尽。本次请求上限：${escapeHTML(result.max_output_tokens || '旧记录未记录')} token；当前测试预算：${escapeHTML(state.config.max_output_tokens || 65536)} token（旧任务重跑也生效）。</div>` : ''}
            ${result.failure_kind === 'timeout' ? `<div class="result-diagnostic">本地等待限制：${result.timeout_kind === 'idle' ? '连续未收到数据' : '请求总耗时'}${result.timeout_kind === 'idle' && result.idle_timeout_ms ? ` ${escapeHTML(duration(result.idle_timeout_ms))}` : result.timeout_ms ? ` ${escapeHTML(duration(result.timeout_ms))}` : ''}${Number.isFinite(result.response_bytes) ? ` · 已接收 ${escapeHTML(result.response_bytes)} 字节` : ''}</div>` : ''}
            ${result.failure_kind === 'upstream_no_svg' ? '<div class="result-diagnostic">上游已返回文字，但未找到完整 SVG，可查看原文确认输出内容。</div>' : ''}
            ${result.failure_kind === 'local_svg_validation' ? '<div class="result-diagnostic">本地未提取到完整 SVG；可查看原文确认是否包含图形及标签结构。</div>' : ''}
            ${result.upstream_message ? `<div class="result-diagnostic">上游信息：${escapeHTML(result.upstream_message)}</div>` : ''}
            ${result.error ? `<div class="result-error">${escapeHTML(result.error)}</div>` : ''}
          ${hasSVG ? '</button>' : '</div>'}
          ${state.config.authenticated || !state.config.management_required ? `<div class="result-actions"><button class="small-button" type="button" data-response-id="${escapeHTML(result.id)}">查看原始响应</button></div>` : ''}
    `
  }

  function updateResultTimers() {
    // 计时只修改文字；重建 iframe 会清空预览并让 SVG 动画重新开始。
    for (const timer of els.results.querySelectorAll('.result-elapsed')) {
      timer.textContent = liveDuration({
        live: true,
        started_at: timer.dataset.startedAt,
        duration_ms: Number(timer.dataset.durationMs) || 0
      })
    }
  }

  function renderResults() {
    const query = state.resultSearch.trim().toLocaleLowerCase()
    const retained = new Set(state.results.map(result => String(result.id)))
    for (const [id, card] of resultCards) {
      if (retained.has(id)) continue
      card.node.remove()
      resultCards.delete(id)
    }
    let visibleCount = 0
    let lastVisible = null
    for (const [index, result] of state.results.entries()) {
      const id = String(result.id)
      const live = Boolean(result.live || result.status === 'running')
      const success = result.status === 'success' || result.status === 'passed'
      const hasSVG = !live && (result.has_svg === true || (result.has_svg == null && success))
      let card = resultCards.get(id)
      if (!card) {
        const node = document.createElement('article')
        const media = document.createElement('div')
        const info = document.createElement('div')
        node.className = 'result-card'
        node.dataset.resultKey = id
        node.append(media, info)
        card = { node, media, info, mediaKind: '', infoHTML: '' }
        resultCards.set(id, card)
        els.results.append(node)
      }
      card.node.classList.toggle('is-live', live)
      // 排序和筛选不移动或卸载已有预览，保持 iframe 文档及动画状态。
      card.node.style.order = String(index)
      card.node.hidden = Boolean(query && !String(result.job_name || '').toLocaleLowerCase().includes(query))
      card.node.classList.remove('last-visible')
      if (!card.node.hidden) {
        visibleCount++
        lastVisible = card.node
      }
      const mediaKind = live ? 'live' : hasSVG ? 'svg' : 'empty'
      if (card.mediaKind !== mediaKind) {
        card.mediaKind = mediaKind
        if (hasSVG) {
          const frame = document.createElement('iframe')
          frame.className = 'result-thumb'
          frame.title = 'SVG 缩略预览'
          frame.loading = 'lazy'
          frame.setAttribute('sandbox', '')
          frame.src = apiURL(`/api/results/${encodeURIComponent(id)}/artifact`)
          card.media.replaceChildren(frame)
        } else if (live) {
          card.media.innerHTML = '<div class="result-live-thumb" aria-label="结果生成中"><span class="result-live-spinner" aria-hidden="true"></span><span>正在等待上游返回 SVG</span></div>'
        } else {
          card.media.replaceChildren()
        }
      }
      const infoHTML = resultInfoHTML(result, live, hasSVG)
      if (card.infoHTML !== infoHTML) {
        card.infoHTML = infoHTML
        card.info.innerHTML = infoHTML
      }
    }
    lastVisible?.classList.add('last-visible')
    els.resultCount.textContent = query ? `${visibleCount} / ${state.results.length} 条结果` : `${state.results.length} 条结果`
    if (!resultsEmpty.isConnected) els.results.append(resultsEmpty)
    resultsEmpty.hidden = visibleCount > 0
    updateResultTimers()
  }

  function firstValue(object, keys) {
    if (!object || typeof object !== 'object') return null
    for (const key of keys) {
      if (object[key] !== undefined && object[key] !== null && object[key] !== '') return object[key]
    }
    return null
  }

  function nestedValue(object, paths) {
    for (const path of paths) {
      let current = object
      for (const key of path) {
        if (!current || typeof current !== 'object') {
          current = null
          break
        }
        current = current[key]
      }
      if (current !== undefined && current !== null && current !== '') return current
    }
    return null
  }

  function formatBillingPrice(value) {
    if (value === undefined || value === null || value === '') return '不可判定'
    if (typeof value === 'object') {
      const amount = firstValue(value, ['amount', 'value', 'price', 'rate'])
      const unit = firstValue(value, ['unit', 'currency'])
      if (amount !== null) return `${amount}${unit ? ` ${unit}` : ''}`
      return '不可判定'
    }
    const text = String(value).trim()
    if (!text) return '不可判定'
    const numeric = Number(text)
    return Number.isFinite(numeric) ? numeric.toString() : text
  }

  function normalizeBillingResult(payload) {
    const source = payload?.result || payload?.data || payload || {}
    const rawModels = source.models || source.model_prices || source.prices || source.items || []
    const modelEntries = Array.isArray(rawModels)
      ? rawModels.map(item => [item?.model || item?.model_name || item?.name || '未命名模型', item])
      : Object.entries(rawModels || {})
    const models = modelEntries.map(([name, item]) => {
      const row = typeof item === 'object' && item !== null ? item : { input_price: item }
      const cacheWrite = row.cache_write || row.cache_creation || row.cache_write_prices || {}
      const rates = row.rates || row.prices || {}
      const verdict = firstValue(row, ['verdict', 'status', 'anomaly_status', 'conclusion'])
      const issueText = Array.isArray(row.issues) && row.issues.length ? row.issues.join('；') : ''
      return {
        model: row.model || row.model_name || name,
        input: nestedValue(row, [['input_price'], ['input_price_per_million'], ['input_price_per_1m'], ['input_unit_price'], ['inputPrice'], ['prices', 'input'], ['rates', 'input', 'per_million']]) ?? firstValue(rates.input, ['per_million', 'value', 'amount']),
        output: nestedValue(row, [['output_price'], ['output_price_per_million'], ['output_price_per_1m'], ['output_unit_price'], ['outputPrice'], ['prices', 'output'], ['rates', 'output', 'per_million']]) ?? firstValue(rates.output, ['per_million', 'value', 'amount']),
        cacheRead: nestedValue(row, [['cache_read_price'], ['cache_read_price_per_million'], ['cache_read_price_per_1m'], ['cache_read'], ['prices', 'cache_read'], ['rates', 'cache_read', 'per_million']]) ?? firstValue(rates.cache_read, ['per_million', 'value', 'amount']),
        cacheWrite5m: nestedValue(row, [['cache_write_5m_price'], ['cache_write_5m'], ['cache_creation_5m_price'], ['cache_5m_write_price'], ['prices', 'cache_write_5m'], ['cache_write', '5m'], ['rates', 'cache_write_5m', 'per_million']]) ?? firstValue(rates.cache_write_5m, ['per_million', 'value', 'amount']) ?? firstValue(cacheWrite, ['5m', '5_min', 'five_minutes']),
        cacheWrite1h: nestedValue(row, [['cache_write_1h_price'], ['cache_write_1h'], ['cache_creation_1h_price'], ['cache_1h_write_price'], ['prices', 'cache_write_1h'], ['cache_write', '1h'], ['rates', 'cache_write_1h', 'per_million']]) ?? firstValue(rates.cache_write_1h, ['per_million', 'value', 'amount']) ?? firstValue(cacheWrite, ['1h', '1_hour', 'one_hour']),
        count: firstValue(row, ['request_count', 'requests', 'count', 'rows', 'record_count']),
        verdict: verdict || (issueText ? '需核对' : row.confidence === 'low' ? '证据不足' : '未发现异常'),
        issues: issueText,
      }
    })
    const modelIssues = models.flatMap(item => item.issues ? [{ message: item.issues, model: item.model, severity: 'warning' }] : [])
    const rawAnomalies = source.anomalies || source.issues || source.warnings || source.summary?.issues || modelIssues
    const anomalies = (Array.isArray(rawAnomalies) ? rawAnomalies : [rawAnomalies]).filter(Boolean).map(item => {
      if (typeof item === 'string') return { message: item, severity: 'warning' }
      return {
        message: item.message || item.description || item.detail || item.reason || '发现需要核对的计费差异',
        severity: item.severity || item.level || (item.confirmed ? 'danger' : 'warning'),
        model: item.model || item.model_name || '',
        category: item.category || item.type || '',
      }
    })
    const summary = source.summary || {}
    return {
      summary: {
        rows: firstValue(summary, ['rows', 'row_count', 'total_rows', 'records']) ?? firstValue(source, ['rows', 'row_count', 'total_rows', 'row_count']),
        models: firstValue(summary, ['models', 'model_count', 'distinct_models']) ?? firstValue(source, ['model_count']) ?? models.length,
        anomalies: firstValue(summary, ['anomalies', 'anomaly_count', 'issue_count']) ?? firstValue(source, ['issue_count']) ?? anomalies.length,
        source: source.source_file || source.filename || source.file_name || '',
      },
      models,
      anomalies,
      analyzedAt: source.generated_at || source.analyzed_at || new Date().toISOString(),
    }
  }

  function billingVerdict(value) {
    if (value === undefined || value === null || value === '') return { text: '待核对', className: 'billing-status-warning' }
    if (typeof value === 'boolean') return value ? { text: '需核对', className: 'billing-status-danger' } : { text: '未发现异常', className: 'billing-status-ok' }
    const text = String(value)
    const normalized = text.toLocaleLowerCase()
    if (normalized.includes('ok') || normalized.includes('正常') || normalized.includes('一致') || normalized.includes('未发现')) return { text, className: 'billing-status-ok' }
    if (normalized.includes('danger') || normalized.includes('异常') || normalized.includes('changed') || normalized.includes('暗改')) return { text, className: 'billing-status-danger' }
    return { text, className: 'billing-status-warning' }
  }

  function renderBilling() {
    const result = state.billing.result
    const hasResult = Boolean(result)
    els.billingEmpty.hidden = hasResult
    els.billingSummary.hidden = !hasResult
    els.billingModelSection.hidden = !hasResult || !result.models.length
    els.billingAnomalySection.hidden = !hasResult
    els.billingDownload.disabled = !hasResult
    els.billingClear.disabled = !state.billing.file && !hasResult
    if (!hasResult) {
      els.billingSummary.innerHTML = ''
      els.billingModelTable.innerHTML = ''
      els.billingAnomalies.innerHTML = ''
      return
    }
    const summary = result.summary || {}
    const anomalyCount = Number(summary.anomalies ?? result.anomalies.length) || 0
    els.billingSummary.innerHTML = [
      ['总记录', summary.rows ?? '—', ''],
      ['模型数', summary.models ?? result.models.length, ''],
      ['异常项', anomalyCount, anomalyCount ? 'warning' : ''],
      ['源文件', state.billing.fileName || summary.source || '已上传 CSV', ''],
    ].map(([label, value, modifier]) => `<div class="billing-summary-item ${modifier}"><span>${escapeHTML(label)}</span><strong title="${escapeHTML(value)}">${escapeHTML(value)}</strong></div>`).join('')
    els.billingModelTable.innerHTML = result.models.map(row => {
      const verdict = billingVerdict(row.verdict)
      return `<tr>
        <td>${escapeHTML(row.model || '未命名模型')}</td>
        <td class="billing-price">${escapeHTML(formatBillingPrice(row.input))}</td>
        <td class="billing-price">${escapeHTML(formatBillingPrice(row.output))}</td>
        <td class="billing-price">${escapeHTML(formatBillingPrice(row.cacheRead))}</td>
        <td class="billing-price">${escapeHTML(formatBillingPrice(row.cacheWrite5m))}</td>
        <td class="billing-price">${escapeHTML(formatBillingPrice(row.cacheWrite1h))}</td>
        <td>${escapeHTML(row.count ?? '—')}</td>
        <td><span class="billing-status ${verdict.className}">${escapeHTML(verdict.text)}</span></td>
      </tr>`
    }).join('')
    if (!result.anomalies.length) {
      els.billingAnomalies.innerHTML = '<div class="billing-anomaly ok"><strong>未发现异常</strong><span>上传文件中没有返回可确认的价格变化；缺失 5 分钟或 1 小时缓存单价时，仍会保留“待核对”状态。</span></div>'
      return
    }
    els.billingAnomalies.innerHTML = result.anomalies.map(item => {
      const danger = String(item.severity || '').toLocaleLowerCase().includes('danger') || String(item.severity || '').includes('异常')
      const context = [item.model, item.category].filter(Boolean).join(' · ')
      return `<div class="billing-anomaly ${danger ? 'danger' : ''}"><strong>${escapeHTML(context || (danger ? '异常' : '提示'))}</strong><span>${escapeHTML(item.message)}</span></div>`
    }).join('')
  }

  function setBillingFile(file) {
    state.billing.file = file || null
    state.billing.fileName = file?.name || ''
    state.billing.fileSize = file?.size || 0
    if (!file) {
      els.billingFileMeta.textContent = '支持 CSV，单文件不超过 8 MB。'
      els.billingAnalyze.disabled = true
      els.billingStatus.textContent = '等待上传 CSV'
      return
    }
    const sizeMB = file.size / (1024 * 1024)
    if (!/\.csv$/i.test(file.name) && file.type !== 'text/csv') {
      els.billingFileMeta.textContent = '文件类型不正确，请选择 CSV 文件。'
      els.billingAnalyze.disabled = true
      els.billingStatus.textContent = '文件类型不支持'
      return
    }
    if (file.size > 8 * 1024 * 1024) {
      els.billingFileMeta.textContent = `文件过大（${sizeMB.toFixed(2)} MB），上限为 8 MB。`
      els.billingAnalyze.disabled = true
      els.billingStatus.textContent = '文件超过大小限制'
      return
    }
    els.billingFileMeta.textContent = `${file.name} · ${sizeMB.toFixed(2)} MB`
    els.billingAnalyze.disabled = false
    els.billingStatus.textContent = '文件已选择，等待分析'
  }

  async function analyzeBilling() {
    const file = state.billing.file
    if (!file || els.billingAnalyze.disabled || state.billing.loading) return
    state.billing.loading = true
    els.billingAnalyze.disabled = true
    els.billingStatus.textContent = '分析中…'
    try {
      const payload = await uploadBillingFile(file)
      state.billing.result = normalizeBillingResult(payload)
      renderBilling()
      els.billingStatus.textContent = `分析完成 · ${formatDate(state.billing.result.analyzedAt)}`
      setNotice('计费 CSV 分析完成')
    } catch (error) {
      els.billingStatus.textContent = '分析失败'
      setNotice(errorText(error, '计费 CSV 分析失败'), true)
    } finally {
      state.billing.loading = false
      els.billingAnalyze.disabled = !state.billing.file
    }
  }

  function downloadBillingResult() {
    if (!state.billing.result) return
    const safeResult = {
      source_file: state.billing.fileName || state.billing.result.summary?.source || '',
      analyzed_at: state.billing.result.analyzedAt,
      summary: state.billing.result.summary,
      models: state.billing.result.models,
      anomalies: state.billing.result.anomalies,
    }
    const blob = new Blob([JSON.stringify(safeResult, null, 2)], { type: 'application/json;charset=utf-8' })
    const link = document.createElement('a')
    link.href = URL.createObjectURL(blob)
    link.download = `billing-audit-${new Date().toISOString().replace(/[:.]/g, '-')}.json`
    link.click()
    URL.revokeObjectURL(link.href)
  }

  function clearBilling() {
    els.billingCsvFile.value = ''
    state.billing = { file: null, fileName: '', fileSize: 0, result: null, loading: false }
    setBillingFile(null)
    renderBilling()
    setNotice('计费核对结果已清空')
  }

  function resetParsePreview() {
    state.parsed = { url: '', key_detected: false, error: '' }
    els.parsedUrl.textContent = '等待粘贴内容'
    els.parsedKey.textContent = '不会显示密钥'
    els.parseStatus.textContent = '尚未解析'
    if (state.lastParseError && els.notice.textContent === state.lastParseError) setNotice('')
    state.lastParseError = ''
  }

  function scheduleParse(immediate = false) {
    clearTimeout(state.parseTimer)
    const text = els.paste.value
    const revision = ++state.parseRevision
    if (!text.trim()) {
      resetParsePreview()
      return
    }
    if (state.lastParseError && els.notice.textContent === state.lastParseError) setNotice('')
    state.lastParseError = ''
    state.parsed = { url: '', key_detected: false, error: '' }
    els.parsedUrl.textContent = '等待解析'
    els.parsedKey.textContent = '不会显示密钥'
    els.parseStatus.textContent = '待解析'
    const run = () => parseSource(text, revision)
    state.parseTimer = setTimeout(run, immediate ? 0 : 350)
  }

  async function parseSource(text, revision) {
    els.parseButton.disabled = true
    els.parseStatus.textContent = '解析中…'
    try {
      const parsed = await request('/api/parse', { method: 'POST', body: { text } })
      if (revision !== state.parseRevision || text !== els.paste.value) return
      state.parsed = { url: parsed?.url || '', key_detected: Boolean(parsed?.key_detected), error: '' }
      els.parsedUrl.textContent = state.parsed.url || '未识别到 URL'
      els.parsedKey.textContent = state.parsed.key_detected ? '已识别密钥（仅服务端保存）' : '未识别到密钥'
      els.parseStatus.textContent = state.parsed.url || state.parsed.key_detected ? '解析完成' : '需要补充信息'
      if (state.lastParseError && els.notice.textContent === state.lastParseError) setNotice('')
      state.lastParseError = ''
    } catch (error) {
      if (revision === state.parseRevision && text === els.paste.value) {
        const message = errorText(error, '解析来源失败')
        state.parsed = { url: '', key_detected: false, error: message }
        state.lastParseError = message
        els.parseStatus.textContent = message
        els.parsedUrl.textContent = '未识别到 URL'
        els.parsedKey.textContent = '未识别到密钥'
        setNotice(message, true)
      }
    } finally {
      if (revision === state.parseRevision) els.parseButton.disabled = false
    }
  }

  async function createBatch(runNow = false) {
    const models = modelList()
    const accountIDs = selectedAccountIDs()
    const pastedText = els.paste.value.trim()
    const sourceRef = els.quickSource.value
    const sourceJobRef = els.quickSourceJob.value
    const selectors = Number(Boolean(pastedText)) + Number(Boolean(accountIDs.length)) + Number(Boolean(sourceRef))
    if (selectors !== 1) {
      setNotice('请选择一个上游账号、粘贴来源或已测试快捷来源，三者只能选择一个', true)
      return
    }
    if (!models.length) {
      setNotice('至少填写一个模型', true)
      return
    }
    const manual = els.interval.value === 'manual'
    const body = {
      name: els.jobName.value.trim() || state.defaultJobName,
      group: els.sourceGroup?.value.trim() || '',
      account_ids: accountIDs,
      models,
      prompt_ids: [...new Set(state.promptIDs)],
      reasoning_effort: els.effortSelect?.value || state.config.reasoning_effort || 'low',
      run_now: Boolean(runNow),
      interval_minutes: manual ? 30 : Math.max(5, Number(els.interval.value) || 30),
      enabled: !manual,
    }
    if (pastedText) body.pasted_text = pastedText
    if (sourceRef && sourceJobRef) body.source_job_id = sourceJobRef
    else if (sourceRef) body.source_url_ref = sourceRef
    els.createJob.disabled = true
    els.createRunJob.disabled = true
    try {
      const response = await request('/api/jobs/batch', { method: 'POST', body })
      els.paste.value = ''
      if (els.sourceGroup) els.sourceGroup.value = ''
      els.quickSource.value = ''
      renderQuickSourceJobs('', '')
      resetParsePreview()
      await loadJobsResults()
      await refreshSources().catch(() => {})
      const items = Array.isArray(response?.items) ? response.items : []
      const started = Array.isArray(response?.started_job_ids) ? response.started_job_ids.length : 0
      const queued = Array.isArray(response?.queued_job_ids) ? response.queued_job_ids.length : 0
      setNotice(runNow
        ? `已创建 ${items.length || state.promptIDs.length} 个任务，已启动 ${started} 个，排队 ${queued} 个`
        : `已创建 ${items.length || state.promptIDs.length} 个任务`)
    } catch (error) {
      setNotice(errorText(error, '创建任务失败'), true)
    } finally {
      els.createJob.disabled = false
      els.createRunJob.disabled = false
    }
  }

  async function createJob() {
    return createBatch(false)
  }

  async function runJob(id) {
    const key = String(id)
    const job = state.jobs.find((item) => String(item.id) === key)
    if (!job || job.running || state.runningJobs.has(key)) return
    state.runningJobs.add(key)
    renderJobs()
    try {
      await request(`/api/jobs/${encodeURIComponent(id)}/run`, { method: 'POST' })
      setNotice('任务已开始运行')
    } catch (error) {
      setNotice(errorText(error, '启动任务失败'), true)
    } finally {
      await loadJobsResults({ quiet: true })
      state.runningJobs.delete(key)
      renderJobs()
    }
  }

  async function toggleJob(id) {
    const job = state.jobs.find((item) => String(item.id) === String(id))
    if (!job || job.running || state.runningJobs.has(String(id))) return
    try {
      await request(`/api/jobs/${encodeURIComponent(id)}`, { method: 'PATCH', body: { enabled: !job.enabled } })
      await loadJobsResults({ quiet: true })
      setNotice(job.enabled ? '任务已暂停' : '任务已启用')
    } catch (error) {
      setNotice(errorText(error, '更新任务失败'), true)
    }
  }

  async function updateJobInterval(id, value) {
    const minutes = Number(value)
    const job = state.jobs.find((item) => String(item.id) === String(id))
    if (!job || !Number.isInteger(minutes)) return
    try {
      await request(`/api/jobs/${encodeURIComponent(id)}`, { method: 'PATCH', body: { interval_minutes: minutes } })
      await loadJobsResults({ quiet: true })
      setNotice(minutes ? `任务间隔已调整为 ${minutes} 分钟` : '任务已改为仅手动运行')
    } catch (error) {
      setNotice(errorText(error, '调整任务间隔失败'), true)
      await loadJobsResults({ quiet: true })
    }
  }

  function reasoningEffortOptions(value = 'low') {
    const options = state.config.reasoning_efforts || ['low', 'medium', 'high', 'max']
    const selected = options.includes(value) ? value : 'low'
    return options.map(item => `<option value="${escapeHTML(item)}" ${item === selected ? 'selected' : ''}>${escapeHTML(item)}</option>`).join('')
  }

  async function updateJobEffort(id, value) {
    try {
      await request(`/api/jobs/${encodeURIComponent(id)}`, { method: 'PATCH', body: { reasoning_effort: value } })
      await loadJobsResults({ quiet: true })
      setNotice(`任务思考程度已调整为 ${value}`)
    } catch (error) {
      setNotice(errorText(error, '调整思考程度失败'), true)
      await loadJobsResults({ quiet: true })
    }
  }

  function readSvgDimensions(raw) {
    const documentNode = new DOMParser().parseFromString(raw, 'image/svg+xml')
    const svg = documentNode.documentElement
    if (!svg || svg.nodeName.toLowerCase() !== 'svg') return null
    const numeric = (value) => {
      const match = String(value || '').trim().match(/^([0-9]+(?:\.[0-9]+)?)/)
      return match ? Number(match[1]) : 0
    }
    let width = numeric(svg.getAttribute('width'))
    let height = numeric(svg.getAttribute('height'))
    if ((!width || !height) && svg.getAttribute('viewBox')) {
      const values = svg.getAttribute('viewBox').trim().split(/[\s,]+/).map(Number)
      if (values.length === 4 && values[2] > 0 && values[3] > 0) {
        width ||= values[2]
        height ||= values[3]
      }
    }
    return width && height ? { width, height } : null
  }

  async function openArtifact(id) {
    const result = state.results.find((item) => String(item.id) === String(id))
    if (!result) return
    els.dialog.showModal()
    els.artifactTitle.textContent = `${result.model || '结果'} · 原始 SVG`
    els.artifactMeta.innerHTML = [
      `<span>任务：${escapeHTML(result.job_name || '未命名任务')}</span>`,
      `<span>提示词：${escapeHTML(promptLabel(result))}</span>`,
      `<span>来源：${escapeHTML(resultSourceURL(result) || '未返回')}</span>`,
      `<span>模型：${escapeHTML(result.model || '—')}</span>`,
      `<span>状态：${escapeHTML(result.status || '—')}</span>`,
      `<span>耗时：${escapeHTML(duration(result.duration_ms ?? result.duration))}</span>`,
      `<span>思考：${escapeHTML(result.reasoning_effort || result.effort || 'low')}</span>`,
    ].join('')
    els.artifactError.hidden = true
    els.artifactLoading.hidden = false
    els.download.disabled = true
    try {
      const raw = await request(`/api/results/${encodeURIComponent(id)}/artifact`)
      state.artifact.raw = typeof raw === 'string' ? raw : ''
      if (!state.artifact.raw) throw new Error('服务端未返回 SVG')
      state.artifact.id = id
      state.artifact.dimensions = readSvgDimensions(state.artifact.raw)
      els.artifactFrame.src = apiURL(`/api/results/${encodeURIComponent(id)}/artifact`)
      setArtifactView('original')
      els.download.disabled = false
    } catch (error) {
      els.artifactError.textContent = errorText(error, '加载原始 SVG 失败')
      els.artifactError.hidden = false
    } finally {
      els.artifactLoading.hidden = true
    }
  }

  function downloadArtifact() {
    if (state.artifact.id == null) return
    const link = document.createElement('a')
    link.href = apiURL(`/api/results/${encodeURIComponent(state.artifact.id)}/artifact?download=1`)
    link.download = `svg-result-${state.artifact.id}.svg`
    link.click()
  }

  async function openResponse(id) {
    if (!state.config.authenticated && state.config.management_required) return
    const result = state.results.find(item => String(item.id) === String(id))
    if (!result || result.live || result.status === 'running') return
    const revision = state.response.revision + 1
    state.response = { id: String(id), raw: '', revision }
    els.responseTitle.textContent = `${result.model || '结果'} · 原始响应`
    els.responseMeta.textContent = `任务：${result.job_name || '未命名任务'} · 提示词：${promptLabel(result)} · 来源：${resultSourceURL(result) || '未返回'}`
    els.responseDiagnostics.textContent = ''
    els.responseText.textContent = ''
    els.responseHint.textContent = ''
    els.responseHint.hidden = true
    els.responseLoading.hidden = false
    els.responseEmpty.hidden = true
    els.responseError.hidden = true
    els.downloadResponse.disabled = true
    els.responseDialog.showModal()
    try {
      const payload = await request(`/api/results/${encodeURIComponent(id)}/response`)
      // 关闭或切换结果后，旧请求不能把响应写回新弹窗。
      if (state.response.revision !== revision || !els.responseDialog.open) return
      const raw = typeof payload?.response_text === 'string' ? payload.response_text : ''
      state.response.raw = raw
      // 模型原文只作为文字显示，不能解析为网页或插入执行。
      els.responseText.textContent = raw
      els.responseDiagnostics.textContent = JSON.stringify(payload?.diagnostics || {}, null, 2)
      const hint = typeof payload?.diagnostics?.analysis_hint === 'string' ? payload.diagnostics.analysis_hint : ''
      els.responseHint.textContent = hint
      els.responseHint.hidden = !hint
      els.responseEmpty.textContent = hint || (payload?.diagnostics?.failure_kind === 'upstream_empty_output'
        ? '本次上游没有返回模型文本，下面保留事件与错误细节。'
        : '此历史记录未保存模型原文，无法从现有记录恢复；重新运行后可查看新响应。')
      els.responseEmpty.hidden = Boolean(raw)
      els.downloadResponse.disabled = !raw
    } catch (error) {
      if (state.response.revision !== revision || !els.responseDialog.open) return
      els.responseError.textContent = errorText(error, '读取原始响应失败')
      els.responseError.hidden = false
    } finally {
      if (state.response.revision === revision) els.responseLoading.hidden = true
    }
  }

  function downloadResponse() {
    if (!state.response.id || !state.response.raw) return
    const blob = new Blob([state.response.raw], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `response-${state.response.id}.txt`
    link.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }

  function setArtifactView(mode) {
    const original = mode === 'original'
    els.artifactStage.classList.toggle('original', original)
    els.artifactStage.classList.toggle('fit', !original)
    els.sizeOriginal.classList.toggle('active', original)
    els.sizeFit.classList.toggle('active', !original)
    if (original && state.artifact.dimensions) {
      els.artifactFrame.style.width = `${state.artifact.dimensions.width}px`
      els.artifactFrame.style.height = `${state.artifact.dimensions.height}px`
    } else {
      els.artifactFrame.style.width = ''
      els.artifactFrame.style.height = ''
    }
  }

  els.promptSelect.addEventListener('change', () => {
    state.promptIDs = [...els.promptSelect.querySelectorAll('[data-prompt-id]:checked')].map(input => input.value)
    if (!state.promptIDs.length) state.promptIDs = ['ak47']
    syncPromptUI()
  })
  els.quickPromptSelect?.addEventListener('change', (event) => {
    if (!event.target.matches('[data-quick-prompt-id]')) return
    state.promptIDs = [...els.quickPromptSelect.querySelectorAll('[data-quick-prompt-id]:checked')].map(input => input.value)
    if (!state.promptIDs.length) state.promptIDs = ['ak47']
    syncPromptUI()
  })
  els.promptEditButton.addEventListener('click', () => openPromptEditor('edit', state.promptIDs[0]))
  els.promptCreateButton.addEventListener('click', () => openPromptEditor('create'))
  els.promptEditorSelect.addEventListener('change', updatePromptEditorFields)
  els.promptEditorForm.addEventListener('submit', savePromptEditor)
  els.promptCancelButton.addEventListener('click', closePromptEditor)
  els.promptCancelButtonSecondary.addEventListener('click', closePromptEditor)
  els.promptEditor.addEventListener('cancel', (event) => {
    event.preventDefault()
    if (!state.promptEditor.submitting) closePromptEditor()
  })
  els.quickSource.addEventListener('change', () => {
    if (els.quickSource.value) {
      els.paste.value = ''
      resetParsePreview()
      for (const input of document.querySelectorAll('[data-account-id]:checked')) input.checked = false
    }
    renderQuickSourceJobs(els.quickSource.value, '')
  })
  els.quickSourceJob.addEventListener('change', () => {
    if (els.quickSourceJob.value) {
      els.paste.value = ''
      resetParsePreview()
      for (const input of document.querySelectorAll('[data-account-id]:checked')) input.checked = false
    }
  })
  els.paste.addEventListener('input', () => {
    if (els.paste.value.trim()) {
      els.quickSource.value = ''
      renderQuickSourceJobs('', '')
      for (const input of document.querySelectorAll('[data-account-id]:checked')) input.checked = false
    }
  })
  els.parseButton.addEventListener('click', () => scheduleParse(true))
  els.accountsToggle.addEventListener('click', () => {
    state.accountsExpanded = !state.accountsExpanded
    els.accountsBody.hidden = !state.accountsExpanded
    els.accountsToggle.textContent = state.accountsExpanded ? '收起' : '展开'
    els.accountsToggle.setAttribute('aria-expanded', String(state.accountsExpanded))
  })
  els.resultSearch.addEventListener('input', () => {
    state.resultSearch = els.resultSearch.value
    renderResults()
  })
  els.paste.addEventListener('input', () => scheduleParse())
  els.paste.addEventListener('paste', () => scheduleParse())
  els.accounts.addEventListener('change', (event) => {
    if (!event.target.matches('[data-account-id]') || !event.target.checked) return
    els.quickSource.value = ''
    renderQuickSourceJobs('', '')
    els.paste.value = ''
    resetParsePreview()
  })
  els.parallelLimit.addEventListener('input', () => {
    state.parallelDraftDirty = true
  })
  els.billingCsvFile.addEventListener('change', () => {
    setBillingFile(els.billingCsvFile.files?.[0] || null)
  })
  els.billingAnalyze.addEventListener('click', analyzeBilling)
  els.billingDownload.addEventListener('click', downloadBillingResult)
  els.billingClear.addEventListener('click', clearBilling)
  els.parallelForm.addEventListener('submit', async (event) => {
    event.preventDefault()
    const value = Number(els.parallelLimit.value)
    if (!Number.isInteger(value) || value < 1 || value > 16) {
      setNotice('并行数必须是 1 至 16 的整数', true)
      return
    }
    els.parallelForm.querySelector('button').disabled = true
    try {
      const settings = await request('/api/settings', { method: 'PATCH', body: { max_parallel_jobs: value } })
      state.parallelDraftDirty = false
      applySettings(settings)
      renderJobs()
      setNotice(`并行数已保存为 ${value}`)
    } catch (error) {
      setNotice(errorText(error, '保存并行数失败'), true)
    } finally {
      els.parallelForm.querySelector('button').disabled = false
    }
  })
  els.createJob.addEventListener('click', createJob)
  els.createRunJob.addEventListener('click', () => createBatch(true))
  els.refresh.addEventListener('click', loadAll)
  els.closeArtifact.addEventListener('click', () => els.dialog.close())
  els.download.addEventListener('click', downloadArtifact)
  els.closeResponse.addEventListener('click', () => els.responseDialog.close())
  els.downloadResponse.addEventListener('click', downloadResponse)
  els.responseDialog.addEventListener('close', () => {
    state.response = { id: null, raw: '', revision: state.response.revision + 1 }
    els.responseText.textContent = ''
    els.responseDiagnostics.textContent = ''
    els.downloadResponse.disabled = true
  })
  els.sizeOriginal.addEventListener('click', () => setArtifactView('original'))
  els.sizeFit.addEventListener('click', () => setArtifactView('fit'))
  els.jobs.addEventListener('click', (event) => {
    const run = event.target.closest('[data-run-job]')
    const toggle = event.target.closest('[data-toggle-job]')
    if (run) runJob(run.dataset.runJob)
    if (toggle) toggleJob(toggle.dataset.toggleJob)
  })
  els.jobs.addEventListener('change', (event) => {
    const select = event.target.closest('[data-interval-job]')
    if (select) updateJobInterval(select.dataset.intervalJob, select.value)
    const effort = event.target.closest('[data-effort-job]')
    if (effort) updateJobEffort(effort.dataset.effortJob, effort.value)
  })
  els.effortSelect?.addEventListener('change', () => {
    const value = els.effortSelect.value
    if ((state.config.reasoning_efforts || ['low', 'medium', 'high', 'max']).includes(value)) {
      els.effort.textContent = value
    }
  })
  els.results.addEventListener('click', (event) => {
    const responseButton = event.target.closest('[data-response-id]')
    if (responseButton) {
      openResponse(responseButton.dataset.responseId)
      return
    }
    const button = event.target.closest('[data-result-id]')
    if (button) openArtifact(button.dataset.resultId)
  })
  els.dialog.addEventListener('close', () => {
    els.artifactFrame.src = 'about:blank'
    els.artifactFrame.style.width = ''
    els.artifactFrame.style.height = ''
    state.artifact = { id: null, raw: '', dimensions: null }
  })
  els.authForm.addEventListener('submit', async (event) => {
    event.preventDefault()
    const token = els.accessToken.value.trim()
    if (!token) return
    els.authForm.querySelector('button').disabled = true
    saveAccessToken(token)
    try {
      await refreshConfig()
      if (!state.config.authenticated) throw new Error('管理口令不正确')
      els.accessToken.value = ''
      await loadAll()
      setNotice('管理区已解锁')
    } catch (error) {
      saveAccessToken('')
      state.config.authenticated = false
      state.config.csrf_token = ''
      syncAuthUI()
      setNotice(errorText(error, '管理口令不正确'), true)
    } finally {
      els.authForm.querySelector('button').disabled = false
    }
  })
  els.authToggle.addEventListener('click', async () => {
    if (state.config.authenticated) {
      saveAccessToken('')
      state.config.authenticated = false
      state.config.csrf_token = ''
      syncAuthUI()
      await loadAll()
      return
    }
    state.authPanelOpen = true
    syncAuthUI()
    els.accessToken.focus()
    els.authPanel.scrollIntoView({ behavior: 'smooth', block: 'center' })
  })
  els.connectionForm.addEventListener('submit', async (event) => {
    event.preventDefault()
    const base_url = els.connectionBaseURL.value.trim()
    const admin_key = els.connectionAdminKey.value
    if (!base_url || !admin_key) return
    const button = els.connectionForm.querySelector('button')
    button.disabled = true
    try {
      await request('/api/connection', { method: 'POST', body: { base_url, admin_key } })
      await loadAll()
      setNotice('Sub2API 连接已保存并验证')
    } catch (error) {
      setNotice(errorText(error, 'Sub2API 连接验证失败'), true)
    } finally {
      els.connectionAdminKey.value = ''
      button.disabled = false
    }
  })

  syncAuthUI()
  setArtifactView('original')
  renderBilling()
  loadAll()
  const pollTimer = setInterval(() => loadJobsResults({ quiet: true }), 10000)
  const liveTimer = setInterval(updateResultTimers, 1000)
  window.addEventListener('beforeunload', () => {
    clearInterval(pollTimer)
    clearInterval(liveTimer)
  })
})()
