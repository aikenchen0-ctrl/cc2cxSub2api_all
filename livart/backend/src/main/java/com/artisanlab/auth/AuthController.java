package com.artisanlab.auth;

import com.artisanlab.common.ApiResponse;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.servlet.http.HttpServletResponse;
import jakarta.validation.Valid;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.time.Instant;

@RestController
@RequestMapping("/api/auth")
public class AuthController {
    private static final String AUTH_SESSION_STORAGE_KEY = "livart_auth_session";

    private final AuthService authService;
    private final AuthContext authContext;
    private final Sub2ApiSsoTickets ssoTickets;
    private final ObjectMapper objectMapper;
    private final String ssoSecret;

    public AuthController(
            AuthService authService,
            AuthContext authContext,
            ObjectMapper objectMapper,
            @Value("${SUB2API_SSO_SECRET:}") String ssoSecret
    ) {
        this.authService = authService;
        this.authContext = authContext;
        this.ssoTickets = new Sub2ApiSsoTickets();
        this.objectMapper = objectMapper;
        this.ssoSecret = ssoSecret == null ? "" : ssoSecret;
    }

    @PostMapping("/register")
    public ApiResponse<AuthDtos.AuthResponse> register(@Valid @RequestBody AuthDtos.RegisterRequest request) {
        return ApiResponse.ok(authService.register(request));
    }

    @PostMapping("/login")
    public ApiResponse<AuthDtos.AuthResponse> login(@Valid @RequestBody AuthDtos.LoginRequest request) {
        return ApiResponse.ok(authService.login(request));
    }

    @GetMapping("/me")
    public ApiResponse<AuthDtos.AuthUser> me() {
        authContext.requireUserId();
        return ApiResponse.ok(authContext.currentUser());
    }

    @PostMapping("/logout")
    public ApiResponse<Void> logout() {
        authService.logout();
        return ApiResponse.ok(null);
    }

    @GetMapping(value = "/sso/callback", produces = MediaType.TEXT_HTML_VALUE)
    public void ssoCallback(
            @RequestParam(value = "ticket", required = false) String ticket,
            HttpServletResponse response
    ) throws IOException {
        response.setHeader("Cache-Control", "no-store");
        response.setHeader("Referrer-Policy", "no-referrer");
        try {
            Sub2ApiSsoTickets.Payload payload = ssoTickets.verify(ticket, ssoSecret, Instant.now());
            AuthDtos.AuthResponse session = authService.completeSso(payload);
            writeSessionBootstrap(response, session, ssoTickets.safeNext(payload.next()));
        } catch (Exception exception) {
            System.err.println("[sso] callback failed: " + exception.getMessage());
            response.sendRedirect("/?sso_error=1");
        }
    }

    private void writeSessionBootstrap(
            HttpServletResponse response,
            AuthDtos.AuthResponse session,
            String next
    ) throws IOException {
        String sessionJson = objectMapper.writeValueAsString(session).replace("</", "<\\/");
        String nextJson = objectMapper.writeValueAsString(next);
        String html = "<!DOCTYPE html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">"
                + "<meta name=\"referrer\" content=\"no-referrer\"><title>livart</title></head><body><script>"
                + "try { localStorage.setItem('" + AUTH_SESSION_STORAGE_KEY + "', JSON.stringify("
                + sessionJson + ")); } catch (e) {}"
                + "location.replace(" + nextJson + ");"
                + "</script></body></html>";
        response.setStatus(HttpServletResponse.SC_OK);
        response.setCharacterEncoding(StandardCharsets.UTF_8.name());
        response.setContentType(MediaType.TEXT_HTML_VALUE);
        response.getWriter().write(html);
    }
}
