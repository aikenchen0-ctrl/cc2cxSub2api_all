package com.artisanlab.ai;

import com.artisanlab.common.ApiException;
import com.artisanlab.common.GlobalExceptionHandler;
import com.artisanlab.userconfig.UserApiConfigDtos;
import org.junit.jupiter.api.Test;
import org.mockito.Answers;
import org.mockito.MockedStatic;
import org.springframework.ai.openai.api.OpenAiApi;
import org.springframework.http.HttpStatus;
import org.springframework.web.client.ResourceAccessException;
import reactor.core.publisher.Flux;

import java.net.ConnectException;
import java.net.SocketTimeoutException;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicBoolean;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.assertj.core.api.Assertions.catchThrowableOfType;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.mockStatic;
import static org.mockito.Mockito.when;

class SpringAiTextServiceTest {
    @Test
    void usesShortChatPathWhenGatewayBaseUrlAlreadyContainsV1() {
        assertThat(SpringAiTextService.resolveCompletionsPath("https://api.sisct2.xyz/v1"))
                .isEqualTo("/chat/completions");
        assertThat(SpringAiTextService.resolveEmbeddingsPath("https://api.sisct2.xyz/v1"))
                .isEqualTo("/embeddings");
    }

    @Test
    void usesDefaultChatPathWhenBaseUrlDoesNotContainVersionSegment() {
        assertThat(SpringAiTextService.resolveCompletionsPath("https://api.openai.com"))
                .isEqualTo("/v1/chat/completions");
        assertThat(SpringAiTextService.resolveEmbeddingsPath("https://api.openai.com"))
                .isEqualTo("/v1/embeddings");
    }

    @Test
    void extractsAssistantContentFromChatCompletion() {
        OpenAiApi.ChatCompletion completion = new OpenAiApi.ChatCompletion(
                "resp-1",
                List.of(new OpenAiApi.ChatCompletion.Choice(
                        null,
                        0,
                        new OpenAiApi.ChatCompletionMessage("优化后的提示词", OpenAiApi.ChatCompletionMessage.Role.ASSISTANT),
                        null
                )),
                1L,
                "gpt-5.4-mini",
                null,
                null,
                "chat.completion",
                null
        );

        assertThat(SpringAiTextService.extractText(completion)).isEqualTo("优化后的提示词");
    }

    @Test
    void fallsBackToRefusalTextWhenContentIsEmpty() {
        OpenAiApi.ChatCompletion completion = new OpenAiApi.ChatCompletion(
                "resp-2",
                List.of(new OpenAiApi.ChatCompletion.Choice(
                        null,
                        0,
                        new OpenAiApi.ChatCompletionMessage(
                                "",
                                OpenAiApi.ChatCompletionMessage.Role.ASSISTANT,
                                null,
                                null,
                                null,
                                "内容被拒绝",
                                null,
                                null
                        ),
                        null
                )),
                1L,
                "gpt-5.4-mini",
                null,
                null,
                "chat.completion",
                null
        );

        assertThat(SpringAiTextService.extractText(completion)).isEqualTo("内容被拒绝");
    }

    @Test
    void extractsAssistantContentFromStreamChunks() {
        List<OpenAiApi.ChatCompletionChunk> chunks = List.of(
                chunk("优化后"),
                chunk("的提示词")
        );

        assertThat(SpringAiTextService.extractTextFromChunks(chunks)).isEqualTo("优化后的提示词");
    }

    @Test
    void throwsWhenChatCompletionHasNoReadableText() {
        OpenAiApi.ChatCompletion completion = new OpenAiApi.ChatCompletion(
                "resp-3",
                List.of(),
                1L,
                "gpt-5.4-mini",
                null,
                null,
                "chat.completion",
                null
        );

        assertThatThrownBy(() -> SpringAiTextService.extractText(completion))
                .hasMessageContaining("未能从 Spring AI 响应中获取文本");
    }

