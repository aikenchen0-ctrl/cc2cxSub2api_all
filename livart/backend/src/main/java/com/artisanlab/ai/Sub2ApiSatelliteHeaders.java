package com.artisanlab.ai;

import com.artisanlab.common.ApiException;
import org.springframework.http.HttpStatus;

import java.net.http.HttpRequest;
import java.util.function.BiConsumer;

/**
 * Applies the server-side authentication contract for Livart's Sub2API relay
 * calls. In managed mode the value kept in the resolved config is the SSO
 * subject, never an API key; the application credential is the only bearer
 * credential sent upstream.
 */
public final class Sub2ApiSatelliteHeaders {
    private Sub2ApiSatelliteHeaders() {
    }

    static void apply(HttpRequest.Builder builder, String apiKeyOrSubject) {
        if (builder == null) {
            throw new ApiException(HttpStatus.INTERNAL_SERVER_ERROR, "SUB2API_REQUEST_INVALID", "无法创建 Sub2API 请求");
        }
        String credential = trim(System.getenv("SUB2API_APP_CREDENTIAL"));
        applyIdentityHeaders(builder::header, apiKeyOrSubject);
        if (!credential.isBlank()) {
            builder.header("Authorization", "Bearer " + credential);
            return;
        }
        String value = trim(apiKeyOrSubject);
        if (value.isBlank()) {
            throw new ApiException(
                    HttpStatus.UNAUTHORIZED,
                    "SUB2API_API_KEY_REQUIRED",
                    "未配置 Sub2API API Key"
            );
        }
        builder.header("Authorization", "Bearer " + value);
    }

    /**
     * Adds only the satellite identity headers. Authorization stays with the
     * OpenAI-compatible client so RestClient and WebClient share the same
     * Sub2API contract.
     */
    public static void applyIdentityHeaders(BiConsumer<String, String> headerSetter, String apiKeyOrSubject) {
        if (headerSetter == null) {
            throw new ApiException(HttpStatus.INTERNAL_SERVER_ERROR, "SUB2API_REQUEST_INVALID", "无法创建 Sub2API 请求");
        }
        String credential = trim(System.getenv("SUB2API_APP_CREDENTIAL"));
        boolean managed = !credential.isBlank() || !trim(System.getenv("SUB2API_SSO_SECRET")).isBlank();
        String value = trim(apiKeyOrSubject);
        if (managed && credential.isBlank()) {
            throw new ApiException(
                    HttpStatus.SERVICE_UNAVAILABLE,
                    "SUB2API_CREDENTIAL_UNAVAILABLE",
                    "Sub2API satellite credential is unavailable"
            );
        }
        if (!credential.isBlank()) {
            if (!isNumericSubject(value)) {
                throw new ApiException(
                        HttpStatus.UNAUTHORIZED,
                        "SUB2API_SESSION_REQUIRED",
                        "当前会话未提供 Sub2API 用户身份"
                );
            }
            headerSetter.accept("X-Sub2API-On-Behalf-Of", value);
            headerSetter.accept("X-Sub2API-Satellite", "livart");
        }
    }

    private static boolean isNumericSubject(String value) {
        return !value.isBlank() && value.chars().allMatch(Character::isDigit);
    }

    private static String trim(String value) {
        return value == null ? "" : value.trim();
    }
}
