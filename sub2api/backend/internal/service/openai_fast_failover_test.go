//go:build unit

package service

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func fastFailoverPoolAccount() *Account {
	return &Account{
		ID: 748, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"pool_mode": true, "pool_mode_retry_count": float64(5), "fast_failover": true,
		},
	}
}

func TestFastFailoverAccountPolicyIsOptInAndAPIKeyOnly(t *testing.T) {
	account := fastFailoverPoolAccount()
	for _, status := range []int{401, 402, 403, 404, 405, 408, 429, 500, 502, 503, 504, 529} {
		require.True(t, account.ShouldFailoverImmediately(status), "status=%d", status)
	}
	for _, status := range []int{200, 400, 413, 422, 600} {
		require.False(t, account.ShouldFailoverImmediately(status), "status=%d", status)
	}
	account.Credentials["fast_failover"] = false
	require.False(t, account.ShouldFailoverImmediately(429))
	account.Credentials["fast_failover"] = true
	account.Type = AccountTypeOAuth
	require.False(t, account.ShouldFailoverImmediately(429))
	account.Type, account.Platform = AccountTypeAPIKey, PlatformGrok
	require.False(t, account.ShouldFailoverImmediately(429))
	var missing *Account
	require.False(t, missing.ShouldFailoverImmediately(429))
}

func TestFastFailoverNormalizesSameAccountFlagAndHealthClassification(t *testing.T) {
	service := &OpenAIGatewayService{}
	account := fastFailoverPoolAccount()
	for _, status := range []int{401, 402, 403, 404, 405, 408, 429, 500, 502, 503, 504, 529} {
		failure := service.newOpenAIAccountFailoverError(account, status, nil, []byte(`{"error":{"message":"upstream unavailable"}}`), "upstream unavailable", false, true)
		require.False(t, failure.RetryableOnSameAccount, "status=%d", status)
		require.True(t, failure.ShouldRetryNextAccount())
		if status == 429 || status >= 500 {
			_, _, eligible := classifyOpenAIAPIKeyHealthFailure(failure)
			require.True(t, eligible, "status=%d", status)
		}
	}
	account.Credentials["fast_failover"] = false
	require.True(t, service.newOpenAIAccountFailoverError(account, 429, nil, nil, "", false, true).RetryableOnSameAccount)
}

func TestFastFailover408ClassificationIsOptIn(t *testing.T) {
	gateway := &OpenAIGatewayService{}
	account := fastFailoverPoolAccount()
	body := []byte(`{"error":{"message":"request timed out"}}`)
	require.True(t, gateway.shouldFailoverOpenAIUpstreamResponse(account, 408, "", body))
	account.Credentials["fast_failover"] = false
	require.False(t, gateway.shouldFailoverOpenAIUpstreamResponse(account, 408, "", body))
	account.Credentials["fast_failover"] = true
	account.Type = AccountTypeOAuth
	require.False(t, gateway.shouldFailoverOpenAIUpstreamResponse(account, 408, "", body))
	require.False(t, gateway.shouldFailoverOpenAIUpstreamResponse(nil, 408, "", body))
}

type fastFailoverConcurrent429Repo struct {
	mockAccountRepoForGemini
	mu           sync.Mutex
	calls        int
	lastUntil    time.Time
	lastModel    string
	firstEntered chan struct{}
	releaseFirst chan struct{}
}

