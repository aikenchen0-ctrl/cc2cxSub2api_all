package com.artisanlab.auth;

import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;
import static org.assertj.core.api.Assertions.assertThat;

class SessionCookiesTest {
    @Test
    void cookieIsPrivateThreeDaysAndOriginIsChecked() {
        var request = new MockHttpServletRequest();
        request.addHeader("Host", "livart.cc2.cx");
        request.addHeader("X-Forwarded-Proto", "https");
        request.addHeader("Origin", "https://livart.cc2.cx");
        assertThat(SessionCookies.sameOrigin(request)).isTrue();
        var response = new MockHttpServletResponse();
        SessionCookies.set(request, response, "private-session", 259200);
        assertThat(response.getHeader("Set-Cookie")).contains("HttpOnly", "Secure", "SameSite=Lax", "Max-Age=259200", "Path=/");
        request.removeHeader("Origin");
        request.addHeader("Origin", "https://another.cc2.cx");
        assertThat(SessionCookies.sameOrigin(request)).isFalse();
        SessionCookies.set(request, response, "", 0);
        assertThat(response.getHeaders("Set-Cookie")).anyMatch(value -> value.contains("Max-Age=0"));
    }
}
