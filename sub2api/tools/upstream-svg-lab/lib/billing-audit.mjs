import { createHash, randomUUID } from 'node:crypto';

const MAX_CSV_BYTES = 8 * 1024 * 1024;
const MAX_ROWS = 100_000;
const RANK_EPSILON = 1e-10;
const RESIDUAL_LIMIT = 0.05;

const aliases = {
  model: ['model', '模型', '模型名称', 'model name'],
  reasoningEffort: ['reasoning effort', '思考程度', '推理强度', 'reasoning'],
  endpoint: ['inbound endpoint', 'endpoint', '接口', '入口'],
  billing: ['billing mode', 'billing', '计费模式', '计费方式'],
  currency: ['currency', '币种', '货币'],
  input: ['input tokens', '输入 tokens', '输入token', '输入令牌', 'prompt tokens', 'prompt token'],
  output: ['output tokens', '输出 tokens', '输出token', '输出令牌', 'completion tokens', 'completion token'],
  cacheRead: ['cache read tokens', '缓存读取 tokens', '缓存读取token', '缓存读 tokens', 'cache hit tokens'],
  cacheWrite: ['cache creation tokens', 'cache write tokens', '缓存写入 tokens', '缓存创建 tokens', '缓存写 tokens'],
  cacheWrite5m: ['cache creation 5m tokens', 'cache write 5m tokens', '缓存写入 5m tokens', '缓存写入（5m）', '缓存写入(5m)', 'cache 5m tokens'],
  cacheWrite1h: ['cache creation 1h tokens', 'cache write 1h tokens', '缓存写入 1h tokens', '缓存写入（1h）', '缓存写入(1h)', 'cache 1h tokens'],
  cachePercent: ['cache percent', '缓存比例', '缓存命中率', 'cache hit percent'],
  billedCost: ['billed cost', '计费成本', '计费金额', '实际费用', '费用', 'cost', 'charged cost'],
  originalCost: ['original cost', '原始成本', '原价成本', '原始费用', 'base cost'],
  multiplier: ['rate multiplier', '倍率', '费率倍率', '价格倍率', 'multiplier'],
  inputRate: ['input price', 'input rate', '输入单价', '输入价格', 'prompt price'],
  outputRate: ['output price', 'output rate', '输出单价', '输出价格', 'completion price'],
  cacheReadRate: ['cache read price', 'cache read rate', '缓存读取单价', '缓存读取价格'],
  cacheWrite5mRate: ['cache write 5m price', 'cache write 5m rate', '缓存写入5m单价', '缓存写入5m价格', '缓存写入（5m）单价', '缓存写入(5m)单价'],
  cacheWrite1hRate: ['cache write 1h price', 'cache write 1h rate', '缓存写入1h单价', '缓存写入1h价格', '缓存写入（1h）单价', '缓存写入(1h)单价'],
};

function normalizeHeader(value) {
  return String(value || '').replace(/^\uFEFF/, '').trim().toLowerCase()
    .replace(/[（）]/g, match => match === '（' ? '(' : ')')
    .replace(/\s+/g, ' ');
}

function findColumn(headers, names) {
  const normalized = new Map(headers.map((header, index) => [normalizeHeader(header), index]));
  for (const name of names) {
    const index = normalized.get(normalizeHeader(name));
    if (index !== undefined) return index;
  }
  return -1;
}

function parseCsv(text) {
  const rows = [];
  let row = [];
  let field = '';
  let quoted = false;
  for (let index = 0; index < text.length; index += 1) {
    const char = text[index];
    if (quoted) {
      if (char === '"' && text[index + 1] === '"') {
        field += '"';
        index += 1;
      } else if (char === '"') quoted = false;
      else field += char;
      continue;
    }
    if (char === '"') quoted = true;
    else if (char === ',') {
      row.push(field);
      field = '';
    } else if (char === '\n' || char === '\r') {
      if (char === '\r' && text[index + 1] === '\n') index += 1;
      row.push(field);
      field = '';
      if (row.some(value => String(value).trim() !== '')) rows.push(row);
      row = [];
    } else field += char;
  }
  if (quoted) throw new Error('CSV 存在未闭合的引号');
  if (field.length || row.length) {
    row.push(field);
    if (row.some(value => String(value).trim() !== '')) rows.push(row);
  }
  if (rows.length < 2) throw new Error('CSV 至少需要一行表头和一行数据');
  if (rows.length > MAX_ROWS + 1) throw new Error(`CSV 行数超过 ${MAX_ROWS} 行上限`);
  return rows;
}

