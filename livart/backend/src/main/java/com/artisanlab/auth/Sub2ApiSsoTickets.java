package com.artisanlab.auth;

import com.artisanlab.common.ApiException;
import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.springframework.http.HttpStatus;

import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.time.Duration;
import java.time.Instant;
import java.util.Base64;
import java.util.concurrent.ConcurrentHashMap;

public final class Sub2ApiSsoTickets {
    public static final String PROVIDER = "sub2api";
    static final int SECRET_MIN_LENGTH = 32;
    static final int TICKET_MAX_BYTES = 8 * 1024;
    static final Duration MAX_LIFETIME = Duration.ofMinutes(2);
    static final Duration CLOCK_SKEW = Duration.ofSeconds(30);

    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final Base64.Encoder URL_ENCODER = Base64.getUrlEncoder().withoutPadding();
    private static final Base64.Decoder URL_DECODER = Base64.getUrlDecoder();

    private final ConcurrentHashMap<String, Instant> consumed = new ConcurrentHashMap<>();

    @JsonIgnoreProperties(ignoreUnknown = true)
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    public record Payload(
            @JsonProperty("sub") String subject,
            String email,
            String username,
            String displayName,
            String avatarUrl,
            @JsonProperty("iat") long issuedAt,
            @JsonProperty("exp") long expiresAt,
            @JsonProperty("jti") String nonce,
            String next,
            @JsonProperty("rk") String relayKey
    ) {
        public Payload withIdentity(String subject, String nonce) {
            return new Payload(subject, email, username, displayName, avatarUrl, issuedAt, expiresAt, nonce, next, relayKey);
        }
    }

    public String sign(Payload payload, String secret) {
        try {
            String encoded = URL_ENCODER.encodeToString(MAPPER.writeValueAsBytes(payload));
            return encoded + "." + URL_ENCODER.encodeToString(hmac(encoded, secret));
        } catch (Exception exception) {
            throw new IllegalStateException("Unable to sign SSO ticket", exception);
        }
    }

    public Payload verify(String raw, String secret, Instant now) {
        String trimmedSecret = secret == null ? "" : secret.trim();
        if (trimmedSecret.length() < SECRET_MIN_LENGTH) {
            throw invalidTicket("SSO_NOT_CONFIGURED", "sub2api sso secret is not configured");
        }
        if (raw == null || raw.length() > TICKET_MAX_BYTES) {
            throw invalidTicket("INVALID_SSO_TICKET", "invalid sso ticket");
        }
        String[] parts = raw.split("\\.", 3);
        if (parts.length != 2 || parts[0].isEmpty() || parts[1].isEmpty()) {
            throw invalidTicket("INVALID_SSO_TICKET", "invalid sso ticket");
        }

        byte[] expected = hmac(parts[0], trimmedSecret);
        byte[] actual;
        try {
            actual = URL_DECODER.decode(parts[1]);
        } catch (IllegalArgumentException exception) {
            throw invalidTicket("INVALID_SSO_TICKET", "invalid sso ticket signature");
        }
        if (!MessageDigest.isEqual(expected, actual)) {
            throw invalidTicket("INVALID_SSO_TICKET", "invalid sso ticket signature");
        }

        Payload payload;
        try {
            payload = MAPPER.readValue(URL_DECODER.decode(parts[0]), Payload.class);
        } catch (Exception exception) {
            throw invalidTicket("INVALID_SSO_TICKET", "invalid sso ticket payload");
        }
        validatePayload(payload);

        Instant issuedAt = Instant.ofEpochSecond(payload.issuedAt());
        Instant expiresAt = Instant.ofEpochSecond(payload.expiresAt());
        if (payload.issuedAt() <= 0
                || payload.expiresAt() <= payload.issuedAt()
                || payload.expiresAt() - payload.issuedAt() > MAX_LIFETIME.toSeconds()
                || !expiresAt.isAfter(now)
                || issuedAt.isAfter(now.plus(CLOCK_SKEW))
                || expiresAt.isAfter(now.plus(MAX_LIFETIME).plus(CLOCK_SKEW))) {
            throw invalidTicket("EXPIRED_SSO_TICKET", "expired sso ticket");
        }

        String subject = payload.subject().trim();
        String nonce = payload.nonce().trim();
        consumed.entrySet().removeIf(entry -> !now.isBefore(entry.getValue()));
        Instant previous = consumed.putIfAbsent(nonce, expiresAt);
        if (previous != null) {
            throw invalidTicket("SSO_TICKET_USED", "sso ticket already used");
        }
        return payload.withIdentity(subject, nonce);
    }

    public String safeNext(String next) {
        if (next == null) {
            return "/";
        }
        if (next.codePointCount(0, next.length()) > 2048 || containsControl(next)) {
            return "/";
        }
        String trimmed = next.trim();
        if (trimmed.isEmpty() || !trimmed.startsWith("/") || trimmed.startsWith("//") || trimmed.contains("\\")) {
            return "/";
        }
        return trimmed;
    }

    private void validatePayload(Payload payload) {
        if (payload == null || isBlank(payload.subject()) || isBlank(payload.nonce())) {
            throw invalidTicket("INVALID_SSO_TICKET", "invalid sso identity or nonce");
        }
        validateField(payload.subject(), 160);
        validateField(payload.nonce(), 160);
        validateField(nullToEmpty(payload.email()), 160);
        validateField(nullToEmpty(payload.username()), 160);
        validateField(nullToEmpty(payload.displayName()), 160);
        validateField(nullToEmpty(payload.avatarUrl()), 2048);
        validateField(nullToEmpty(payload.next()), 2048);
    }

    private void validateField(String value, int limit) {
        if (value.codePointCount(0, value.length()) > limit || containsControl(value)) {
            throw invalidTicket("INVALID_SSO_TICKET", "invalid sso identity field");
        }
    }

    private byte[] hmac(String encoded, String secret) {
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(secret.getBytes(StandardCharsets.UTF_8), "HmacSHA256"));
            return mac.doFinal(encoded.getBytes(StandardCharsets.UTF_8));
        } catch (Exception exception) {
            throw new IllegalStateException("Unable to verify SSO ticket", exception);
        }
    }

    private static boolean isBlank(String value) {
        return value == null || value.trim().isEmpty();
    }

    private static String nullToEmpty(String value) {
        return value == null ? "" : value;
    }

    private static boolean containsControl(String value) {
        return value.codePoints().anyMatch(Character::isISOControl);
    }

    private static ApiException invalidTicket(String code, String message) {
        return new ApiException(HttpStatus.UNAUTHORIZED, code, message);
    }
}
