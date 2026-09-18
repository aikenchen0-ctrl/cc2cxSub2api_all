import { NextRequest, NextResponse } from "next/server";
import { createHandoff, verifyTicket, HANDOFF_COOKIE } from "../../../../auth-sso";

export async function GET(request: NextRequest) {
  const result = verifyTicket(request.nextUrl.searchParams.get("ticket") || "");
  if (!result) return NextResponse.redirect(new URL("/?sso_error=invalid", request.url));
  const response = NextResponse.redirect(new URL(`/?sso=1&redirect=${encodeURIComponent(result.next)}`, request.url));
  response.cookies.set(HANDOFF_COOKIE, createHandoff(result.user), { httpOnly: true, secure: request.nextUrl.protocol === "https:", sameSite: "strict", maxAge: 60, path: "/api/auth/sso/exchange" });
  response.headers.set("Cache-Control", "no-store");
  response.headers.set("Referrer-Policy", "no-referrer");
  return response;
}