function parseNumber(value, { percent = false, nonNegative = true } = {}) {
  const raw = String(value ?? '').trim();
  if (!raw) return { status: 'missing', value: null };
  const normalized = raw.replace(/[$¥￥,\s]/g, '').replace(/%$/, '');
  const parsed = Number(normalized);
  if (!Number.isFinite(parsed)) return { status: 'invalid', value: null };
  const number = percent && raw.endsWith('%') ? parsed / 100 : percent && parsed > 1 ? parsed / 100 : parsed;
  if (nonNegative && number < 0) return { status: 'negative', value: null };
  return { status: 'ok', value: number };
}

function format(value) {
  return value === null || value === undefined || !Number.isFinite(value) ? null : Number(value.toFixed(8));
}

function uniqueRates(values) {
  const output = [];
  for (const value of values.filter(item => Number.isFinite(item) && item >= 0)) {
    if (!output.some(existing => Math.abs(existing - value) <= Math.max(Math.abs(existing), Math.abs(value), 1e-9) * 0.01)) output.push(value);
  }
  return output.sort((left, right) => left - right);
}

function matrixRank(matrix) {
  if (!matrix.length || !matrix[0]?.length) return 0;
  const work = matrix.map(row => [...row]);
  const rows = work.length;
  const columns = work[0].length;
  let rank = 0;
  for (let column = 0; column < columns && rank < rows; column += 1) {
    let pivot = rank;
    for (let row = rank + 1; row < rows; row += 1) {
      if (Math.abs(work[row][column]) > Math.abs(work[pivot][column])) pivot = row;
    }
    if (Math.abs(work[pivot][column]) <= RANK_EPSILON) continue;
    [work[pivot], work[rank]] = [work[rank], work[pivot]];
    const divisor = work[rank][column];
    for (let current = column; current < columns; current += 1) work[rank][current] /= divisor;
    for (let row = 0; row < rows; row += 1) {
      if (row === rank) continue;
      const factor = work[row][column];
      if (Math.abs(factor) <= RANK_EPSILON) continue;
      for (let current = column; current < columns; current += 1) work[row][current] -= factor * work[rank][current];
    }
    rank += 1;
  }
  return rank;
}

function solveOLS(rows, dimensions, costField) {
  const usable = rows.filter(row => row.valid && row[costField] !== null && dimensions.some(field => row[field] > 0));
  const active = dimensions.filter(field => usable.some(row => row[field] > 0));
  if (!active.length || usable.length < active.length) return { status: 'insufficient', rows: usable.length, active };
  const scale = Object.fromEntries(active.map(field => [field, Math.max(...usable.map(row => row[field]), 1)]));
  const matrix = usable.map(row => active.map(field => row[field] / scale[field]));
  const rank = matrixRank(matrix);
  if (rank < active.length) return { status: 'rank_deficient', rows: usable.length, active, rank };
  const normal = Array.from({ length: active.length }, () => Array(active.length).fill(0));
  const vector = Array(active.length).fill(0);
  for (const [rowIndex, values] of matrix.entries()) {
    for (let left = 0; left < values.length; left += 1) {
      vector[left] += values[left] * usable[rowIndex][costField];
      for (let right = 0; right < values.length; right += 1) normal[left][right] += values[left] * values[right];
    }
  }
  const augmented = normal.map((row, index) => [...row, vector[index]]);
  for (let column = 0; column < active.length; column += 1) {
    let pivot = column;
    for (let row = column + 1; row < active.length; row += 1) {
      if (Math.abs(augmented[row][column]) > Math.abs(augmented[pivot][column])) pivot = row;
    }
    if (Math.abs(augmented[pivot][column]) <= RANK_EPSILON) return { status: 'rank_deficient', rows: usable.length, active, rank };
    [augmented[pivot], augmented[column]] = [augmented[column], augmented[pivot]];
    const divisor = augmented[column][column];
    for (let current = column; current <= active.length; current += 1) augmented[column][current] /= divisor;
    for (let row = 0; row < active.length; row += 1) {
      if (row === column) continue;
      const factor = augmented[row][column];
      for (let current = column; current <= active.length; current += 1) augmented[row][current] -= factor * augmented[column][current];
    }
  }
  const normalized = augmented.map(row => row[active.length]);
  const coefficients = Object.fromEntries(dimensions.map(field => [field, null]));
  active.forEach((field, index) => { coefficients[field] = normalized[index] / scale[field] * 1_000_000; });
  const errors = usable.map(row => {
    const predicted = active.reduce((sum, field) => sum + row[field] / 1_000_000 * coefficients[field], 0);
    return { absolute: Math.abs(predicted - row[costField]), actual: Math.abs(row[costField]) };
  });
  const meanAbsoluteError = errors.reduce((sum, item) => sum + item.absolute, 0) / errors.length;
  const meanActual = errors.reduce((sum, item) => sum + item.actual, 0) / errors.length;
  const relativeResidual = meanAbsoluteError / Math.max(meanActual, 1e-12);
  return {
    status: relativeResidual > RESIDUAL_LIMIT ? 'high_residual' : 'identified',
    rows: usable.length,
    active,
    rank,
    coefficients,
    mean_absolute_error: format(meanAbsoluteError),
    relative_residual: format(relativeResidual),
  };
}

