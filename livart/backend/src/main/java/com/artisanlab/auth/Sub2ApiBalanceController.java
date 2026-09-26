package com.artisanlab.auth;

import com.artisanlab.ai.Sub2ApiSatelliteHeaders;
import com.artisanlab.common.ApiException;
import com.artisanlab.common.ApiResponse;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.UUID;

@RestController
@RequestMapping("/api/sub2api")
public class Sub2ApiBalanceController {
    private final AuthContext authContext;
    private final UserIdentityMapper identityMapper;
    private final ObjectMapper objectMapper;
    private final String relayBaseUrl;
    private final String publicLink;
    private final HttpClient httpClient;

    public Sub2ApiBalanceController(
            AuthContext authContext,
            UserIdentityMapper identityMapper,
            ObjectMapper objectMapper,
            @Value("${SUB2API_RELAY_BASE_URL:${LINK:}}") String relayBaseUrl,
            @Value("${LINK:}") String publicLink
    ) {
        this.authContext = authContext;
        this.identityMapper = identityMapper;
        this.objectMapper = objectMapper;
        this.relayBaseUrl = relayBaseUrl;
        this.publicLink = publicLink;
        this.httpClient = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(5)).build();
    }

    @GetMapping("/balance")
    public ResponseEntity<ApiResponse<BalanceResponse>> balance(HttpServletResponse response) {
        response.setHeader("Cache-Control", "no-store");
        UUID userId = authContext.requireUserId();
        UserIdentityEntity identity = identityMapper.findByUserIdAndProvider(userId, Sub2ApiSsoTickets.PROVIDER);
        if (identity == null || identity.getSubject() == null || identity.getSubject().isBlank()) {
            throw new ApiException(HttpStatus.UNAUTHORIZED, "SUB2API_SESSION_REQUIRED", "当前会话未关联 Sub2API 账户");
        }
        String credential = System.getenv("SUB2API_APP_CREDENTIAL");
        if (credential == null || credential.isBlank()) {
            throw new ApiException(HttpStatus.SERVICE_UNAVAILABLE, "SUB2API_CREDENTIAL_UNAVAILABLE", "Sub2API satellite credential is unavailable");
        }

        URI endpoint = relayEndpoint(relayBaseUrl);
        HttpRequest.Builder request = HttpRequest.newBuilder(endpoint)
                .timeout(Duration.ofSeconds(12))
                .header("Accept", "application/json")
                .GET();
        Sub2ApiSatelliteHeaders.applyIdentityHeaders(request::header, identity.getSubject());
        request.header("Authorization", "Bearer " + credential);

        JsonNode upstreamBody;
        try {
            HttpResponse<String> upstream = httpClient.send(request.build(), HttpResponse.BodyHandlers.ofString());
            if (upstream.statusCode() != 200) {
                HttpStatus status = upstream.statusCode() == 401
                        ? HttpStatus.UNAUTHORIZED
                        : upstream.statusCode() == 503 ? HttpStatus.SERVICE_UNAVAILABLE : HttpStatus.BAD_GATEWAY;
                throw new ApiException(status, "SUB2API_BALANCE_UNAVAILABLE", "读取 Sub2API 余额失败");
            }
            upstreamBody = objectMapper.readTree(upstream.body());
        } catch (ApiException exception) {
            throw exception;
        } catch (Exception exception) {
            throw new ApiException(HttpStatus.BAD_GATEWAY, "SUB2API_BALANCE_UNAVAILABLE", "读取 Sub2API 余额失败");
        }

        JsonNode rawBalance = upstreamBody == null ? null : upstreamBody.get("balance");
        if (rawBalance == null || !rawBalance.isNumber() || !Double.isFinite(rawBalance.doubleValue())) {
            throw new ApiException(HttpStatus.BAD_GATEWAY, "SUB2API_BALANCE_INVALID", "Sub2API 余额响应无效");
        }
        return ResponseEntity.ok(ApiResponse.ok(new BalanceResponse(rawBalance.doubleValue(), purchaseUrl(publicLink))));
    }

    private static URI relayEndpoint(String raw) {
        String value = raw == null ? "" : raw.trim();
        if (value.isBlank()) {
            throw new ApiException(HttpStatus.SERVICE_UNAVAILABLE, "SUB2API_RELAY_UNAVAILABLE", "Sub2API relay URL is unavailable");
        }
        if (!value.contains("://")) value = "http://" + value;
        try {
            URI base = URI.create(value);
            String path = base.getPath() == null ? "" : base.getPath().replaceAll("/$", "");
            if (( !"http".equalsIgnoreCase(base.getScheme()) && !"https".equalsIgnoreCase(base.getScheme()))
                    || base.getHost() == null || base.getUserInfo() != null || base.getQuery() != null
                    || base.getFragment() != null || (!path.isEmpty() && !"/v1".equals(path))) {
                throw new IllegalArgumentException("invalid relay URL");
            }
            return URI.create(base.getScheme() + "://" + base.getRawAuthority() + "/v1/sub2api/balance");
        } catch (RuntimeException exception) {
            throw new ApiException(HttpStatus.SERVICE_UNAVAILABLE, "SUB2API_RELAY_UNAVAILABLE", "Sub2API relay URL is invalid");
        }
    }

    private static String purchaseUrl(String raw) {
        String value = raw == null ? "" : raw.trim();
        if (value.isBlank()) return "";
        if (!value.contains("://")) value = "http://" + value;
        try {
            URI base = URI.create(value);
            if (( !"http".equalsIgnoreCase(base.getScheme()) && !"https".equalsIgnoreCase(base.getScheme()))
                    || base.getHost() == null || base.getUserInfo() != null || base.getQuery() != null || base.getFragment() != null) {
                return "";
            }
            return base.getScheme() + "://" + base.getRawAuthority() + "/purchase";
        } catch (RuntimeException exception) {
            return "";
        }
    }

    public record BalanceResponse(double balance, @JsonProperty("recharge_url") String rechargeUrl) {}
}
