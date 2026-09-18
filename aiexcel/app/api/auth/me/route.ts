import { NextRequest, NextResponse } from "next/server";
import { readSession, SESSION_COOKIE } from "../../../auth-sso";

export async function GET(request: NextRequest) {
  const user = readSession(request.cookies.get(SESSION_COOKIE)?.value || "");
  return user ? NextResponse.json({ user }) : NextResponse.json({ user: null }, { status: 401 });
}