function rateObject(value, source, samples = 0, observedRates = [], extra = {}) {
  const values = uniqueRates(observedRates);
  return {
    per_million_tokens: format(value),
    source,
    samples,
    observed_rates: values.map(format),
    mixed: values.length > 1,
    ...extra,
  };
}

function directRate(rows, field) {
  const observed = rows.filter(row => row.valid && row[field] !== null).map(row => row[field]);
  if (!observed.length) return null;
  const values = uniqueRates(observed);
  return { value: values.length === 1 ? values[0] : null, values, samples: observed.length };
}

function emptyRates() {
  return Object.fromEntries(['input', 'output', 'cache_read', 'cache_write_5m', 'cache_write_1h'].map(name => [
    name, rateObject(null, 'not_identifiable')
  ]));
}

function groupKey(values) {
  return [values.model, values.reasoningEffort, values.multiplier, values.endpoint, values.billing, values.currency]
    .map(value => value === null || value === undefined || value === '' ? '未标记' : String(value)).join('|');
}

function analyzeGroup(group, columns) {
  const { model, rows } = group;
  const issues = [];
  const validRows = rows.filter(row => row.valid);
  const invalidRows = rows.length - validRows.length;
  if (invalidRows) issues.push(`${invalidRows} 行存在非法或负数计费字段，已跳过计算`);
  const quality = {};
  for (const field of ['input', 'output', 'cacheRead', 'cacheWrite', 'cacheWrite5m', 'cacheWrite1h', 'billedCost', 'originalCost', 'multiplier', 'cachePercent']) {
    quality[field] = { ok: 0, zero: 0, missing: 0, invalid: 0, negative: 0 };
    for (const row of rows) {
      const status = row.statuses?.[field] || 'missing';
      if (status === 'ok' && row[field] === 0) quality[field].zero += 1;
      else quality[field][status] = (quality[field][status] || 0) + 1;
    }
  }
  const baseRates = emptyRates();
  const effectiveRates = emptyRates();
  const dimensions = ['input', 'output', 'cacheRead', 'cacheWrite5m', 'cacheWrite1h'];
  const names = ['input', 'output', 'cache_read', 'cache_write_5m', 'cache_write_1h'];
  const explicitFields = {
    input: 'inputRate',
    output: 'outputRate',
    cache_read: 'cacheReadRate',
    cache_write_5m: 'cacheWrite5mRate',
    cache_write_1h: 'cacheWrite1hRate',
  };
  for (const [name, field] of Object.entries(explicitFields)) {
    const direct = directRate(rows, field);
    if (direct) {
      baseRates[name] = rateObject(direct.value, 'csv_explicit', direct.samples, direct.values, { currency: group.currency || null });
      if (direct.values.length > 1) issues.push(`${name} 存在多个显式单价`);
    }
  }
  const originalFit = columns.originalCost >= 0 ? solveOLS(rows, dimensions, 'originalCost') : { status: 'missing' };
  const billedFit = columns.billedCost >= 0 ? solveOLS(rows, dimensions, 'billedCost') : { status: 'missing' };
  const fitRates = (fit, target) => {
    if (fit.status === 'rank_deficient') issues.push(`${target} 特征共线，无法识别单价`);
    if (fit.status === 'high_residual') issues.push(`${target} 回归残差过大，单价不可识别`);
    if (fit.status === 'insufficient') issues.push(`${target} 有效样本不足，无法稳定反推单价`);
    if (fit.status !== 'identified') return;
    for (const [index, name] of names.entries()) {
      const field = dimensions[index];
      if (!rows.some(row => row.valid && row[field] > 0)) continue;
      const targetRates = target === '原始成本' ? baseRates : effectiveRates;
      if (targetRates[name].source === 'not_identifiable') {
        targetRates[name] = rateObject(fit.coefficients[field], target === '原始成本' ? 'original_cost_ols' : 'billed_cost_ols', fit.rows, [], {
          currency: group.currency || null,
          relative_residual: fit.relative_residual,
        });
      }
    }
  };
  fitRates(originalFit, '原始成本');
  fitRates(billedFit, '有效计费');
  const aggregateWriteObserved = columns.cacheWrite >= 0 && rows.some(row => row.valid && row.cacheWrite > 0);
  const splitObserved = columns.cacheWrite5m >= 0 || columns.cacheWrite1h >= 0;
  if (aggregateWriteObserved && !splitObserved) issues.push('只有缓存写入合计列，无法区分 5 分钟与 1 小时写入价格；合计列未参与单价回归');
  if (!aggregateWriteObserved && !splitObserved) issues.push('未观察到缓存写入 token，5 分钟与 1 小时写入价格不可判定');
  if (splitObserved && !rows.some(row => row.valid && (row.cacheWrite5m > 0 || row.cacheWrite1h > 0))) issues.push('存在缓存写入分栏，但没有有效写入样本');
  const cachePercentRows = rows.filter(row => row.valid && row.cachePercent !== null && row.input + row.cacheRead > 0);
  const cachePercentErrors = cachePercentRows.filter(row => Math.abs(row.cachePercent - row.cacheRead / (row.input + row.cacheRead)) > 0.05).length;
  if (cachePercentErrors) issues.push(`缓存比例与 token 计算不一致（${cachePercentErrors} 行）`);
  const confidence = issues.some(issue => /共线|残差过大|有效样本不足|无法判定/.test(issue)) ? 'low' : (validRows.length >= 8 ? 'medium' : 'low');
  const makeTotals = field => format(validRows.reduce((sum, row) => sum + (row[field] || 0), 0));
  return {
    group_key: group.key,
    model,
    reasoning_effort: group.reasoningEffort || '',
    rate_tier: group.multiplier,
    endpoint: group.endpoint || '',
    billing_mode: group.billing || '',
    currency: group.currency || null,
    rows: rows.length,
    valid_rows: validRows.length,
    skipped_rows: invalidRows,
    quality,
    confidence,
    totals: {
      billed_cost: makeTotals('billedCost'),
      original_cost: makeTotals('originalCost'),
      input_tokens: makeTotals('input'),
      output_tokens: makeTotals('output'),
      cache_read_tokens: makeTotals('cacheRead'),
      cache_write_tokens: makeTotals('cacheWrite'),
    },
    rates: baseRates,
    base_rates: baseRates,
    effective_rates: effectiveRates,
    original_fit: originalFit.status === 'identified' ? { rows: originalFit.rows, relative_residual: originalFit.relative_residual } : { status: originalFit.status },
    billed_fit: billedFit.status === 'identified' ? { rows: billedFit.rows, relative_residual: billedFit.relative_residual } : { status: billedFit.status },
    issues,
  };
}

