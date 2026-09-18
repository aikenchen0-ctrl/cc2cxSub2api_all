package com.artisanlab.auth;

import com.artisanlab.common.ApiException;
import org.junit.jupiter.api.Test;

import java.time.Instant;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class Sub2ApiSsoTicketsTest {
    private static final String SECRET = "s".repeat(32);

    @Test
    void validTicketIssuesPayloadAndRejectsReplay() {
        Sub2ApiSsoTickets tickets = new Sub2ApiSsoTickets();
        Instant now = Instant.ofEpochSecond(1_800_000_000L);
        Sub2ApiSsoTickets.Payload payload = new Sub2ApiSsoTickets.Payload(
                "user-1",
                "user@example.com",
                "canvas-user",
                "Canvas",
                "https://cdn.example/a.png",
                now.getEpochSecond(),
                now.plusSeconds(60).getEpochSecond(),
                "nonce-1",
                "/"
        );
        String raw = tickets.sign(payload, SECRET);

        Sub2ApiSsoTickets.Payload verified = tickets.verify(raw, SECRET, now);
        assertThat(verified.subject()).isEqualTo("user-1");
        assertThat(verified.username()).isEqualTo("canvas-user");
        assertThat(verified.nonce()).isEqualTo("nonce-1");

        assertThatThrownBy(() -> tickets.verify(raw, SECRET, now))
                .isInstanceOf(ApiException.class)
                .hasMessageContaining("already used");
    }

    @Test
    void tamperedTicketFails() {
        Sub2ApiSsoTickets tickets = new Sub2ApiSsoTickets();
        Instant now = Instant.ofEpochSecond(1_800_000_000L);
        String raw = tickets.sign(new Sub2ApiSsoTickets.Payload(
                "user-1", null, "canvas-user", null, null,
                now.getEpochSecond(), now.plusSeconds(60).getEpochSecond(), "nonce-2", "/"
        ), SECRET);
        String tampered = raw + "x";

        assertThatThrownBy(() -> tickets.verify(tampered, SECRET, now))
                .isInstanceOf(ApiException.class)
                .hasMessageContaining("invalid sso ticket");
    }

    @Test
    void expiredTicketFails() {
        Sub2ApiSsoTickets tickets = new Sub2ApiSsoTickets();
        Instant now = Instant.ofEpochSecond(1_800_000_000L);
        String raw = tickets.sign(new Sub2ApiSsoTickets.Payload(
                "user-1", null, "canvas-user", null, null,
                now.minusSeconds(180).getEpochSecond(), now.minusSeconds(60).getEpochSecond(), "nonce-3", "/"
        ), SECRET);

        assertThatThrownBy(() -> tickets.verify(raw, SECRET, now))
                .isInstanceOf(ApiException.class)
                .hasMessageContaining("expired sso ticket");
    }

    @Test
    void wrongSecretFails() {
        Sub2ApiSsoTickets tickets = new Sub2ApiSsoTickets();
        Instant now = Instant.ofEpochSecond(1_800_000_000L);
        String raw = tickets.sign(new Sub2ApiSsoTickets.Payload(
                "user-1", null, "canvas-user", null, null,
                now.getEpochSecond(), now.plusSeconds(60).getEpochSecond(), "nonce-4", "/"
        ), SECRET);

        assertThatThrownBy(() -> tickets.verify(raw, "t".repeat(32), now))
                .isInstanceOf(ApiException.class)
                .hasMessageContaining("invalid sso ticket signature");
    }
}