func (r *fastFailoverConcurrent429Repo) SetModelRateLimit(ctx context.Context, _ int64, model string, until time.Time, _ ...string) error {
	r.mu.Lock()
	r.calls++
	first := r.calls == 1
	r.mu.Unlock()
	if first {
		close(r.firstEntered)
		select {
		case <-r.releaseFirst:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r.mu.Lock()
	r.lastUntil = until
	r.lastModel = model
	r.mu.Unlock()
	return nil
}

func (r *fastFailoverConcurrent429Repo) snapshot() (int, time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.lastUntil
}

func TestFastFailover429ConcurrentPersistenceIsCoalesced(t *testing.T) {
	for _, extend := range []bool{false, true} {
		name := "same_deadline"
		if extend {
			name = "longer_deadline"
		}
		t.Run(name, func(t *testing.T) {
			repo := &fastFailoverConcurrent429Repo{firstEntered: make(chan struct{}), releaseFirst: make(chan struct{})}
			var release sync.Once
			t.Cleanup(func() { release.Do(func() { close(repo.releaseFirst) }) })
			deferred := &DeferredService{}
			t.Cleanup(func() { deferred.cooldownQueue.stop(time.Second) })
			gateway := &OpenAIGatewayService{rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil), deferredService: deferred}
			account := fastFailoverPoolAccount()
			base := time.Now().UTC().Truncate(time.Second)
			initial := base.Add(30 * time.Second)
			header := func(until time.Time) http.Header {
				return http.Header{"Retry-After": []string{until.Format(http.TimeFormat)}}
			}
			firstDone := make(chan bool, 1)
			go func() {
				firstDone <- gateway.handleOpenAIAccountUpstreamError(context.Background(), account, 429, header(initial), nil, "gpt-6.1-sol")
			}()
			select {
			case <-repo.firstEntered:
			case <-time.After(2 * time.Second):
				t.Fatal("首个冷却写入未启动")
			}
			select {
			case stopped := <-firstDone:
				require.True(t, stopped)
			case <-time.After(time.Second):
				t.Fatal("首个请求仍等待后台数据库写入")
			}
			// 数据库保持阻塞，全部 500 个请求（包括领取者）均须完成切换判定。
			results := make(chan bool, 499)
			for i := range 499 {
				until := initial
				if extend {
					until = base.Add(time.Duration(60+i%60) * time.Second)
				}
				go func(deadline time.Time) {
					results <- gateway.handleOpenAIAccountUpstreamError(context.Background(), account, 429, header(deadline), nil, "gpt-6.1-sol")
				}(until)
			}
			deadline := time.NewTimer(2 * time.Second)
			defer deadline.Stop()
			for range 499 {
				select {
				case stopped := <-results:
					require.True(t, stopped)
				case <-deadline.C:
					t.Fatal("并发请求等待了已有数据库写入")
				}
			}
			calls, _ := repo.snapshot()
			require.Equal(t, 1, calls)
			wantUntil, wantCalls := initial, 1
			if extend {
				wantUntil, wantCalls = base.Add(119*time.Second), 2
			}
			state := gateway.getOpenAIAccountModelTransientState()
			require.True(t, state.isBlocked(account.ID, "gpt-6.1-sol", wantUntil.Add(-time.Millisecond)))
			release.Do(func() { close(repo.releaseFirst) })
			require.Eventually(t, func() bool {
				calls, until := repo.snapshot()
				return calls == wantCalls && until.Equal(wantUntil)
			}, 2*time.Second, time.Millisecond)
			// 已持久化后再收到较短期限，不得新增写入或提前解除限流。
			require.True(t, gateway.handleOpenAIAccountUpstreamError(context.Background(), account, 429, header(initial), nil, "gpt-6.1-sol"))
			gateway.clearOpenAIAccountModelTransientState(account.ID, "gpt-6.1-sol")
			calls, persistedUntil := repo.snapshot()
			require.Equal(t, wantCalls, calls)
			require.Equal(t, wantUntil, persistedUntil)
			require.True(t, state.isBlocked(account.ID, "gpt-6.1-sol", wantUntil.Add(-time.Millisecond)))
			t.Logf("500 个并发 429：持久化 %d 次，最大截止时间 %s", calls, persistedUntil.Format(time.RFC3339))
		})
	}
}

func TestFastFailover429UsesModelCooldownAndRetryAfter(t *testing.T) {
	repo := &fastFailoverConcurrent429Repo{firstEntered: make(chan struct{}), releaseFirst: make(chan struct{})}
	close(repo.releaseFirst)
	rateLimit := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	deferred := &DeferredService{}
	t.Cleanup(func() { deferred.cooldownQueue.stop(time.Second) })
	gateway := &OpenAIGatewayService{rateLimitService: rateLimit, deferredService: deferred}
	account := fastFailoverPoolAccount()
	account.Credentials["model_mapping"] = map[string]any{"public-model": "upstream-model", "upstream-model": "different-model"}
	headers := http.Header{"Retry-After": []string{"120"}}
	stopped := gateway.handleOpenAIAccountUpstreamError(context.Background(), account, 429, headers, []byte(`{"error":{"message":"rate limit exceeded"}}`), "upstream-model")
	require.True(t, stopped)
	require.Eventually(t, func() bool {
		calls, until := repo.snapshot()
		return calls == 1 && time.Until(until) > 119*time.Second
	}, time.Second, time.Millisecond)
	repo.mu.Lock()
	require.Equal(t, "upstream-model", repo.lastModel)
	repo.mu.Unlock()
	require.True(t, gateway.isOpenAIAccountModelRuntimeBlocked(account, "public-model"))
	require.False(t, gateway.isOpenAIAccountModelRuntimeBlocked(account, "different-model"))
	require.False(t, gateway.isOpenAIAccountRuntimeBlocked(account))
	// 较早开始但较晚完成的成功响应不能撤销上游明确的限流期限。
	gateway.clearOpenAIAccountModelTransientState(account.ID, "upstream-model")
	require.True(t, gateway.isOpenAIAccountModelRuntimeBlocked(account, "public-model"))
}

func TestFastFailover429DefaultCooldownSurvivesWithoutRepository(t *testing.T) {
	gateway := &OpenAIGatewayService{}
	account := fastFailoverPoolAccount()
	require.True(t, gateway.handleOpenAIAccountUpstreamError(context.Background(), account, 429, nil, nil, "gpt-6.1-sol"))
	state := gateway.getOpenAIAccountModelTransientState()
	now := time.Now()
	require.True(t, state.isBlocked(account.ID, "gpt-6.1-sol", now))
	require.False(t, state.isBlocked(account.ID, "gpt-6.1-sol", now.Add(6*time.Second)))
}

func TestFastFailoverRetryable5xxUsesExistingModelBreaker(t *testing.T) {
	repo := &errorPolicyRepoStub{}
	gateway := &OpenAIGatewayService{rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
	account := fastFailoverPoolAccount()
	account.Credentials["pool_mode_retry_status_codes"] = []any{float64(524)}
	for range 2 {
		require.False(t, gateway.handleOpenAIAccountUpstreamError(context.Background(), account, 524, nil, []byte(`{"error":{"message":"upstream timeout"}}`), "gpt-6.1-sol"))
	}
	require.True(t, gateway.isOpenAIAccountModelRuntimeBlocked(account, "gpt-6.1-sol"))
	require.False(t, gateway.isOpenAIAccountModelRuntimeBlocked(account, "gpt-6-astra"))
	require.False(t, gateway.isOpenAIAccountRuntimeBlocked(account))
}

func TestFastFailoverDoesNotPenalizeClient400OrRequestScopedCapacity(t *testing.T) {
	repo := &errorPolicyRepoStub{}
	gateway := &OpenAIGatewayService{rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
	account := fastFailoverPoolAccount()
	for _, tc := range []struct {
		status int
		body   []byte
	}{
		{400, []byte(`{"error":{"message":"Invalid type for input[0].arguments"}}`)},
		{429, []byte(`{"error":{"type":"server_error","message":"Our servers are currently overloaded. Please try again later."}}`)},
	} {
		require.False(t, gateway.handleOpenAIAccountUpstreamError(context.Background(), account, tc.status, nil, tc.body, "gpt-6.1-sol"))
		require.False(t, gateway.isOpenAIAccountModelRuntimeBlocked(account, "gpt-6.1-sol"))
	}
	require.Zero(t, repo.setErrCalls)
	require.Zero(t, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
}

func TestFastFailoverModel404KeepsOtherModelsAvailable(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{}
	gateway := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
	account := fastFailoverPoolAccount()
	require.True(t, gateway.handleOpenAIAccountUpstreamError(context.Background(), account, 404, nil, []byte(`{"error":{"code":"model_not_found","message":"model not found"}}`), "gpt-6.1-sol"))
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Zero(t, repo.tempCalls)
	require.False(t, gateway.isOpenAIAccountRuntimeBlocked(account))
}
