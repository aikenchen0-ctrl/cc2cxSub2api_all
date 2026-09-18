import { NextRequest, NextResponse } from "next/server";
import { createSession, readHandoff, HANDOFF_COOKIE, SESSION_COOKIE } from "../../../../auth-sso";

export async function POST(request: NextRequest) {
  if (request.headers.get("x-aiexcel-sso") !== "1" || request.headers.get("sec-fetch-site") === "cross-site") return NextResponse.json({ error: "SSO exchange rejected" }, { status: 403 });
  const response = NextResponse.json({ ok: true });
  const raw = request.cookies.get(HANDOFF_COOKIE)?.value || "";
  const user = readHandoff(raw);
  response.cookies.set(HANDOFF_COOKIE, "", { httpOnly: true, maxAge: 0, path: "/api/auth/sso/exchange" });
  if (!user) return NextResponse.json({ error: "SSO handoff expired" }, { status: 401 });
  response.cookies.set(SESSION_COOKIE, createSession(user), { httpOnly: true, secure: request.nextUrl.protocol === "https:", sameSite: "lax", maxAge: 7 * 86400, path: "/" });
  return response;
}
