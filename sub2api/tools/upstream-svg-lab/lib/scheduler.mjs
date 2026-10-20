import { randomUUID, createHash } from 'node:crypto';
import { accountSource } from './admin.mjs';
import { cleanSourceURL, parseCredentials } from './credentials.mjs';
import { MODELS, probe, REASONING_EFFORTS, DEFAULT_TIMEOUT_MS, DEFAULT_IDLE_TIMEOUT_MS, DEFAULT_MAX_OUTPUT_TOKENS, normalizeMaxOutputTokens } from './probe.mjs';
import { getPrompt } from './prompts.mjs';
import { PromptCatalog } from './prompt-catalog.mjs';

const MAX_PARALLEL_LIMIT = 16;
const PENDING_QUEUE_LIMIT = 200;

function normalizeMaxParallel(value) {
  const number = Number(value);
  if (!Number.isInteger(number) || number < 1 || number > MAX_PARALLEL_LIMIT) {
    throw new Error('并行任务数须为 1 至 16 的整数');
  }
  return number;
}

function normalizeReasoningEffort(value) {
  const effort = String(value || 'low');
  if (!REASONING_EFFORTS.includes(effort)) throw new Error('思考程度必须是 low、medium、high 或 max');
  return effort;
}

function normalizeGroup(value) {
  const group = String(value ?? '').trim();
  if (group.length > 80) throw new Error('分组名称不能超过 80 个字符');
  return group;
}

