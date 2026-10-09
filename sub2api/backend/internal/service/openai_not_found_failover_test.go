package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIManagedNotFoundPreservesRequestScopedGuards(t *testing.T) {
	managed := &OpenAIGatewayService{accountRepo: &modelNotFoundManagedAccountRepo{}}
	for _, tc := range []struct {
		name string
		body string
	}{
		{"cyber policy", `{"error":{"code":"cyber_policy","message":"blocked"}}`},
		{"context limit", `{"error":{"code":"context_length_exceeded","message":"maximum context length exceeded"}}`},
		{"missing continuation", `{"error":{"code":"previous_response_not_found","message":"Previous response not found."}}`},
		{"missing continuation without code", `{"error":{"message":"Previous response not found."}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.False(t, managed.shouldFailoverOpenAIUpstreamResponse(&Account{Platform: PlatformOpenAI}, http.StatusNotFound, "", []byte(tc.body)))
		})
	}
}

func TestOpenAIManagedNotFoundRetriesNextAccountBeforeResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		path        string
		request     string
		healthyBody string
		healthyType string
		forward     func(*OpenAIGatewayService, *gin.Context, *Account, []byte) (*OpenAIForwardResult, error)
	}{
		{
			name:        "native Chat Completions",
			path:        "/v1/chat/completions",
			request:     `{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"}],"stream":false}`,
			healthyBody: `{"id":"chatcmpl_healthy","object":"chat.completion","model":"gpt-5.5","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			healthyType: "application/json",
			forward: func(s *OpenAIGatewayService, c *gin.Context, a *Account, body []byte) (*OpenAIForwardResult, error) {
				return s.forwardAsRawChatCompletions(context.Background(), c, a, body, "")
			},
		},
		{
			name:    "streaming native Chat Completions",
			path:    "/v1/chat/completions",
			request: `{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"}],"stream":true}`,
			healthyBody: "data: {\"id\":\"chatcmpl_healthy\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-5.5\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n" +
				"data: {\"id\":\"chatcmpl_healthy\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-5.5\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n" +
				"data: [DONE]\n\n",
			healthyType: "text/event-stream",
			forward: func(s *OpenAIGatewayService, c *gin.Context, a *Account, body []byte) (*OpenAIForwardResult, error) {
				return s.forwardAsRawChatCompletions(context.Background(), c, a, body, "")
			},
		},
		{
			name:        "native Responses",
			path:        "/v1/responses",
			request:     `{"model":"gpt-5.5","instructions":"test","input":"hello","stream":false}`,
			healthyBody: `{"id":"resp_healthy","object":"response","status":"completed","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
			healthyType: "application/json",
			forward: func(s *OpenAIGatewayService, c *gin.Context, a *Account, body []byte) (*OpenAIForwardResult, error) {
				return s.Forward(context.Background(), c, a, body)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.request)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: http.StatusNotFound, Header: http.Header{"Content-Type": []string{"text/plain"}}, Body: io.NopCloser(strings.NewReader("404 page not found"))},
				{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{tc.healthyType}}, Body: io.NopCloser(strings.NewReader(tc.healthyBody))},
			}}
			svc := &OpenAIGatewayService{
				cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{AllowInsecureHTTP: true}}},
				accountRepo:  &modelNotFoundManagedAccountRepo{},
				httpUpstream: upstream,
			}
			first := &Account{ID: 48, Name: "missing-endpoint", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
				Credentials: map[string]any{"api_key": "sk-first", "base_url": "http://missing.example"},
				Extra:       map[string]any{"openai_responses_supported": true}}
			second := *first
			second.ID = 55
			second.Credentials = map[string]any{"api_key": "sk-second", "base_url": "http://healthy.example"}

			result, err := tc.forward(svc, c, first, body)
			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusNotFound, failoverErr.StatusCode)
			require.True(t, failoverErr.ShouldRetryNextAccount())
			require.False(t, failoverErr.RetryableOnSameAccount)
			require.False(t, c.Writer.Written(), "404 must leave the response open for a healthy candidate")
			require.Empty(t, rec.Body.String())

			result, err = tc.forward(svc, c, &second, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, "healthy.example", upstream.requests[1].URL.Host)
			require.Equal(t, http.StatusOK, rec.Code)
			require.NotContains(t, rec.Body.String(), "404")
			if tc.healthyType == "text/event-stream" {
				require.Contains(t, rec.Header().Get("Content-Type"), tc.healthyType)
				require.Contains(t, rec.Body.String(), `"content":"ok"`)
				require.Contains(t, rec.Body.String(), "data: [DONE]")
			}
		})
	}
}

func TestOpenAIManagedPreviousResponseNotFoundRemainsTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.5","previous_response_id":"resp_expired","input":"hello","stream":false}`)
	c, rec := newOpenAIUpstreamErrorTestContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	upstream := &httpUpstreamRecorder{resp: newOpenAIUpstreamErrorResponse(http.StatusNotFound,
		`{"error":{"type":"invalid_request_error","code":"previous_response_not_found","message":"Previous response not found."}}`)}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{AllowInsecureHTTP: true}}},
		accountRepo:  &modelNotFoundManagedAccountRepo{},
		httpUpstream: upstream,
	}
	account := &Account{ID: 48, Name: "expired-continuation", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "http://upstream.example"},
		Extra:       map[string]any{"openai_responses_supported": true}}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.Nil(t, result)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr, "missing request context must not rotate credentials")
	require.Len(t, upstream.requests, 1)
	require.True(t, c.Writer.Written())
	require.Equal(t, http.StatusBadGateway, rec.Code, "preserve the existing terminal HTTP mapping")
}
