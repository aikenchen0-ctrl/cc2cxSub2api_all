import { mkdir, readdir, readFile, stat, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { gatewayEnabled, tenantUploadQuotaBytes } from './config.ts';
import { currentTenant } from './tenant-context.ts';
import { tenantProfile } from './tenant.ts';

interface QuotaRecord {
  usedBytes: number;
  updatedAt: number;
}

const memory = new Map<string, QuotaRecord>();
let ignorePersistedQuota = false;

function quotaPath(userId: string): string {
  return join(tenantProfile(userId).rootDir, 'upload-quota-v1.json');
}

async function directoryBytes(dir: string): Promise<number> {
  let total = 0;
  let entries;
  try {
    entries = await readdir(dir, { withFileTypes: true });
  } catch {
    return 0;
  }
  for (const entry of entries) {
    if (entry.name.startsWith('.')) continue;
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      total += await directoryBytes(full);
      continue;
    }
    if (!entry.isFile()) continue;
    try {
      total += (await stat(full)).size;
    } catch {
      // skip vanished files
    }
  }
  return total;
}

async function loadRecord(userId: string): Promise<QuotaRecord> {
  const cached = memory.get(userId);
  if (cached) return cached;
  if (!ignorePersistedQuota) {
    try {
      const parsed = JSON.parse(await readFile(quotaPath(userId), 'utf8')) as QuotaRecord;
      if (typeof parsed.usedBytes === 'number' && parsed.usedBytes >= 0) {
        memory.set(userId, parsed);
        return parsed;
      }
    } catch {
      // recount from disk
    }
  }
  const usedBytes = ignorePersistedQuota ? 0 : await directoryBytes(tenantProfile(userId).mediaDir);
  const record = { usedBytes, updatedAt: Date.now() };
  memory.set(userId, record);
  return record;
}

async function persist(userId: string, record: QuotaRecord): Promise<void> {
  memory.set(userId, record);
  try {
    await mkdir(tenantProfile(userId).rootDir, { recursive: true, mode: 0o700 });
    await writeFile(quotaPath(userId), JSON.stringify(record), { mode: 0o600 });
  } catch {
    // In-memory accounting still holds if the quota file cannot be written.
  }
}

export class TenantUploadQuotaError extends Error {
  readonly remainingBytes: number;
  constructor(remainingBytes: number) {
    super(`上传配额不足，剩余 ${Math.max(0, remainingBytes)} 字节`);
    this.name = 'TenantUploadQuotaError';
    this.remainingBytes = remainingBytes;
  }
}

export async function assertTenantUploadQuota(additionalBytes: number): Promise<void> {
  if (!gatewayEnabled()) return;
  const tenant = currentTenant();
  if (!tenant) throw new TenantUploadQuotaError(0);
  const cap = tenantUploadQuotaBytes();
  const record = await loadRecord(tenant.userId);
  const remaining = cap - record.usedBytes;
  if (additionalBytes > remaining) throw new TenantUploadQuotaError(remaining);
}

export async function recordTenantUpload(bytes: number): Promise<void> {
  if (!gatewayEnabled() || bytes <= 0) return;
  const tenant = currentTenant();
  if (!tenant) return;
  const record = await loadRecord(tenant.userId);
  await persist(tenant.userId, {
    usedBytes: record.usedBytes + bytes,
    updatedAt: Date.now(),
  });
}

export async function releaseTenantUpload(bytes: number): Promise<void> {
  if (!gatewayEnabled() || bytes <= 0) return;
  const tenant = currentTenant();
  if (!tenant) return;
  const record = await loadRecord(tenant.userId);
  await persist(tenant.userId, {
    usedBytes: Math.max(0, record.usedBytes - bytes),
    updatedAt: Date.now(),
  });
}

export function resetUploadQuotaForTests(): void {
  memory.clear();
  ignorePersistedQuota = true;
}