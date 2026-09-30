import { NextRequest, NextResponse } from "next/server";
import { createHandoff, verifyTicket, HANDOFF_COOKIE } from "../../../../auth-sso";

function externalUrl(request: NextRequest, path: string) {
  const forwardedProto = request.headers.get("x-forwarded-proto")?.split(",", 1)[0]?.trim();
  const forwardedHost = request.headers.get("x-forwarded-host")?.split(",", 1)[0]?.trim();
  const protocol = forwardedProto || request.nextUrl.protocol.replace(/:$/, "");
  const host = forwardedHost || request.headers.get("host") || request.nextUrl.host;
  return new URL(path, `${protocol}://${host}`);
}

export async function GET(request: NextRequest) {
  const result = verifyTicket(request.nextUrl.searchParams.get("ticket") || "");
  if (!result) return NextResponse.redirect(externalUrl(request, "/?sso_error=invalid"));
  const redirectUrl = externalUrl(request, `/?sso=1&redirect=${encodeURIComponent(result.next)}`);
  const response = NextResponse.redirect(redirectUrl);
  response.cookies.set(HANDOFF_COOKIE, createHandoff(result.user), { httpOnly: true, secure: redirectUrl.protocol === "https:", sameSite: "strict", maxAge: 60, path: "/api/auth/sso/exchange" });
  response.headers.set("Cache-Control", "no-store");
  response.headers.set("Referrer-Policy", "no-referrer");
  return response;
}