export function analyzeBillingCSV(input, { filename = 'usage.csv' } = {}) {
  const buffer = Buffer.isBuffer(input) ? input : Buffer.from(String(input || ''), 'utf8');
  if (!buffer.length) throw new Error('CSV 文件为空');
  if (buffer.length > MAX_CSV_BYTES) throw new Error('CSV 文件超过 8 MB 上限');
  const rows = parseCsv(buffer.toString('utf8').replace(/^\uFEFF/, ''));
  const headers = rows[0];
  const columns = Object.fromEntries(Object.entries(aliases).map(([name, candidates]) => [name, findColumn(headers, candidates)]));
  if (columns.model < 0) throw new Error('CSV 缺少 Model/模型列');
  const groups = new Map();
  let skippedRows = 0;
  for (const source of rows.slice(1)) {
    const values = Object.fromEntries(Object.entries(columns).map(([name, index]) => [name, index >= 0 ? source[index] : '']));
    const numeric = {};
    const statuses = {};
    const invalid = [];
    const numericFields = ['input', 'output', 'cacheRead', 'cacheWrite', 'cacheWrite5m', 'cacheWrite1h', 'billedCost', 'originalCost', 'inputRate', 'outputRate', 'cacheReadRate', 'cacheWrite5mRate', 'cacheWrite1hRate'];
    for (const field of numericFields) {
      const parsed = parseNumber(values[field]);
      numeric[field] = parsed.value;
      statuses[field] = parsed.status;
      if (parsed.status === 'invalid' || parsed.status === 'negative') invalid.push(`${field}:${parsed.status}`);
    }
    const percent = parseNumber(values.cachePercent, { percent: true });
    statuses.cachePercent = percent.status;
    if (percent.status === 'invalid' || percent.status === 'negative') invalid.push(`cachePercent:${percent.status}`);
    const multiplier = parseNumber(values.multiplier);
    statuses.multiplier = multiplier.status;
    if (multiplier.status === 'invalid' || multiplier.status === 'negative') invalid.push(`multiplier:${multiplier.status}`);
    const row = {
      model: String(values.model || '').trim() || '未知模型',
      reasoningEffort: String(values.reasoningEffort || '').trim(),
      endpoint: String(values.endpoint || '').trim(),
      billing: String(values.billing || '').trim(),
      currency: String(values.currency || '').trim(),
      multiplier: multiplier.value,
      cachePercent: percent.value,
      ...numeric,
      statuses,
      valid: invalid.length === 0,
      invalid,
    };
    const key = groupKey(row);
    if (!groups.has(key)) groups.set(key, { key, model: row.model, reasoningEffort: row.reasoningEffort, multiplier: row.multiplier, endpoint: row.endpoint, billing: row.billing, currency: row.currency, rows: [] });
    groups.get(key).rows.push(row);
    if (invalid.length) skippedRows += 1;
  }
  const models = [...groups.values()].map(group => analyzeGroup(group, columns));
  const issues = models.flatMap(item => item.issues.map(issue => `${item.group_key}：${issue}`));
  return {
    id: randomUUID(),
    filename: String(filename || 'usage.csv').replace(/[\\/"\u0000-\u001f]/g, '_').slice(0, 160),
    file_size: buffer.length,
    fingerprint: createHash('sha256').update(buffer).digest('hex').slice(0, 16),
    analyzed_at: new Date().toISOString(),
    row_count: rows.length - 1,
    skipped_rows: skippedRows,
    columns: headers.map(value => String(value)),
    models,
    summary: {
      model_count: models.length,
      issue_count: issues.length,
      issues,
      cache_write_split_observed: columns.cacheWrite5m >= 0 || columns.cacheWrite1h >= 0,
      conclusion: issues.length ? '发现需要人工核对的计费差异或证据不足' : '当前样本未显示明显混合费率',
    },
  };
}

function comparableRate(item, name) {
  const rate = item?.base_rates?.[name];
  return rate && Number.isFinite(rate.per_million_tokens) ? rate.per_million_tokens : null;
}

export function compareBillingAudits(current, baseline, { tolerance = 0.02 } = {}) {
  const before = new Map((baseline?.models || []).map(item => [item.group_key, item]));
  const changes = [];
  for (const item of current?.models || []) {
    const previous = before.get(item.group_key);
    if (!previous) {
      changes.push({ group_key: item.group_key, status: 'insufficient', reason: '基准中没有相同模型、费率档位或端点分组' });
      continue;
    }
    for (const name of ['input', 'output', 'cache_read', 'cache_write_5m', 'cache_write_1h']) {
      const currentRate = comparableRate(item, name);
      const baselineRate = comparableRate(previous, name);
      if (currentRate === null || baselineRate === null) {
        changes.push({ group_key: item.group_key, rate: name, status: 'insufficient', reason: '当前或基准单价不可识别' });
        continue;
      }
      const delta = (currentRate - baselineRate) / Math.max(Math.abs(baselineRate), 1e-12);
      changes.push({
        group_key: item.group_key,
        rate: name,
        baseline_per_million_tokens: baselineRate,
        current_per_million_tokens: currentRate,
        relative_change: format(delta),
        status: Math.abs(delta) > tolerance ? 'changed' : 'stable',
        currency: item.currency || previous.currency || null,
      });
    }
  }
  return {
    compared_at: new Date().toISOString(),
    tolerance,
    changes,
    summary: {
      changed: changes.filter(item => item.status === 'changed').length,
      stable: changes.filter(item => item.status === 'stable').length,
      insufficient: changes.filter(item => item.status === 'insufficient').length,
      conclusion: changes.some(item => item.status === 'changed') ? '发现可比单价发生变化' : '没有足够证据确认单价变化',
    },
  };
}

export { MAX_CSV_BYTES };
