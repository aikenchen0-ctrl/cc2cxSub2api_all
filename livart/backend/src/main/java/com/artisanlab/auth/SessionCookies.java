package com.artisanlab.auth;

import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.http.ResponseCookie;
import org.springframework.web.util.WebUtils;

public final class SessionCookies {
    private static final String NAME = "livart_session";

    private SessionCookies() {}

    public static void set(HttpServletRequest request, HttpServletResponse response, String token, long age) {
        response.addHeader("Set-Cookie", ResponseCookie.from(NAME, token).path("/").httpOnly(true)
                .secure(request.isSecure() || "https".equalsIgnoreCase(request.getHeader("X-Forwarded-Proto")))
                .sameSite("Lax").maxAge(age).build().toString());
    }

    public static String read(HttpServletRequest request) {
        var cookie = WebUtils.getCookie(request, NAME);
        return cookie == null ? "" : cookie.getValue();
    }

    public static boolean sameOrigin(HttpServletRequest request) {
        if ("cross-site".equals(request.getHeader("Sec-Fetch-Site"))) return false;
        String origin = request.getHeader("Origin");
        String scheme = request.isSecure() || "https".equalsIgnoreCase(request.getHeader("X-Forwarded-Proto")) ? "https" : "http";
        return origin == null || origin.equals(scheme + "://" + request.getHeader("Host"));
    }
}
