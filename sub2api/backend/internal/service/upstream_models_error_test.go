package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func openAIAPIKeyModelSyncAccount() *Account {
	return &Account{
		ID:       1234,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "upstream-key",
			"base_url": "https://relay.example/v1",
		},
	}
}

func modelListResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// Scenario: 上游返回 403 欠费时，错误必须带上 HTTP 状态、商家错误码和原文说明，
// 否则管理员只能看到 "Failed to sync upstream models" 无法判断原因。
func TestFetchUpstreamModelListSurfacesFlatProviderErrorDetail(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: modelListResponse(
		http.StatusForbidden,
		`{"code":"INSUFFICIENT_BALANCE","message":"Insufficient account balance"}`,
	)}
	svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), openAIAPIKeyModelSyncAccount())
	require.Error(t, err)

	var syncErr *UpstreamModelSyncError
	require.ErrorAs(t, err, &syncErr)
	require.Equal(t, UpstreamModelSyncErrorUpstream, syncErr.Kind)
	require.Equal(t, http.StatusForbidden, syncErr.StatusCode)
	require.Equal(t, "INSUFFICIENT_BALANCE", syncErr.Code)

	message := syncErr.SafeMessage()
	require.Contains(t, message, "HTTP 403")
	require.Contains(t, message, "INSUFFICIENT_BALANCE")
	require.Contains(t, message, "Insufficient account balance")
	require.Equal(t, "HTTP 403 INSUFFICIENT_BALANCE (Insufficient account balance)", syncErr.SafeDetail())
	require.True(t, upstreamModelListInsufficientBalance(err))
}

// Scenario: 多数中转商把错误放在 error 对象里，需要能取到嵌套的 code/message。
func TestFetchUpstreamModelListParsesNestedProviderErrorDetail(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: modelListResponse(
		http.StatusUnauthorized,
		`{"error":{"code":"invalid_api_key","message":"Incorrect API key provided"}}`,
	)}
	svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), openAIAPIKeyModelSyncAccount())
	require.Error(t, err)

	var syncErr *UpstreamModelSyncError
	require.ErrorAs(t, err, &syncErr)
	require.Equal(t, http.StatusUnauthorized, syncErr.StatusCode)
	require.Equal(t, "invalid_api_key", syncErr.Code)
	require.Equal(t, "Incorrect API key provided", syncErr.UpstreamMessage)
	require.Contains(t, syncErr.SafeMessage(), "HTTP 401 invalid_api_key (Incorrect API key provided)")
}

// Scenario: 上游返回 HTML/纯文本时也要保留原文，并折叠成单行。
func TestFetchUpstreamModelListFallsBackToRawBodyForNonJSONError(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: modelListResponse(
		http.StatusBadGateway,
		"<html>\n  <body>upstream gateway down</body>\n</html>\n",
	)}
	svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), openAIAPIKeyModelSyncAccount())
	require.Error(t, err)

	var syncErr *UpstreamModelSyncError
	require.ErrorAs(t, err, &syncErr)
	require.Empty(t, syncErr.Code)
	require.Contains(t, syncErr.UpstreamMessage, "upstream gateway down")

	message := syncErr.SafeMessage()
	require.Contains(t, message, "HTTP 502")
	require.NotContains(t, message, "\n")
}

// Scenario: 上游回传超长错误体时必须截断，避免污染后台响应和日志。
func TestUpstreamModelSyncErrorSafeMessageBoundsProviderDetail(t *testing.T) {
	t.Parallel()

	syncErr := &UpstreamModelSyncError{
		Kind:            UpstreamModelSyncErrorUpstream,
		Message:         "Upstream model list request failed with HTTP 400",
		StatusCode:      http.StatusBadRequest,
		UpstreamMessage: strings.Repeat("overflow ", 200),
	}

	detail := syncErr.SafeDetail()
	require.Less(t, len([]rune(detail)), 240)
	require.True(t, strings.HasPrefix(detail, "HTTP 400 ("))
	require.Contains(t, detail, "\u2026")
	require.True(t, strings.HasSuffix(detail, ")"))
	require.False(t, strings.ContainsAny(detail, "\r\n"))
}

// Scenario: 没有上游细节时保持原有安全文案不变。
func TestUpstreamModelSyncErrorSafeMessageWithoutProviderDetail(t *testing.T) {
	t.Parallel()

	syncErr := &UpstreamModelSyncError{
		Kind:    UpstreamModelSyncErrorConfiguration,
		Message: "Account test service is not configured",
	}
	require.Equal(t, "Account test service is not configured", syncErr.SafeMessage())
	require.Empty(t, syncErr.SafeDetail())

	var nilErr *UpstreamModelSyncError
	require.Equal(t, "Failed to sync upstream models", nilErr.SafeMessage())
	require.Empty(t, nilErr.SafeDetail())
}

// Scenario: 嵌套错误码也要能命中“欠费时退回配置模型做能力同步”的兜底判定。
func TestUpstreamModelListInsufficientBalanceDetectsNestedPayload(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: modelListResponse(
		http.StatusForbidden,
		`{"error":{"code":"insufficient_balance","message":"no balance"}}`,
	)}
	svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}

	_, err := svc.FetchUpstreamSupportedModels(context.Background(), openAIAPIKeyModelSyncAccount())
	require.Error(t, err)
	require.True(t, upstreamModelListInsufficientBalance(err))
}

// Scenario: 上游 /models 不可用而退回配置映射时，必须显式告警，
// 否则后台会把「配置里的模型」当成「上游真实模型列表」。
func TestSyncUpstreamModelCatalogWarnsWhenLiveModelListUnavailable(t *testing.T) {
	t.Parallel()

	upstream := &httpUpstreamRecorder{resp: modelListResponse(
		http.StatusForbidden,
		`{"code":"INSUFFICIENT_BALANCE","message":"Insufficient account balance"}`,
	)}
	svc := &AccountTestService{httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	account := &Account{
		ID:       62,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":       "upstream-key",
			"base_url":      "https://relay.example/v1",
			"model_mapping": map[string]any{"gpt-5": "gpt-5"},
		},
	}

	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5"}, catalog.Models)
	warnIdx := -1
	for i, warning := range catalog.Warnings {
		if warning.Code == UpstreamModelListUnavailableCode {
			warnIdx = i
		}
	}
	require.NotEqual(t, -1, warnIdx, "expected upstream_model_list_unavailable warning")
	require.Contains(t, catalog.Warnings[warnIdx].Message, "HTTP 403 INSUFFICIENT_BALANCE")
	require.Contains(t, catalog.Warnings[warnIdx].Message, "configured model mapping")
}
