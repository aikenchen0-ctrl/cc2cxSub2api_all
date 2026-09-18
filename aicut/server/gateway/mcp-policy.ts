import { publicMcpEnabled } from './config.ts';

export function publicMcpAllowed(): boolean {
  return publicMcpEnabled();
}
