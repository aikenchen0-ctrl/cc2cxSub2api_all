package com.artisanlab.auth;

import com.artisanlab.common.ApiException;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.http.HttpStatus;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.HexFormat;
import java.util.Locale;
import java.util.UUID;
import java.util.regex.Pattern;

@Service
public class AuthService {
    private static final Pattern USERNAME_PATTERN = Pattern.compile("^[a-z0-9_@.\\-]{3,80}$");
    private static final Pattern SSO_USERNAME_SANITIZE = Pattern.compile("[^a-z0-9_@.\\-]+");

    private final UserMapper userMapper;
    private final UserIdentityMapper identityMapper;
    private final PasswordEncoder passwordEncoder;
    private final JwtService jwtService;

    public AuthService(
            UserMapper userMapper,
            UserIdentityMapper identityMapper,
            PasswordEncoder passwordEncoder,
            JwtService jwtService
    ) {
        this.userMapper = userMapper;
        this.identityMapper = identityMapper;
        this.passwordEncoder = passwordEncoder;
        this.jwtService = jwtService;
    }

    @Transactional
    public AuthDtos.AuthResponse register(AuthDtos.RegisterRequest request) {
        String username = normalizeUsername(request.username());
        String displayName = normalizeDisplayName(request.displayName(), username);

        UserEntity user = new UserEntity();
        user.setId(UUID.randomUUID());
        user.setUsername(username);
        user.setDisplayName(displayName);
        user.setPasswordHash(passwordEncoder.encode(request.password()));

        try {
            userMapper.insertUser(user);
        } catch (DuplicateKeyException exception) {
            throw new ApiException(HttpStatus.CONFLICT, "USERNAME_EXISTS", "用户名已存在");
        }

        return createJwtResponse(userMapper.findById(user.getId()));
    }

    @Transactional
    public AuthDtos.AuthResponse login(AuthDtos.LoginRequest request) {
        String username = normalizeUsername(request.username());
        UserEntity user = userMapper.findByUsername(username);
        if (user == null || !passwordEncoder.matches(request.password(), user.getPasswordHash())) {
            throw new ApiException(HttpStatus.UNAUTHORIZED, "INVALID_CREDENTIALS", "用户名或密码错误");
        }

        return createJwtResponse(user);
    }

    @Transactional(readOnly = true)
    public AuthDtos.AuthUser findUserById(UUID userId) {
        UserEntity user = userMapper.findById(userId);
        return user == null ? null : toUser(user);
    }

    public void logout() {
    }

    @Transactional
    public AuthDtos.AuthResponse completeSso(Sub2ApiSsoTickets.Payload payload) {
        String subject = payload.subject() == null ? "" : payload.subject().trim();
        if (subject.isBlank()) {
            throw new ApiException(HttpStatus.UNAUTHORIZED, "INVALID_SSO_TICKET", "invalid sso identity or nonce");
        }

        UserIdentityEntity identity = identityMapper.findByProviderAndSubject(Sub2ApiSsoTickets.PROVIDER, subject);
        UserEntity user;
        if (identity != null) {
            user = userMapper.findById(identity.getUserId());
            if (user == null) {
                throw new ApiException(HttpStatus.UNAUTHORIZED, "INVALID_SSO_TICKET", "SSO identity is no longer valid");
            }
            identity.setProviderUsername(trimTo(payload.username(), 160));
            identityMapper.updateIdentity(identity);
        } else {
            user = createSsoUser(payload, subject);
        }
        return createJwtResponse(userMapper.findById(user.getId()));
    }

    private UserEntity createSsoUser(Sub2ApiSsoTickets.Payload payload, String subject) {
        String username = allocateSsoUsername(payload.username(), subject);
        UserEntity user = new UserEntity();
        user.setId(UUID.randomUUID());
        user.setUsername(username);
        user.setDisplayName(normalizeDisplayName(payload.displayName(), username));
        user.setPasswordHash(passwordEncoder.encode(UUID.randomUUID().toString()));

        try {
            userMapper.insertUser(user);
        } catch (DuplicateKeyException exception) {
            username = allocateSsoUsername("", subject);
            user.setUsername(username);
            user.setDisplayName(normalizeDisplayName(payload.displayName(), username));
            userMapper.insertUser(user);
        }

        UserIdentityEntity identity = new UserIdentityEntity();
        identity.setId(UUID.randomUUID());
        identity.setUserId(user.getId());
        identity.setProvider(Sub2ApiSsoTickets.PROVIDER);
        identity.setSubject(subject);
        identity.setProviderUsername(trimTo(payload.username(), 160));
        identityMapper.insertIdentity(identity);
        return user;
    }

    private String allocateSsoUsername(String preferred, String subject) {
        String base = SSO_USERNAME_SANITIZE.matcher(preferred == null ? "" : preferred.trim().toLowerCase(Locale.ROOT))
                .replaceAll("_")
                .replaceAll("^[_.\\-]+|[_.\\-]+$", "");
        if (base.length() < 3) {
            base = "sub2api_" + shortSubject(subject);
        }
        if (base.length() > 24) {
            base = base.substring(0, 24);
        }
        if (userMapper.findByUsername(base) == null) {
            return base;
        }
        String suffix = shortSubject(subject);
        String candidate = trimTo(base, Math.max(3, 80 - 1 - suffix.length())) + "_" + suffix;
        if (userMapper.findByUsername(candidate) == null) {
            return candidate;
        }
        return ("sub2api_" + suffix + UUID.randomUUID().toString().replace("-", "")).substring(0, 24);
    }

    private static String shortSubject(String subject) {
        try {
            byte[] digest = MessageDigest.getInstance("SHA-256").digest(subject.getBytes(StandardCharsets.UTF_8));
            return HexFormat.of().formatHex(digest, 0, 4);
        } catch (Exception exception) {
            return UUID.randomUUID().toString().replace("-", "").substring(0, 8);
        }
    }

    private static String trimTo(String value, int maxChars) {
        String trimmed = value == null ? "" : value.trim();
        return trimmed.length() > maxChars ? trimmed.substring(0, maxChars) : trimmed;
    }

    private AuthDtos.AuthResponse createJwtResponse(UserEntity user) {
        return jwtService.issueToken(toUser(user));
    }

    private AuthDtos.AuthUser toUser(UserEntity user) {
        return new AuthDtos.AuthUser(
                user.getId(),
                user.getUsername(),
                user.getDisplayName(),
                user.getCreatedAt()
        );
    }

    private String normalizeUsername(String username) {
        String normalized = username == null ? "" : username.trim().toLowerCase(Locale.ROOT);
        if (!USERNAME_PATTERN.matcher(normalized).matches()) {
            throw new ApiException(HttpStatus.BAD_REQUEST, "INVALID_USERNAME", "用户名只能包含字母、数字、下划线、横线、点或邮箱符号，长度 3-80");
        }
        return normalized;
    }

    private String normalizeDisplayName(String displayName, String username) {
        if (displayName == null || displayName.isBlank()) {
            return username;
        }
        String trimmed = displayName.trim();
        return trimmed.length() > 120 ? trimmed.substring(0, 120) : trimmed;
    }

}
