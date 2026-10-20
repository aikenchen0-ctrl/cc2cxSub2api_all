import fs from 'node:fs/promises';
import path from 'node:path';
import { randomBytes, createCipheriv, createDecipheriv, randomUUID } from 'node:crypto';

export class Store {
  constructor(directory) {
    this.directory = path.resolve(directory);
    this.state = { jobs: [], results: [], billing_audits: [] };
    this.queue = Promise.resolve();
    this.resultQueue = Promise.resolve();
  }
  async init() {
    await fs.mkdir(path.join(this.directory, 'artifacts'), { recursive: true, mode: 0o700 });
    try {
      this.key = await fs.readFile(path.join(this.directory, 'vault.key'));
    } catch (error) {
      if (error.code !== 'ENOENT') throw error;
      this.key = randomBytes(32);
      await fs.writeFile(path.join(this.directory, 'vault.key'), this.key, { mode: 0o600, flag: 'wx' });
    }
    try {
      this.state = JSON.parse(await fs.readFile(path.join(this.directory, 'state.json'), 'utf8'));
    } catch (error) {
      if (error.code !== 'ENOENT') throw new Error('状态文件无法读取；为保护历史记录，服务停止启动');
    }
    this.state.jobs ||= [];
    this.state.results ||= [];
    this.state.billing_audits ||= [];
    // 重启后释放本地执行标记；下一次到期仅补执行一轮，不重放所有漏掉的时间点。
    const names = new Set();
    for (const job of this.state.jobs) {
      job.running = false;
      const base = String(job.name || 'AK47 上游对比').slice(0, 100);
      let name = base;
      let suffix = 2;
      while (names.has(name)) {
        const tail = ` (${suffix++})`;
        name = `${base.slice(0, Math.max(1, 100 - tail.length))}${tail}`;
      }
      job.name = name;
      names.add(name);
    }
    return this;
  }
  seal(text) {
    const iv = randomBytes(12);
    const cipher = createCipheriv('aes-256-gcm', this.key, iv);
    return Buffer.concat([iv,
      cipher.update(text, 'utf8'), cipher.final(), cipher.getAuthTag()]).toString('base64');
  }
  open(value) {
    const bytes = Buffer.from(value, 'base64');
    const decipher = createDecipheriv('aes-256-gcm', this.key, bytes.subarray(0, 12));
    decipher.setAuthTag(bytes.subarray(-16));
    return Buffer.concat([decipher.update(bytes.subarray(12, -16)), decipher.final()]).toString('utf8');
  }
  publicJob(job) {
    const { secret, ...safe } = job;
    return { ...safe, key_configured: !!secret };
  }
  async change(mutator) {
    const work = this.queue.then(async () => {
      const previous = structuredClone(this.state);
      try {
        const output = await mutator(this.state);
        const temp = path.join(this.directory, `${randomUUID()}.tmp`);
        await fs.writeFile(temp, JSON.stringify(this.state), { mode: 0o600 });
        await fs.rename(temp, path.join(this.directory, 'state.json'));
        return output;
      } catch (error) {
        this.state = previous;
        throw error;
      }
    });
    this.queue = work.catch(() => {});
    return work;
  }
  saveResult(result) {
    // 文件写入、登记与清理必须属于同一串行操作，避免清理另一个任务尚未登记的产物。
    const work = this.resultQueue.then(() => this.writeResult(result));
    this.resultQueue = work.catch(() => {});
    return work;
  }
  async writeResult(result) {
    const { svg, response_text, ...metadata } = result;
    const id = randomUUID();
    // 原始响应只保存在本地私有目录，不进入结果列表或请求日志。
    await fs.writeFile(path.join(this.directory, 'artifacts', `${id}.json`), JSON.stringify({ svg, response_text }), { mode: 0o600 });
    try {
      await this.change(state => {
        state.results.unshift({ ...metadata, id, has_svg: !!svg, has_response_text: Boolean(response_text?.trim()) });
        state.results = state.results.slice(0, 1000);
      });
    } catch (error) {
      await fs.unlink(path.join(this.directory, 'artifacts', `${id}.json`)).catch(() => {});
      throw error;
    }
    // 同步删除超过保留数量的原始响应，防止长期运行积累文件。
    const retained = new Set(this.state.results.map(item => `${item.id}.json`));
    for (const name of await fs.readdir(path.join(this.directory, 'artifacts'))) {
      if (/^[a-f\d-]+\.json$/.test(name) && !retained.has(name)) await fs.unlink(path.join(this.directory, 'artifacts', name));
    }
    return id;
  }
  async artifact(id) {
    if (!this.state.results.some(result => result.id === id)) throw new Error('结果不存在');
    return JSON.parse(await fs.readFile(path.join(this.directory, 'artifacts', `${id}.json`), 'utf8'));
  }
  async saveBillingAudit(audit) {
    if (!audit || typeof audit !== 'object' || !audit.id) throw new Error('计费审计结果无效');
    await this.change(state => {
      state.billing_audits = [audit, ...(state.billing_audits || [])].slice(0, 100);
    });
    return audit;
  }
}
