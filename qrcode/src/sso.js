import { createHmac, timingSafeEqual } from "node:crypto";

export const SESSION_COOKIE = "qrcode_session";
const SESSION_TTL_SECONDS = 3 * 86400;
const consumed = new Map();

function secret() {
  return String(process.env.SUB2API_SSO_SECRET || "").trim();
}

function sessionSecret() {
  const value = String(process.env.QR_SESSION_SECRET || process.env.SUB2API_SSO_SECRET || "").trim();
  if (value.length < 32) throw new Error("QRCode session secret is not configured");
  return value;
}

function safeNext(value) {
  if (!value || value.length > 2048 || !value.startsWith("/") || value.startsWith("//") || value.includes("\\") || /[\r\n]/.test(value)) {
    return "/";
  }
  return value;
}

function consume(jti, exp) {
  const now = Date.now();
  for (const [key, expires] of consumed) if (expires <= now) consumed.delete(key);
  if (consumed.has(jti)) return false;
  consumed.set(jti, exp * 1000);
  return true;
}

function pack(value, signingSecret) {
  const encoded = Buffer.from(JSON.stringify(value), "utf8").toString("base64url");
  const mac = createHmac("sha256", signingSecret).update(encoded).digest("base64url");
  return `${encoded}.${mac}`;
}

function unpack(raw, signingSecret) {
  const [encoded, mac] = String(raw || "").split(".");
  if (!encoded || !mac) return null;
  const expected = createHmac("sha256", signingSecret).update(encoded).digest("base64url");
  const left = Buffer.from(mac);
  const right = Buffer.from(expected);
  if (left.length !== right.length || !timingSafeEqual(left, right)) return null;
  try {
    return JSON.parse(Buffer.from(encoded, "base64url").toString("utf8"));
  } catch {
    return null;
  }
}

export function verifyQrcodeSSOTicket(raw, now = Math.floor(Date.now() / 1000)) {
  const shared = secret();
  if (shared.length < 32) throw new Error("QRCode SSO is not configured");
  const parts = String(raw || "").split(".");
  const encoded = parts[0];
  const signature = parts[1];
  if (parts.length !== 2 || !encoded || !signature || raw.length > 16384) throw new Error("invalid SSO ticket");
  const expected = createHmac("sha256", shared).update(encoded).digest("base64url");
  const left = Buffer.from(signature);
  const right = Buffer.from(expected);
  if (left.length !== right.length || !timingSafeEqual(left, right)) throw new Error("invalid SSO ticket signature");
  let ticket;
  try {
    ticket = JSON.parse(Buffer.from(encoded, "base64url").toString("utf8"));
  } catch {
    throw new Error("invalid SSO ticket payload");
  }
  if (ticket.iss !== "sub2api" || ticket.aud !== "qrcode" || !/^[0-9]+$/.test(ticket.sub) || !ticket.jti) {
    throw new Error("invalid SSO ticket claims");
  }
  if (!Number.isSafeInteger(ticket.iat) || !Number.isSafeInteger(ticket.exp) || ticket.exp <= now || ticket.iat > now + 30 || ticket.exp - ticket.iat > 180) {
    throw new Error("expired SSO ticket");
  }
  if (!consume(ticket.jti, ticket.exp)) throw new Error("SSO ticket already used");
  return { userId: ticket.sub, email: ticket.email || "", relayKey: "", next: safeNext(ticket.next) };
}

export function createSessionCookie(userId) {
  const token = pack({ sub: userId, exp: Math.floor(Date.now() / 1000) + SESSION_TTL_SECONDS }, sessionSecret());
  const secure = process.env.QR_SECURE_COOKIE === "1" ? "; Secure" : "";
  return `${SESSION_COOKIE}=${token}; Path=/; HttpOnly; SameSite=Lax; Max-Age=${SESSION_TTL_SECONDS}${secure}`;
}

export function readSessionUserId(request) {
  const cookie = String(request.headers.cookie || "");
  const match = cookie.split(";").map((part) => part.trim()).find((part) => part.startsWith(`${SESSION_COOKIE}=`));
  if (!match) return "";
  const raw = match.slice(SESSION_COOKIE.length + 1);
  const payload = unpack(raw, sessionSecret());
  if (!payload || payload.exp <= Math.floor(Date.now() / 1000) || !payload.sub) return "";
  return String(payload.sub);
}

export function satelliteHeaders(userId) {
  const credential = String(process.env.SUB2API_APP_CREDENTIAL || "").trim();
  const subject = String(userId || "").trim();
  if (!credential || !subject) {
    return {};
  }
  return {
    Authorization: `Bearer ${credential}`,
    "X-Sub2API-On-Behalf-Of": subject,
    "X-Sub2API-Satellite": "qrcode"
  };
}

export function sub2apiV1Base() {
  let base = String(process.env.SUB2API_RELAY_BASE_URL || process.env.LINK || "").trim().replace(/\/+$/, "");
  if (base && !/^https?:\/\//i.test(base)) base = `http://${base}`;
  return base ? (base.endsWith("/v1") ? base : `${base}/v1`) : "";
}

export function sub2apiPurchaseUrl() {
  const raw = String(process.env.LINK || "").trim();
  if (!raw) return null;
  try {
    const url = new URL(/^https?:\/\//i.test(raw) ? raw : `http://${raw}`);
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) return null;
    url.pathname = "/purchase";
    url.search = "";
    url.hash = "";
    return url.toString().replace(/\/$/, "");
  } catch {
    return null;
  }
}

export function qrcodeRequestHeaders(userId, fallbackApiKey = "") {
  const credential = String(process.env.SUB2API_APP_CREDENTIAL || "").trim();
  const managed = Boolean(credential || String(process.env.SUB2API_SSO_SECRET || "").trim());
  const headers = satelliteHeaders(userId);
  if (managed) {
    if (!headers.Authorization || !headers["X-Sub2API-On-Behalf-Of"]) {
      throw new Error("Sub2API satellite credential and session user are required");
    }
    return headers;
  }
  if (String(process.env.NODE_ENV || "").toLowerCase() === "production") {
    throw new Error("QRCode Sub2API satellite is not configured");
  }
  const apiKey = String(fallbackApiKey || "").trim();
  if (apiKey) return { Authorization: `Bearer ${apiKey}` };
  // Unmanaged local test/development mode may intentionally exercise the
  // request pipeline without an upstream credential. Managed deployments are
  // rejected above when SSO is configured but the session headers are absent.
  return {};
}

export function ssoConfigured() {
  return secret().length >= 32;
}

export function requireQrcodeUser(request, response) {
  if (!ssoConfigured()) {
    if (String(process.env.NODE_ENV || "").toLowerCase() === "production") {
      response.status(503).json({ error: "QRCode SSO is not configured" });
      return null;
    }
    return "local";
  }
  try {
    const userId = readSessionUserId(request);
    if (!userId) {
      response.status(401).json({ error: "请先通过 Sub2API 登录" });
      return null;
    }
    return userId;
  } catch {
    response.status(401).json({ error: "请先通过 Sub2API 登录" });
    return null;
  }
}
