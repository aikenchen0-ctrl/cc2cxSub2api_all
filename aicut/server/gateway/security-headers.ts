import type { Plugin } from 'vite';
import { gatewayEnabled } from './config.ts';

/** Public-gateway browser hardening. Local single-user OCC keeps its existing headers. */
export function applyGatewaySecurityHeaders(res: { setHeader(name: string, value: string): void }): void {
  if (!gatewayEnabled()) return;
  res.setHeader('X-Content-Type-Options', 'nosniff');
  res.setHeader('X-Frame-Options', 'DENY');
  res.setHeader('Referrer-Policy', 'strict-origin-when-cross-origin');
  res.setHeader('X-DNS-Prefetch-Control', 'off');
  res.setHeader('Permissions-Policy', 'camera=(), microphone=(), geolocation=()');
}

export function securityHeadersPlugin(): Plugin {
  return {
    name: 'openchatcut-gateway-security-headers',
    configureServer(server) {
      server.middlewares.use((_req, res, next) => {
        applyGatewaySecurityHeaders(res);
        next();
      });
    },
    configurePreviewServer(server) {
      server.middlewares.use((_req, res, next) => {
        applyGatewaySecurityHeaders(res);
        next();
      });
    },
  };
}
