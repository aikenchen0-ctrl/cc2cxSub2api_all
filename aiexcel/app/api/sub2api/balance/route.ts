import { NextRequest, NextResponse } from "next/server";
import { readSession, satelliteHeaders, SESSION_COOKIE } from "../../../auth-sso";

function rechargeUrl(): string | null {
  const raw = process.env.LINK?.trim();
  if (!raw) return null;
  try {
    const url = new URL(raw.includes("://") ? raw : `http://${raw}`);
    if ((url.protocol !== "http:" && url.protocol !== "https:") || url.username || url.password) return null;
    url.pathname = "/purchase";
    url.search = "";
    url.hash = "";
    return url.toString().replace(/\/$/, "");
  } catch {
    return null;
  }
}

export async function GET(request: NextRequest) {
  const user = readSession(request.cookies.get(SESSION_COOKIE)?.value || "");
  if (!user) return NextResponse.json({ error: "请先通过 Sub2API 登录" }, { status: 401, headers: { "Cache-Control": "no-store" } });

  const credential = process.env.SUB2API_APP_CREDENTIAL?.trim();
  const configuredBaseUrl = (process.env.SUB2API_RELAY_BASE_URL || process.env.LINK || "").trim().replace(/\/$/, "");
  if (!credential || !configuredBaseUrl) return NextResponse.json({ error: "Sub2API 未配置" }, { status: 503, headers: { "Cache-Control": "no-store" } });

  const baseUrl = configuredBaseUrl.includes("://") ? configuredBaseUrl : `http://${configuredBaseUrl}`;
  const v1Url = baseUrl.endsWith("/v1") ? baseUrl : `${baseUrl}/v1`;
  try {
    const response = await fetch(`${v1Url}/sub2api/balance`, { headers: satelliteHeaders(user.sub), cache: "no-store" });
    if (!response.ok) return NextResponse.json({ error: "余额暂时不可用" }, { status: response.status === 401 ? 401 : response.status === 503 ? 503 : 502, headers: { "Cache-Control": "no-store" } });
    const data = await response.json() as { balance?: unknown };
    const balance = Number(data.balance);
    if (!Number.isFinite(balance)) throw new Error("Invalid balance response");
    return NextResponse.json({ balance, recharge_url: rechargeUrl() }, { headers: { "Cache-Control": "no-store" } });
  } catch {
    return NextResponse.json({ error: "余额暂时不可用" }, { status: 502, headers: { "Cache-Control": "no-store" } });
  }
}
