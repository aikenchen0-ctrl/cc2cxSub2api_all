import { apiGet, apiPost } from "@/services/api/request";

export const AUTH_TOKEN_KEY = "infinite-canvas-auth-token-v1";

export type UserRole = "guest" | "user" | "admin";

export type AuthUser = {
    id: string;
    username: string;
    displayName: string;
    avatarUrl: string;
    role: UserRole;
    credits: number;
    createdAt: string;
    updatedAt: string;
};

export type AuthSession = {
    token: string;
    user: AuthUser;
};

export type AuthPayload = {
    username: string;
    password: string;
};

export async function login(payload: AuthPayload) {
    return apiPost<AuthSession>("/api/auth/login", payload);
}

export async function register(payload: AuthPayload) {
    return apiPost<AuthSession>("/api/auth/register", payload);
}

export async function fetchCurrentUser(token?: string) {
    return apiGet<AuthUser>("/api/auth/me", undefined, token);
}

export async function exchangeSub2APISession(): Promise<AuthSession> {
    const response = await fetch("/api/auth/sso/exchange", { method: "POST", credentials: "same-origin", headers: { "X-Canvas-SSO": "1" }, cache: "no-store" });
    const result = await response.json();
    if (!response.ok || result.code !== 0) throw new Error("Sub2API 登录已失效，请重新跳转");
    return result.data;
}
