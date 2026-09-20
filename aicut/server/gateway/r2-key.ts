/** R2 object keys stay unnamespaced locally. Gateway mode prefixes per tenant. */

import { gatewayEnabled } from './config.ts';
import { currentTenant } from './tenant-context.ts';
import { safeUserId } from './tenant.ts';

export function r2UploadObjectKey(name: string): string {
  if (!gatewayEnabled()) return `uploads/${name}`;
  const tenant = currentTenant();
  if (!tenant) {
    throw new Error('R2 object access requires a logged-in tenant when gateway is on');
  }
  return `uploads/tenants/${safeUserId(tenant.userId)}/${name}`;
}
