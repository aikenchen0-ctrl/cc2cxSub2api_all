import { createHmac, timingSafeEqual } from "node:crypto";

export type AIExcelUser = { sub: string; email?: string; username?: string; displayName?: string; avatarUrl?: string };

const replayed = new Map<string, number>();
export const HANDOFF_COOKIE = "aiexcel_sso_handoff";
export const SESSION_COOKIE = "aiexcel_session";
export const SESSION_TTL_SECONDS = 3 * 86400;

function secret() {
  const value = process.env.SUB2API_SSO_SECRET?.trim();
  if (!value || value.length < 32) throw new Error("SSO is not configured");
  return value;
}

function sign(encoded: string) {
  return createHmac("sha256", secret()).update(encoded).digest("base64url");
}

function pack(value: unknown) {
  const encoded = Buffer.from(JSON.stringify(value), "utf8").toString("base64url");
  return `${encoded}.${sign(encoded)}`;
}

function unpack<T>(raw: string): T | null {
  const [encoded, mac] = raw.split(".");
  if (!encoded || !mac) return null;
  const expected = sign(encoded);
  if (mac.length !== expected.length || !timingSafeEqual(Buffer.from(mac), Buffer.from(expected))) return null;
  try { return JSON.parse(Buffer.from(encoded, "base64url").toString("utf8")) as T; } catch { return null; }
}

export function satelliteHeaders(userId: string) {
  const credential = process.env.SUB2API_APP_CREDENTIAL?.trim();
  const subject = userId?.trim();
  if (!credential || !subject) return { "Content-Type": "application/json" };
  return {
    Authorization: `Bearer ${credential}`,
    "Content-Type": "application/json",
    "X-Sub2API-On-Behalf-Of": subject,
    "X-Sub2API-Satellite": "aiexcel"
  };
}

export function verifyTicket(raw: string): { user: AIExcelUser; next: string } | null {
  let key: string;
  try { key = secret(); } catch { return null; }
  const [encoded, mac] = raw.split(".");
  if (!encoded || !mac || raw.length > 8192) return null;
  const expected = createHmac("sha256", key).update(encoded).digest("base64url");
  if (mac.length !== expected.length || !timingSafeEqual(Buffer.from(mac), Buffer.from(expected))) return null;
  let payload: Record<string, unknown>;
  try { payload = JSON.parse(Buffer.from(encoded, "base64url").toString("utf8")); } catch { return null; }
  const now = Math.floor(Date.now() / 1000);
  const sub = typeof payload.sub === "string" ? payload.sub.trim() : "";
  const jti = typeof payload.jti === "string" ? payload.jti.trim() : "";
  const iat = Number(payload.iat); const exp = Number(payload.exp);
  if (payload.iss !== "sub2api" || payload.aud !== "aiexcel" || !sub || !jti || !Number.isFinite(iat) || !Number.isFinite(exp) || exp <= now || iat > now + 30 || exp - iat > 120) return null;
  const previous = replayed.get(jti);
  if (previous && previous > now) return null;
  replayed.set(jti, exp);
  for (const [key, expiry] of replayed) if (expiry <= now) replayed.delete(key);
  const next = typeof payload.next === "string" && payload.next.startsWith("/") && !payload.next.startsWith("//") && !payload.next.includes("\\") ? payload.next : "/";
  return { user: { sub, email: typeof payload.email === "string" ? payload.email : undefined, username: typeof payload.username === "string" ? payload.username : undefined, displayName: typeof payload.displayName === "string" ? payload.displayName : undefined, avatarUrl: typeof payload.avatarUrl === "string" ? payload.avatarUrl : undefined }, next };
}

export function createHandoff(user: AIExcelUser) { return pack({ user, exp: Math.floor(Date.now() / 1000) + 60 }); }
export function readHandoff(raw: string) {
  const value = unpack<{ user: AIExcelUser; exp: number }>(raw);
  return value && value.exp > Math.floor(Date.now() / 1000) && value.user?.sub ? value.user : null;
}
export function createSession(user: AIExcelUser) { return pack({ user, exp: Math.floor(Date.now() / 1000) + SESSION_TTL_SECONDS }); }
export function readSession(raw: string) {
  const value = unpack<{ user: AIExcelUser; exp: number }>(raw);
  return value && value.exp > Math.floor(Date.now() / 1000) && value.user?.sub ? value.user : null;
}