    @Test
    void explainsNotFoundAsUpstreamEndpointOrModelAvailabilityProblem() {
        String message = SpringAiTextService.upstreamErrorMessage(
                "{\"error\":{\"message\":\"Upstream error: 404\"}}",
                404
        );

        assertThat(message)
                .contains("Spring AI 上游错误")
                .contains("上游接口或模型不存在")
                .contains("Sub2API 上游账号")
                .contains("Responses/Chat Completions");
    }

    @Test
    void blockedStreamTimeoutReturnsHttp504AndCancelsUpstreamSubscription() {
        AtomicBoolean cancelled = new AtomicBoolean();
        Flux<OpenAiApi.ChatCompletionChunk> stream = Flux.<OpenAiApi.ChatCompletionChunk>never()
                .doOnCancel(() -> cancelled.set(true));

        ApiException exception = callWithStream(stream);

        assertTimeoutResponse(exception);
        assertThat(cancelled).isTrue();
    }

    @Test
    void wrappedTimeoutWithoutMessageReturnsHttp504() {
        RuntimeException timeout = new RuntimeException(new TimeoutException());

        assertTimeoutResponse(callWithStream(Flux.error(timeout)));
    }

    @Test
    void socketReadTimeoutReturnsHttp504() {
        ResourceAccessException timeout = new ResourceAccessException(
                "I/O error while reading upstream response", new SocketTimeoutException());

        assertTimeoutResponse(callWithStream(Flux.error(timeout)));
    }

    @Test
    void connectionFailureKeepsHttp502Classification() {
        ResourceAccessException connectionFailure = new ResourceAccessException(
                "I/O error while connecting to upstream", new ConnectException("Connection refused"));

        ApiException exception = callWithStream(Flux.error(connectionFailure));

        assertThat(exception.status()).isEqualTo(HttpStatus.BAD_GATEWAY);
        assertThat(exception.code()).isEqualTo("SPRING_AI_RESOURCE_ACCESS_FAILED");
        assertThat(new GlobalExceptionHandler().handleApiException(exception).getStatusCode())
                .isEqualTo(HttpStatus.BAD_GATEWAY);
    }

    private static ApiException callWithStream(Flux<OpenAiApi.ChatCompletionChunk> stream) {
        OpenAiApi api = mock(OpenAiApi.class);
        OpenAiApi.Builder builder = mock(OpenAiApi.Builder.class, Answers.RETURNS_SELF);
        when(builder.build()).thenReturn(api);
        when(api.chatCompletionStream(any(OpenAiApi.ChatCompletionRequest.class))).thenReturn(stream);
        UserApiConfigDtos.ResolvedConfig config = new UserApiConfigDtos.ResolvedConfig(
                "http://relay.test/v1", "123", "gpt-image-2", "gpt-5.5", "", "", false);

        try (MockedStatic<OpenAiApi> openAiApi = mockStatic(OpenAiApi.class)) {
            openAiApi.when(OpenAiApi::builder).thenReturn(builder);
            return catchThrowableOfType(() -> new SpringAiTextService().completeText(
                    config, "system", "user", Duration.ofMillis(1), "timeout-regression"), ApiException.class);
        }
    }

    private static void assertTimeoutResponse(ApiException exception) {
        assertThat(exception).isNotNull();
        assertThat(exception.status()).isEqualTo(HttpStatus.GATEWAY_TIMEOUT);
        assertThat(exception.code()).isEqualTo("SPRING_AI_TIMEOUT");
        var response = new GlobalExceptionHandler().handleApiException(exception);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.GATEWAY_TIMEOUT);
        assertThat(response.getBody()).isNotNull();
        assertThat(response.getBody().success()).isFalse();
        assertThat(response.getBody().error().code()).isEqualTo("SPRING_AI_TIMEOUT");
    }

    private static OpenAiApi.ChatCompletionChunk chunk(String content) {
        return new OpenAiApi.ChatCompletionChunk(
                "chunk-1",
                List.of(new OpenAiApi.ChatCompletionChunk.ChunkChoice(
                        null,
                        0,
                        new OpenAiApi.ChatCompletionMessage(content, OpenAiApi.ChatCompletionMessage.Role.ASSISTANT),
                        null
                )),
                1L,
                "gpt-5.4-mini",
                null,
                null,
                "chat.completion.chunk",
                null
        );
    }
}