export class Scheduler {
  constructor({ store, admin, probeImpl = probe, now = () => Date.now(), timeoutMs = DEFAULT_TIMEOUT_MS, idleTimeoutMs = DEFAULT_IDLE_TIMEOUT_MS, maxOutputTokens = DEFAULT_MAX_OUTPUT_TOKENS, reasoningEffort = 'low', maxParallelJobs = 6 }) {
    const saved = store?.state?.settings?.max_parallel_jobs;
    const selected = saved === undefined ? maxParallelJobs : saved;
    Object.assign(this, { store, admin, probeImpl, now, timeoutMs, idleTimeoutMs,
      maxOutputTokens: normalizeMaxOutputTokens(maxOutputTokens), reasoningEffort: normalizeReasoningEffort(reasoningEffort) });
    this.promptCatalog = new PromptCatalog(store);
    this.maxParallelJobs = normalizeMaxParallel(selected);
    this.runningIdsSet = new Set();
    this.liveResults = new Map();
    this.pendingJobs = [];
    this.pendingIdsSet = new Set();
    this.stopped = false;
  }
  get active() { return this.activeCount > 0; }
  get activeCount() { return this.runningIdsSet.size; }
  get activeModelCount() { return this.liveResults.size; }
  get queueCount() { return this.pendingJobs.length; }
  getLiveResults() {
    return [...this.liveResults.values()].map(({ started_ms, ...result }) => ({
      ...result,
      duration_ms: Math.max(0, this.now() - started_ms)
    }));
  }
  setMaxParallelJobs(value) {
    this.maxParallelJobs = normalizeMaxParallel(value);
    return this.maxParallelJobs;
  }
  isQueued(id) { return this.pendingIdsSet.has(id); }
  cancelQueued(id) {
    if (!this.pendingIdsSet.delete(id)) return false;
    this.pendingJobs = this.pendingJobs.filter(item => item.id !== id);
    return true;
  }
  runError(id, manual = true) {
    if (this.stopped) return '调度器已停止，不能启动新测试';
    const job = this.store.state.jobs.find(item => item.id === id);
    if (!job) return '计划不存在';
    if (this.runningIdsSet.has(id)) return '该任务正在运行，请等待本轮完成';
    if (this.pendingIdsSet.has(id)) return '该任务已在等待队列中，请等待空闲名额';
    if (this.activeCount >= this.maxParallelJobs) return `同时运行的任务已达 ${this.maxParallelJobs} 个上限，请稍后重试`;
    if (!manual && !job.enabled) return '计划已暂停';
    return '';
  }
  hasUsableSecret(job) {
    if (!job?.secret || !job.source_url || typeof this.store.open !== 'function') return false;
    try {
      const secret = this.store.open(job.secret);
      return typeof secret === 'string' && secret.length > 0;
    } catch {
      return false;
    }
  }
  sourceCandidates() {
    const candidates = [];
    const jobs = [...(this.store.state.jobs || [])]
      .map((job, index) => ({ job, index }))
      .sort((a, b) => {
        const left = Date.parse(a.job.last_run_at || a.job.created_at || '') || 0;
        const right = Date.parse(b.job.last_run_at || b.job.created_at || '') || 0;
        return right - left || b.index - a.index;
      })
      .map(item => item.job);
    for (const job of jobs) {
      if (this.hasUsableSecret(job)) {
        candidates.push({
          source_url: job.source_url,
          group: job.group || '',
          source_job_id: job.id,
          job,
          account_ids: [],
          sources: {},
          key_configured: true,
          key_fingerprint: job.key_fingerprint || ''
        });
      }
      for (const rawId of job.account_ids || []) {
        const id = Number(rawId);
        const source = job.sources?.[rawId] || job.sources?.[id];
        if (!Number.isSafeInteger(id) || !source?.url) continue;
        candidates.push({
          source_url: source.url,
          group: job.group || '',
          source_job_id: job.id,
          job,
          account_ids: [id],
          sources: { [id]: { url: source.url, name: source.name || '' } },
          key_configured: false,
          key_fingerprint: ''
        });
      }
    }
    return candidates;
  }
  sourceList() {
    const grouped = new Map();
    for (const candidate of this.sourceCandidates()) {
      const groupKey = `${candidate.source_url}\u0000${candidate.group || ''}`;
      const existing = grouped.get(groupKey);
      if (existing) {
        if (!existing.jobs.some(item => item.id === candidate.source_job_id)) {
          existing.jobs.push({
            id: candidate.source_job_id,
            name: candidate.job.name || '',
            prompt_id: candidate.job.prompt_id || 'ak47',
            prompt_label: candidate.job.prompt_label || candidate.job.prompt_id || 'ak47',
            group: candidate.job.group || '',
            last_test_at: candidate.job.last_run_at || candidate.job.created_at || '',
            key_configured: candidate.key_configured
          });
        }
        if (candidate.key_configured && !existing.key_configured) {
          existing.source_ref = candidate.source_job_id;
          existing.source_job_id = candidate.source_job_id;
          existing.job_id = candidate.source_job_id;
          existing.job_name = candidate.job.name || '';
          existing.account_ids = candidate.account_ids;
          existing.sources = candidate.sources;
          existing.key_configured = true;
          existing.key_fingerprint = candidate.key_fingerprint;
        }
        existing.prompt_ids = [...new Set([...existing.prompt_ids, candidate.job.prompt_id || 'ak47'])];
        continue;
      }
      const job = {
        id: candidate.source_job_id,
        name: candidate.job.name || '',
        prompt_id: candidate.job.prompt_id || 'ak47',
        prompt_label: candidate.job.prompt_label || candidate.job.prompt_id || 'ak47',
        group: candidate.job.group || '',
        last_test_at: candidate.job.last_run_at || candidate.job.created_at || '',
        key_configured: candidate.key_configured
      };
      grouped.set(groupKey, {
        source_ref: candidate.source_job_id,
        source_job_id: candidate.source_job_id,
        source_url: candidate.source_url,
        group: candidate.group || '',
        job_id: candidate.source_job_id,
        job_name: candidate.job.name || '',
        account_ids: candidate.account_ids,
        sources: candidate.sources,
        key_configured: candidate.key_configured,
        key_fingerprint: candidate.key_fingerprint,
        prompt_ids: [candidate.job.prompt_id || 'ak47'],
        last_test_at: candidate.job.last_run_at || candidate.job.created_at || '',
        jobs: [job]
      });
    }
    return [...grouped.values()];
  }
  resolveSourceReference(input) {
    const sourceJobId = typeof input.source_job_id === 'string' ? input.source_job_id.trim() : '';
    const sourceURLRef = typeof input.source_url_ref === 'string' ? input.source_url_ref.trim() : '';
    if (sourceJobId && sourceURLRef) throw new Error('source_job_id 与 source_url_ref 不能同时使用');
    if (!sourceJobId && !sourceURLRef) return null;
    if (sourceJobId) {
      const job = this.store.state.jobs.find(item => item.id === sourceJobId);
      if (!job) throw new Error('快捷来源任务不存在');
      if (this.hasUsableSecret(job)) {
        return {
          account_ids: [], sources: {}, source_url: job.source_url, secret: job.secret,
          key_fingerprint: job.key_fingerprint || '', source_ref: job.id, source_url_ref: job.source_url
        };
      }
      if (Array.isArray(job.account_ids) && job.account_ids.length) {
        const accountIds = [...new Set(job.account_ids)];
        const sources = {};
        for (const id of accountIds) {
          const source = job.sources?.[id] || job.sources?.[String(id)];
          if (source?.url) sources[id] = { url: source.url, name: source.name || '' };
        }
        if (Object.keys(sources).length) {
          return { account_ids: accountIds, sources, source_url: '', secret: undefined,
            key_fingerprint: '', source_ref: job.id, source_url_ref: '' };
        }
      }
      throw new Error('快捷来源任务没有可复用的密钥或账号');
    }
    let requestedURL;
    try {
      requestedURL = cleanSourceURL(sourceURLRef);
    } catch {
      throw new Error('source_url_ref 必须是有效的上游 URL');
    }
    const candidate = this.sourceCandidates().find(item => item.source_url === requestedURL);
    if (!candidate) throw new Error('快捷来源 URL 不存在或已失效');
    if (candidate.key_configured) {
      return {
        account_ids: [], sources: {}, source_url: candidate.source_url, secret: candidate.job.secret,
        key_fingerprint: candidate.key_fingerprint, source_ref: candidate.source_job_id,
        source_url_ref: candidate.source_url
      };
    }
    if (candidate.account_ids.length) {
      return {
        account_ids: candidate.account_ids, sources: candidate.sources, source_url: '',
        secret: undefined, key_fingerprint: '', source_ref: candidate.source_job_id,
        source_url_ref: candidate.source_url
      };
    }
    throw new Error('快捷来源没有可复用的密钥或账号');
  }
  async prepareJob(input, { batchId = '', batchSize = 1, promptId } = {}) {
    const requestedPrompt = promptId ?? input.prompt_id ?? 'ak47';
    const selectedPrompt = this.promptCatalog.get(requestedPrompt);
    const reasoningEffort = normalizeReasoningEffort(input.reasoning_effort ?? this.reasoningEffort);
    const models = [...new Set(input.models || MODELS)];
    if (!models.length || models.length > 8 || models.some(model => typeof model !== 'string' || !/^[\w./:-]{1,100}$/.test(model))) throw new Error('请选择 1 至 8 个有效模型');
    const interval = Number(input.interval_minutes ?? 30);
    const group = normalizeGroup(input.group);
    if (!Number.isInteger(interval) || interval < 0 || interval > 10080) throw new Error('测试间隔须为 0 至 10080 分钟的整数');
    if (interval !== 0 && interval < 5) throw new Error('自动测试间隔至少为 5 分钟');
    const ids = [...new Set(input.account_ids || [])];
    if (ids.some(id => !Number.isSafeInteger(id) || id < 1) || ids.length > 100) throw new Error('账号编号无效或超过 100 个');
    const hasPasted = typeof input.pasted_text === 'string' && input.pasted_text.trim().length > 0;
    const reference = this.resolveSourceReference(input);
    const selectors = Number(hasPasted) + Number(ids.length > 0) + Number(!!reference);
    if (selectors !== 1) throw new Error('请选择现有上游账号、粘贴一个上游或快捷来源，三者选择其一');
    const source = hasPasted ? parseCredentials(input.pasted_text) : null;
    const resolvedIds = reference ? reference.account_ids : ids;
    const sources = reference ? { ...reference.sources } : {};
    for (const id of resolvedIds) {
      const account = await this.admin.account(id);
      const checked = accountSource(account);
      if (!checked.eligible) throw new Error(`账号 #${id}：${checked.reason}`);
      sources[id] = { url: checked.source_url, name: account.name };
    }
    const baseName = String(input.name || `${selectedPrompt.label} 上游对比`).trim().slice(0, 100);
    const label = batchSize > 1 ? ` - ${selectedPrompt.label}` : '';
    const name = `${baseName}${label}`.slice(0, 100);
    return {
      id: randomUUID(), name, batch_id: batchId || undefined,
      group,
      account_ids: resolvedIds, sources, models, interval_minutes: interval, enabled: input.enabled !== false,
      prompt_id: selectedPrompt.id, prompt_label: selectedPrompt.label, prompt_version: selectedPrompt.version,
      prompt: selectedPrompt.text, reasoning_effort: reasoningEffort, running: false,
      source_url: source?.url || reference?.source_url || '', secret: source ? this.store.seal(source.key) : reference?.secret,
      key_fingerprint: source ? createHash('sha256').update(source.key).digest('hex').slice(0, 12) : (reference?.key_fingerprint || ''),
      source_ref: reference?.source_ref || undefined, source_url_ref: reference?.source_url_ref || '',
      created_at: new Date(this.now()).toISOString(), next_run_at: interval ? new Date(this.now() + interval * 60_000).toISOString() : null
    };
  }
  async commitJobs(jobs) {
    if (!Array.isArray(jobs) || !jobs.length) throw new Error('至少需要创建一个计划');
    await this.store.change(state => {
      if (state.jobs.length + jobs.length > 200) throw new Error('计划数量已达 200 个上限');
      const existing = new Set(state.jobs.map(item => item.name));
      for (const job of jobs) {
        const baseName = job.name || `${job.prompt_label || 'AK47'} 上游对比`;
        let uniqueName = baseName;
        let suffix = 2;
        while (existing.has(uniqueName)) {
          const tail = ` (${suffix++})`;
          uniqueName = `${baseName.slice(0, Math.max(1, 100 - tail.length))}${tail}`;
        }
        job.name = uniqueName;
        existing.add(uniqueName);
      }
      state.jobs.push(...jobs);
    });
    return jobs.map(job => this.store.publicJob(job));
  }
  async create(input) {
    const job = await this.prepareJob(input);
    return (await this.commitJobs([job]))[0];
  }
  async createBatch(input) {
    if (!Array.isArray(input.prompt_ids) || !input.prompt_ids.length) throw new Error('请选择至少一种提示词');
    const promptIds = [...new Set(input.prompt_ids)];
    if (promptIds.length > 16) throw new Error('批量提示词数量不能超过 16 个');
    if (promptIds.some(promptId => typeof promptId !== 'string')) throw new Error('提示词标识必须是字符串');
    if (input.run_now === true && this.stopped) throw new Error('调度器已停止，不能启动新测试');
    const batchId = randomUUID();
    const jobs = [];
    for (const promptId of promptIds) {
      jobs.push(await this.prepareJob(input, { batchId, batchSize: promptIds.length, promptId }));
    }
    const items = await this.commitJobs(jobs);
    const startedJobIds = [];
    const queuedJobIds = [];
    if (input.run_now === true) {
      for (const job of items) {
        const status = this.queueRun(job.id, true);
        if (status.status === 'running') startedJobIds.push(job.id);
        if (status.status === 'queued') queuedJobIds.push(job.id);
      }
    }
    return {
      batch_id: batchId, items, queued: input.run_now === true,
      started_job_ids: startedJobIds, queued_job_ids: queuedJobIds
    };
  }
  async update(id, patch) {
    if (['prompt_id', 'prompt_label', 'prompt'].some(field => Object.hasOwn(patch, field))) {
      if (this.runningIdsSet.has(id)) throw new Error('任务正在运行，不能修改提示词');
      throw new Error('提示词在创建任务时确定，请新建任务切换提示词');
    }
    if (typeof patch.enabled !== 'boolean' && patch.interval_minutes === undefined && patch.reasoning_effort === undefined) throw new Error('请提供启用状态、间隔或思考程度');
    const nextReasoningEffort = patch.reasoning_effort === undefined
      ? null
      : normalizeReasoningEffort(patch.reasoning_effort);
    if (patch.interval_minutes !== undefined) {
      const interval = Number(patch.interval_minutes);
      if (!Number.isInteger(interval) || interval < 0 || interval > 10080 || (interval !== 0 && interval < 5)) {
        throw new Error('测试间隔须为 0（仅手动）或 5 至 10080 分钟');
      }
    }
    const output = await this.store.change(state => {
      const job = state.jobs.find(item => item.id === id);
      if (!job) throw new Error('计划不存在');
      if (typeof patch.interval_minutes !== 'undefined') {
        job.interval_minutes = Number(patch.interval_minutes);
        if (!job.interval_minutes) job.enabled = false;
      }
      if (nextReasoningEffort) job.reasoning_effort = nextReasoningEffort;
      if (typeof patch.enabled === 'boolean' && job.interval_minutes) job.enabled = patch.enabled;
      if (job.enabled && job.interval_minutes) job.next_run_at = new Date(this.now() + job.interval_minutes * 60_000).toISOString();
      if (!job.interval_minutes) job.next_run_at = null;
      return this.store.publicJob(job);
    });
    if (patch.enabled === false || Number(patch.interval_minutes) === 0) this.cancelQueued(id);
    return output;
  }
  queueRun(id, manual = true) {
    if (this.stopped) throw new Error('调度器已停止，不能启动新测试');
    const job = this.store.state.jobs.find(item => item.id === id);
    if (!job) throw new Error('计划不存在');
    if (this.runningIdsSet.has(id)) return { status: 'running', id };
    if (this.pendingIdsSet.has(id)) return { status: 'queued', id };
    if (!manual && !job.enabled) return { status: 'skipped', id };
    if (this.activeCount < this.maxParallelJobs) {
      void this.run(id, manual).catch(() => console.error('手动测试执行失败，请检查上游配置'));
      return { status: 'running', id };
    }
    if (this.pendingJobs.length >= PENDING_QUEUE_LIMIT) throw new Error('等待队列已满，请稍后重试');
    this.pendingJobs.push({ id, manual });
    this.pendingIdsSet.add(id);
    return { status: 'queued', id };
  }
  drainPending() {
    const runs = [];
    while (!this.stopped && this.activeCount < this.maxParallelJobs && this.pendingJobs.length) {
      const pending = this.pendingJobs.shift();
      this.pendingIdsSet.delete(pending.id);
      if (this.runningIdsSet.has(pending.id)) continue;
      if (!this.store.state.jobs.some(item => item.id === pending.id)) continue;
      runs.push(this.run(pending.id, pending.manual));
    }
    return runs;
  }
  async run(id, manual = false) {
    const problem = this.runError(id, manual);
    if (problem) {
      if (!manual && problem === '计划已暂停') return;
      throw new Error(problem);
    }
    // 在首次等待前预占名额，避免同一任务重入或并发请求突破上限。
    this.runningIdsSet.add(id);
    let job;
    try {
      await this.store.change(state => {
        const current = state.jobs.find(item => item.id === id);
        if (!current) throw new Error('计划不存在');
        const selectedPrompt = current.prompt
          ? { id: current.prompt_id || 'ak47', label: current.prompt_label || getPrompt('ak47').label, text: current.prompt, version: current.prompt_version || 1 }
          : this.promptCatalog.get(current.prompt_id || 'ak47');
        current.prompt_id = selectedPrompt.id;
        current.prompt_label = selectedPrompt.label;
        current.prompt_version = selectedPrompt.version;
        current.prompt = selectedPrompt.text;
        current.reasoning_effort = normalizeReasoningEffort(current.reasoning_effort || this.reasoningEffort);
        current.running = true;
        current.last_run_at = new Date(this.now()).toISOString();
        current.next_run_at = current.interval_minutes ? new Date(this.now() + current.interval_minutes * 60_000).toISOString() : null;
        // 固定本轮输入；后续存储回滚或间隔调整不应改变已开始测试的内容。
        job = structuredClone(current);
      });
      const promptSnapshot = { prompt_id: job.prompt_id, prompt_label: job.prompt_label, prompt_version: job.prompt_version, prompt: job.prompt };
      const targets = job.account_ids.length ? job.account_ids : [null];
      // 同一来源的模型一起启动；来源之间依次推进，限制单个任务的请求规模。
      const models = [...job.models];
      for (const accountId of targets) {
        if (this.stopped) return;
        let target;
        try {
          if (accountId === null) target = { url: job.source_url, key: this.store.open(job.secret), name: job.name };
          else target = await this.admin.source(accountId);
        } catch (error) {
          if (this.stopped) return;
          for (const model of models) await this.store.saveResult({
            job_id: job.id, job_name: job.name, group: job.group || '', account_id: accountId, source_url: accountId ? job.sources?.[accountId]?.url || '' : job.source_url, request_url: '',
            model, reasoning_effort: job.reasoning_effort, ...promptSnapshot, status: 'failed', duration_ms: 0,
            started_at: new Date(this.now()).toISOString(), error: `${error.message || '无法读取上游配置'}；本轮未发送请求`, response_text: '', svg: ''
          });
          continue;
        }
        const modelRuns = models.map(async model => {
          if (this.stopped) return;
          const started = this.now();
          const liveId = randomUUID();
          this.liveResults.set(liveId, {
            id: liveId, live: true, job_id: job.id, job_name: job.name, group: job.group || '',
            account_id: accountId, account_name: target.name,
            source_url: target.url, request_url: '', model, status: 'running',
            max_output_tokens: this.maxOutputTokens,
            reasoning_effort: job.reasoning_effort, ...promptSnapshot,
            started_at: new Date(started).toISOString(), started_ms: started
          });
          let result;
          try {
            try {
              result = await this.probeImpl({ ...target, model, reasoning_effort: job.reasoning_effort, maxOutputTokens: this.maxOutputTokens, timeoutMs: this.timeoutMs, idleTimeoutMs: this.idleTimeoutMs, ...promptSnapshot });
            } catch (error) {
              const message = String(error?.message || '测试执行失败');
              result = {
                source_url: target.url, request_url: '', model, status: 'failed', failure_kind: 'local_probe_error',
                started_at: new Date(started).toISOString(), duration_ms: Math.max(0, this.now() - started),
                error: target.key ? message.split(target.key).join('[密钥已隐藏]') : message, response_text: '', svg: ''
              };
            }
            await this.store.saveResult({
              ...result, ...promptSnapshot, reasoning_effort: job.reasoning_effort,
              job_id: job.id, job_name: job.name, group: job.group || '', account_id: accountId, account_name: target.name
            });
          } finally {
            this.liveResults.delete(liveId);
          }
        });
        // 等待所有模型及结果保存收尾，再释放任务名额；单路失败不提前退出其他请求。
        const settled = await Promise.allSettled(modelRuns);
        const failed = settled.find(item => item.status === 'rejected');
        if (failed) throw failed.reason;
      }
      await this.store.change(state => {
        const current = state.jobs.find(item => item.id === id);
        if (current) current.round_count = (current.round_count || 0) + 1;
      });
    } finally {
      try {
        await this.store.change(state => {
          const current = state.jobs.find(item => item.id === id);
          if (current) current.running = false;
        });
      } finally {
        this.runningIdsSet.delete(id);
      }
    }
  }
  async tick() {
    if (this.stopped) return;
    const pendingRuns = this.drainPending();
    if (this.activeCount >= this.maxParallelJobs) {
      await Promise.allSettled(pendingRuns);
      return;
    }
    const due = this.store.state.jobs.filter(item => item.enabled && !this.runningIdsSet.has(item.id)
      && item.next_run_at && Date.parse(item.next_run_at) <= this.now())
      .sort((a, b) => Date.parse(a.next_run_at) - Date.parse(b.next_run_at));
    const runs = [...pendingRuns];
    for (const job of due) {
      if (this.stopped || this.activeCount >= this.maxParallelJobs) break;
      runs.push(this.run(job.id));
    }
    const results = await Promise.allSettled(runs);
    const failure = results.find(result => result.status === 'rejected');
    if (failure) throw failure.reason;
  }
  start() {
    this.timer = setInterval(() => { this.tick().catch(() => console.error('定时测试执行失败，请检查本地状态目录')); }, 5000);
    return this;
  }
  stop() {
    this.stopped = true;
    this.pendingJobs = [];
    this.pendingIdsSet.clear();
    clearInterval(this.timer);
  }
}
