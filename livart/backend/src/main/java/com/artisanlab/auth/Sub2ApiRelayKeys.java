package com.artisanlab.auth;

import javax.crypto.Cipher;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.Base64;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;

public final class Sub2ApiRelayKeys {
    private static final ConcurrentHashMap<UUID, String> KEYS = new ConcurrentHashMap<>();

    private Sub2ApiRelayKeys() {
    }

    public static void put(UUID userId, String relayKey) {
        if (userId != null && relayKey != null && !relayKey.isBlank()) {
            KEYS.put(userId, relayKey);
        }
    }

    public static String get(UUID userId) {
        return userId == null ? "" : KEYS.getOrDefault(userId, "");
    }

    public static String decrypt(String ciphertext, String secret) throws Exception {
        byte[] raw = Base64.getUrlDecoder().decode(ciphertext);
        byte[] key = MessageDigest.getInstance("SHA-256").digest(secret.getBytes(StandardCharsets.UTF_8));
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(Cipher.DECRYPT_MODE, new SecretKeySpec(key, "AES"), new GCMParameterSpec(128, raw, 0, 12));
        return new String(cipher.doFinal(raw, 12, raw.length - 12), StandardCharsets.UTF_8);
    }
}
