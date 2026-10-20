import { randomUUID } from 'node:crypto';
import { PROMPTS } from './prompts.mjs';

const MAX_PROMPTS = 100;
export const MAX_PROMPT_LENGTH = 32000;

function validate(input) {
  if (typeof input.label !== 'string' || !input.label.trim() || input.label.trim().length > 60) {
    throw new Error('提示词名称须为 1 至 60 个字符');
  }
  if (typeof input.text !== 'string' || !input.text.trim() || input.text.length > MAX_PROMPT_LENGTH) {
    throw new Error('提示词原文须为 1 至 32000 个字符');
  }
  return { label: input.label.trim(), text: input.text };
}

export class PromptCatalog {
  constructor(store) { this.store = store; }
  list(state = this.store.state) {
    const defaults = PROMPTS.map(item => ({ ...item, version: 1, built_in: true }));
    for (const saved of state.prompt_options || []) {
      const index = defaults.findIndex(item => item.id === saved.id);
      if (index >= 0) defaults[index] = { ...saved, built_in: true };
      else defaults.push({ ...saved, built_in: false });
    }
    return defaults;
  }
  get(id = 'ak47', state = this.store.state) {
    const item = this.list(state).find(preset => preset.id === id);
    if (!item) throw new Error('提示词类型无效或选项不存在，请刷新页面后重新选择');
    return item;
  }
  async create(input) {
    const fields = validate(input);
    return this.store.change(state => {
      if (this.list(state).length >= MAX_PROMPTS) throw new Error('提示词选项已达 100 个上限');
      if (this.list(state).some(item => item.label === fields.label)) throw new Error('提示词名称已存在，请换一个名称');
      const preset = { id: `custom-${randomUUID()}`, ...fields, version: 1, built_in: false, updated_at: new Date().toISOString() };
      state.prompt_options = [...(state.prompt_options || []), preset];
      return preset;
    });
  }
  async update(id, input) {
    return this.store.change(state => {
      const current = this.get(id, state);
      if (!Number.isInteger(input.version) || input.version !== current.version) {
        throw Object.assign(new Error('提示词已被更新，请刷新后再编辑，避免覆盖其他修改'), { status: 409, code: 'PROMPT_VERSION_CONFLICT' });
      }
      const fields = validate({ label: input.label ?? current.label, text: input.text });
      if (this.list(state).some(item => item.id !== id && item.label === fields.label)) throw new Error('提示词名称已存在，请换一个名称');
      const preset = { ...current, ...fields, version: current.version + 1, updated_at: new Date().toISOString() };
      state.prompt_options = [...(state.prompt_options || []).filter(item => item.id !== id), preset];
      return preset;
    });
  }
}
