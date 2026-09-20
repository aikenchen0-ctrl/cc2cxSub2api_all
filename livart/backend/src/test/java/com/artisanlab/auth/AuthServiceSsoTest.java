package com.artisanlab.auth;

import org.junit.jupiter.api.Test;
import org.springframework.security.crypto.password.PasswordEncoder;

import java.time.OffsetDateTime;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicReference;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.doAnswer;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class AuthServiceSsoTest {
    @Test
    void completeSsoCreatesThenReusesIdentity() {
        UserMapper userMapper = mock(UserMapper.class);
        UserIdentityMapper identityMapper = mock(UserIdentityMapper.class);
        PasswordEncoder passwordEncoder = mock(PasswordEncoder.class);
        JwtService jwtService = mock(JwtService.class);

        AtomicReference<UserEntity> storedUser = new AtomicReference<>();
        AtomicReference<UserIdentityEntity> storedIdentity = new AtomicReference<>();

        when(passwordEncoder.encode(any())).thenReturn("hashed-password");
        when(userMapper.findByUsername(any())).thenReturn(null);
        doAnswer(invocation -> {
            storedUser.set(invocation.getArgument(0));
            return null;
        }).when(userMapper).insertUser(any());
        when(userMapper.findById(any())).thenAnswer(invocation -> storedUser.get());
        when(identityMapper.findByProviderAndSubject(any(), any())).thenAnswer(invocation -> storedIdentity.get());
        doAnswer(invocation -> {
            storedIdentity.set(invocation.getArgument(0));
            return null;
        }).when(identityMapper).insertIdentity(any());
        when(jwtService.issueToken(any())).thenAnswer(invocation -> {
            AuthDtos.AuthUser user = invocation.getArgument(0);
            return new AuthDtos.AuthResponse(user, "livart-session-token", OffsetDateTime.now().plusDays(1));
        });

        AuthService service = new AuthService(userMapper, identityMapper, passwordEncoder, jwtService);
        Sub2ApiSsoTickets.Payload payload = new Sub2ApiSsoTickets.Payload(
                " subject-1 ",
                "user@example.com",
                "canvas-user",
                "SSO user",
                null,
                1_800_000_000L,
                1_800_000_060L,
                "nonce-1",
                "/",
                null
        );

        AuthDtos.AuthResponse first = service.completeSso(payload);
        AuthDtos.AuthResponse second = service.completeSso(payload);

        assertThat(first.token()).isEqualTo("livart-session-token");
        assertThat(first.user().username()).isEqualTo("canvas-user");
        assertThat(first.user().displayName()).isEqualTo("SSO user");
        assertThat(second.user().id()).isEqualTo(first.user().id());
        assertThat(storedIdentity.get().getProvider()).isEqualTo(Sub2ApiSsoTickets.PROVIDER);
        assertThat(storedIdentity.get().getSubject()).isEqualTo("subject-1");
        assertThat(storedUser.get().getPasswordHash()).isEqualTo("hashed-password");
        assertThat(payload.username()).isEqualTo("canvas-user");
    }
}
