import { join } from 'node:path';
import { homedir } from 'node:os';
import {
  resolveRuntimeProfile,
  type RuntimeProfile,
} from '../runtime-profile.ts';
import { currentTenant, type TenantContext } from './tenant-context.ts';

export type { TenantContext } from './tenant-context.ts';
export { currentTenant, runWithTenant } from './tenant-context.ts';

export function safeUserId(userId: string): string {
  const cleaned = userId.replace(/[^a-zA-Z0-9_-]/g, '_').slice(0, 80);
  return cleaned || 'anonymous';
}

export function tenantRoot(userId: string): string {
  return join(homedir(), '.openchatcut', 'tenants', safeUserId(userId));
}

export function tenantProfile(userId: string): RuntimeProfile {
  const rootDir = tenantRoot(userId);
  return resolveRuntimeProfile(
    { OPENCHATCUT_DATA_DIR: rootDir },
    { homeDir: homedir(), cwd: process.cwd() },
  );
}

export function activeRuntimeProfile(fallback: () => RuntimeProfile): RuntimeProfile {
  const tenant = currentTenant();
  return tenant ? tenantProfile(tenant.userId) : fallback();
}
