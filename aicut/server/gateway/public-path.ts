/** Anonymous paths when the multi-user gateway is on. Everything else needs a session. */

const AUTH_PATHS = new Set([
  '/api/auth/login',
  '/api/auth/logout',
  '/api/auth/me',
  '/api/sub2api/balance',
  '/api/auth/models',
  '/api/auth/register',
  '/api/auth/config',
]);

const PROTECTED_EXACT = new Set([
  '/llm',
  '/e2b',
  '/generate',
  '/export',
  '/upload',
  '/render-still',
  '/render-clip',
  '/media/uploads',
]);

const PROTECTED_PREFIXES = [
  '/api/',
  '/llm/',
  '/e2b/',
  '/generate/',
  '/export/',
  '/upload/',
  '/render-still/',
  '/render-clip/',
  '/media/uploads/',
];

export function isAnonymousGatewayPath(pathname: string): boolean {
  return AUTH_PATHS.has(pathname) || pathname === '/api/external-mcp/mcp';
}

export function isProtectedGatewayPath(pathname: string): boolean {
  if (isAnonymousGatewayPath(pathname)) return false;
  if (PROTECTED_EXACT.has(pathname)) return true;
  return PROTECTED_PREFIXES.some((prefix) => pathname.startsWith(prefix));
}
