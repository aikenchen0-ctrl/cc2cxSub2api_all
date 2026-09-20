import { AsyncLocalStorage } from 'node:async_hooks';

export interface TenantContext {
  userId: string;
  email: string;
}

const tenantStore = new AsyncLocalStorage<TenantContext>();

export function runWithTenant<T>(tenant: TenantContext, fn: () => T): T {
  return tenantStore.run(tenant, fn);
}

export function enterTenant(tenant: TenantContext): void {
  tenantStore.enterWith(tenant);
}

export function currentTenant(): TenantContext | undefined {
  return tenantStore.getStore();
}
