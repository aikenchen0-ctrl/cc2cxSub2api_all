package com.artisanlab.userconfig;

import com.artisanlab.common.ApiException;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;

import java.time.OffsetDateTime;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class UserApiConfigServiceTest {
    @Test
    void getConfigDoesNotExposeSavedApiKey() {
        UUID userId = UUID.randomUUID();
        UserApiConfigMapper mapper = mock(UserApiConfigMapper.class);
        when(mapper.findByUserId(userId)).thenReturn(entity(userId, "https://api.example/v1", "sk-secret"));
        UserApiConfigService service = service(mapper, "", "");

        UserApiConfigDtos.Response response = service.getConfig(userId);
        UserApiConfigDtos.ResolvedConfig resolvedConfig = service.getRequiredConfig(userId);

        assertThat(response.apiKey()).isEmpty();
        assertThat(response.hasApiKey()).isTrue();
        assertThat(resolvedConfig.apiKey()).isEqualTo("sk-secret");
    }

    @Test
    void saveConfigKeepsExistingApiKeyWhenRequestLeavesItBlank() {
        UUID userId = UUID.randomUUID();
        UserApiConfigMapper mapper = mock(UserApiConfigMapper.class);
        UserApiConfigEntity existingEntity = entity(userId, "https://api.example/v1", "sk-existing");
        when(mapper.findByUserId(userId)).thenReturn(existingEntity);
        when(mapper.findByUserId(any(UUID.class))).thenReturn(existingEntity);
        UserApiConfigService service = service(mapper, "", "");

        UserApiConfigDtos.Response response = service.saveConfig(
                userId,
                new UserApiConfigDtos.SaveRequest(
                        "https://api.example/v1",
                        "",
                        "gpt-image-2",
                        "gpt-5.4-mini"
                )
        );
        ArgumentCaptor<UserApiConfigEntity> entityCaptor = ArgumentCaptor.forClass(UserApiConfigEntity.class);
        verify(mapper).upsert(entityCaptor.capture());

        assertThat(entityCaptor.getValue().getApiKey()).isEqualTo("sk-existing");
        assertThat(response.apiKey()).isEmpty();
        assertThat(response.hasApiKey()).isTrue();
    }

    @Test
    void getConfigReportsReadyWhenOnlyServerDefaultSub2ApiCredentialsExist() {
        UUID userId = UUID.randomUUID();
        UserApiConfigMapper mapper = mock(UserApiConfigMapper.class);
        when(mapper.findByUserId(userId)).thenReturn(null);
        UserApiConfigService service = service(mapper, "https://api.example/v1", "sk-default");

        UserApiConfigDtos.Response response = service.getConfig(userId);
        assertThatCode(() -> service.getRequiredConfig(userId)).doesNotThrowAnyException();
        UserApiConfigDtos.ResolvedConfig resolvedConfig = service.getRequiredConfig(userId);

        assertThat(response).isNotNull();
        assertThat(response.serverDefault()).isTrue();
        assertThat(response.hasApiKey()).isTrue();
        assertThat(response.apiKey()).isEmpty();
        assertThat(response.baseUrl()).isEqualTo("https://api.example/v1");
        assertThat(resolvedConfig.apiKey()).isEqualTo("sk-default");
        assertThat(resolvedConfig.serverDefault()).isTrue();
    }

    @Test
    void juStyleRelaySuperKeySatisfiesRequiredConfigWithoutUserForm() {
        UUID userId = UUID.randomUUID();
        UserApiConfigMapper mapper = mock(UserApiConfigMapper.class);
        when(mapper.findByUserId(userId)).thenReturn(null);
        UserApiConfigService service = new UserApiConfigService(
                mapper,
                null,
                "",
                "",
                "gpt-image-2",
                "gpt-5.4-mini",
                "http://sub2api:8080/v1",
                "sk-super-held-on-server"
        );

        assertThatCode(() -> service.getRequiredConfig(userId)).doesNotThrowAnyException();
        UserApiConfigDtos.ResolvedConfig resolvedConfig = service.getRequiredConfig(userId);

        assertThat(resolvedConfig.baseUrl()).isEqualTo("http://sub2api:8080/v1");
        assertThat(resolvedConfig.apiKey()).isEqualTo("sk-super-held-on-server");
        assertThat(resolvedConfig.serverDefault()).isTrue();
        assertThat(service.getConfig(userId).apiKey()).isEmpty();
    }

    @Test
    void livartDefaultCredentialsTakePrecedenceOverRelay() {
        UUID userId = UUID.randomUUID();
        UserApiConfigMapper mapper = mock(UserApiConfigMapper.class);
        when(mapper.findByUserId(userId)).thenReturn(null);
        UserApiConfigService service = new UserApiConfigService(
                mapper,
                null,
                "https://livart-default.example/v1",
                "sk-livart-default",
                "gpt-image-2",
                "gpt-5.4-mini",
                "http://sub2api:8080/v1",
                "sk-relay"
        );

        UserApiConfigDtos.ResolvedConfig resolvedConfig = service.getRequiredConfig(userId);

        assertThat(resolvedConfig.baseUrl()).isEqualTo("https://livart-default.example/v1");
        assertThat(resolvedConfig.apiKey()).isEqualTo("sk-livart-default");
    }

    @Test
    void missingUserConfigWithoutServerDefaultStillRequiresPersonalConfig() {
        UUID userId = UUID.randomUUID();
        UserApiConfigMapper mapper = mock(UserApiConfigMapper.class);
        when(mapper.findByUserId(userId)).thenReturn(null);
        UserApiConfigService service = service(mapper, "", "");

        UserApiConfigDtos.Response response = service.getConfig(userId);

        assertThat(response).isNull();
        assertThatThrownBy(() -> service.getRequiredConfig(userId))
                .isInstanceOf(ApiException.class)
                .hasMessageContaining("请先配置自己的中转站 Base URL、API Key、生图模型和对话模型")
                .satisfies(error -> assertThat(((ApiException) error).code()).isEqualTo("USER_API_CONFIG_REQUIRED"));
    }

    @Test
    void exposesServerDefaultConfigForInternalServices() {
        UserApiConfigMapper mapper = mock(UserApiConfigMapper.class);
        UserApiConfigService service = service(mapper, "https://api.example/v1", "sk-default");

        UserApiConfigDtos.ResolvedConfig resolvedConfig = service.getRequiredServerDefaultConfig();

        assertThat(resolvedConfig.baseUrl()).isEqualTo("https://api.example/v1");
        assertThat(resolvedConfig.apiKey()).isEqualTo("sk-default");
        assertThat(resolvedConfig.model()).isEqualTo("gpt-image-2");
        assertThat(resolvedConfig.serverDefault()).isTrue();
    }

    @Test
    void unknownChatModelFallsBackToGpt55() {
        UUID userId = UUID.randomUUID();
        UserApiConfigMapper mapper = mock(UserApiConfigMapper.class);
        UserApiConfigEntity existingEntity = entity(userId, "https://api.example/v1", "sk-existing");
        existingEntity.setChatModel("legacy-chat");
        when(mapper.findByUserId(userId)).thenReturn(existingEntity);
        UserApiConfigService service = new UserApiConfigService(
                mapper,
                null,
                "",
                "",
                "gpt-image-2",
                "gpt-5.5",
                "",
                ""
        );

        UserApiConfigDtos.Response response = service.getConfig(userId);
        UserApiConfigDtos.ResolvedConfig resolvedConfig = service.getRequiredConfig(userId);

        assertThat(response.chatModel()).isEqualTo("gpt-5.5");
        assertThat(resolvedConfig.chatModel()).isEqualTo("gpt-5.5");
    }

    private static UserApiConfigService service(UserApiConfigMapper mapper, String baseUrl, String apiKey) {
        return new UserApiConfigService(
                mapper,
                null,
                baseUrl,
                apiKey,
                "gpt-image-2",
                "gpt-5.4-mini",
                "",
                ""
        );
    }

    private static UserApiConfigEntity entity(UUID userId, String baseUrl, String apiKey) {
        UserApiConfigEntity entity = new UserApiConfigEntity();
        entity.setUserId(userId);
        entity.setBaseUrl(baseUrl);
        entity.setApiKey(apiKey);
        entity.setImageModel("gpt-image-2");
        entity.setChatModel("gpt-5.4-mini");
        entity.setCreatedAt(OffsetDateTime.now());
        entity.setUpdatedAt(OffsetDateTime.now());
        return entity;
    }
}
