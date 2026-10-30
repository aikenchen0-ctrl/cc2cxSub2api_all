//go:build unit

package service

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestApplyMediaSameAccountRetryPolicy(t *testing.T) {
	oauth := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKeyFast := &Account{
		ID:       2,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"fast_failover": true,
		},
	}

	tests := []struct {
		name       string
		account    *Account
		status     int
		headers    http.Header
		preRetry   bool
		allow      bool
		wantRetry  bool
		wantScoped bool
		wantDelay  time.Duration
	}{
		{name: "unauthorized switches immediately", account: oauth, status: http.StatusUnauthorized, allow: true},
		{name: "forbidden switches immediately", account: oauth, status: http.StatusForbidden, allow: true},
		{name: "payment required switches immediately", account: oauth, status: http.StatusPaymentRequired, allow: true},
		{name: "server error retries once", account: oauth, status: http.StatusBadGateway, allow: true, wantRetry: true, wantScoped: true},
		{name: "timeout retries once", account: oauth, status: http.StatusRequestTimeout, allow: true, wantRetry: true, wantScoped: true},
		{name: "rate limit without retry-after switches immediately", account: oauth, status: http.StatusTooManyRequests, allow: true},
		{name: "rate limit with long retry-after switches immediately", account: oauth, status: http.StatusTooManyRequests, headers: http.Header{"Retry-After": []string{"60"}}, allow: true},
		{name: "rate limit with short retry-after waits the header delay", account: oauth, status: http.StatusTooManyRequests, headers: http.Header{"Retry-After": []string{"1"}}, allow: true, wantRetry: true, wantDelay: time.Second},
		{name: "capacity rate limit already classified as retryable keeps replay", account: oauth, status: http.StatusTooManyRequests, preRetry: true, allow: true, wantRetry: true},
		{name: "client error does not retry", account: oauth, status: http.StatusBadRequest, allow: true},
		{name: "fast failover keeps immediate switch", account: apiKeyFast, status: http.StatusTooManyRequests, allow: true},
		{name: "disable result blocks same-account replay", account: oauth, status: http.StatusBadGateway, allow: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &UpstreamFailoverError{
				StatusCode:             tt.status,
				ResponseHeaders:        tt.headers,
				RetryableOnSameAccount: tt.preRetry,
			}
			applyMediaSameAccountRetryPolicy(tt.account, err, tt.allow)
			require.Equal(t, tt.wantRetry, err.RetryableOnSameAccount)
			require.Equal(t, tt.wantScoped, err.RequestScopedTransient)
			if tt.wantRetry {
				require.Equal(t, mediaSameAccountRetryMax, err.SameAccountRetryMax)
				wantDelay := tt.wantDelay
				if wantDelay <= 0 {
					wantDelay = mediaSameAccountRetryDelay
				}
				require.Equal(t, wantDelay, err.SameAccountRetryDelay)
			}
		})
	}
}

func TestNewMediaEmptyOutputFailoverErrorUsesBoundedSameAccountRetry(t *testing.T) {
	err := newMediaEmptyOutputFailoverError(http.StatusBadGateway, []byte(`{"error":"empty"}`))
	require.True(t, err.RetryableOnSameAccount)
	require.True(t, err.RequestScopedTransient)
	require.Equal(t, mediaSameAccountRetryMax, err.SameAccountRetryMax)
	require.Equal(t, mediaSameAccountRetryDelay, err.SameAccountRetryDelay)
	require.Equal(t, mediaSameAccountRetryMax, 1)
	require.Equal(t, 500*time.Millisecond, mediaSameAccountRetryDelay)
}

func TestGrokMediaEndpointAsyncVideoCreate(t *testing.T) {
	for _, endpoint := range []GrokMediaEndpoint{
		GrokMediaEndpointVideosGenerations,
		GrokMediaEndpointVideosEdits,
		GrokMediaEndpointVideosExtensions,
	} {
		require.True(t, endpoint.IsAsyncVideoCreateRequest(), endpoint)
	}
	for _, endpoint := range []GrokMediaEndpoint{
		GrokMediaEndpointImagesGenerations,
		GrokMediaEndpointImagesEdits,
		GrokMediaEndpointVideoStatus,
		GrokMediaEndpointVideoContent,
	} {
		require.False(t, endpoint.IsAsyncVideoCreateRequest(), endpoint)
	}
}
