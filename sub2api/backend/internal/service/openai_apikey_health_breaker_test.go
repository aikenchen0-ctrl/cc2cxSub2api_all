package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAIAPIKeyHealthSettingRepo struct {
	SettingRepository
	value    string
	getCalls int
}

func (r *openAIAPIKeyHealthSettingRepo) GetValue(context.Context, string) (string, error) {
	r.getCalls++
	return r.value, nil
}

type openAIAPIKeyHealthAccountRepo struct {
	AccountRepository
	setCalls int
	reason   string
	until    time.Time
}

func (r *openAIAPIKeyHealthAccountRepo) SetTempUnschedulable(_ context.Context, _ int64, until time.Time, reason string) error {
	r.setCalls++
	r.reason = reason
	r.until = until
	return nil
}

type openAIAPIKeyHealthCacheStub struct {
	TempUnschedCache
	recordCalls int
	setCalls    int
	tripped     bool
	window      int
	threshold   int
}

func (c *openAIAPIKeyHealthCacheStub) RecordOpenAIAPIKeyHealthFailure(_ context.Context, _ int64, window, threshold int) (int64, bool, error) {
	c.recordCalls++
	c.window = window
	c.threshold = threshold
	return 3, c.tripped, nil
}

func (c *openAIAPIKeyHealthCacheStub) SetTempUnsched(context.Context, int64, *TempUnschedState) error {
	c.setCalls++
	return nil
}

type openAIAPIKeyHealthRuntimeBlocker struct{ calls int }

func (b *openAIAPIKeyHealthRuntimeBlocker) BlockAccountScheduling(*Account, time.Time, string) {
	b.calls++
}
func (*openAIAPIKeyHealthRuntimeBlocker) ClearAccountSchedulingBlock(int64) {}

func openAIHealthPoolAccount() *Account {
	return &Account{
		ID:       42,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"pool_mode": true,
		},
	}
}

func TestClassifyOpenAIAPIKeyHealthFailureExclusions(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		eligible bool
	}{
		{name: "account attributed 502", err: &UpstreamFailoverError{StatusCode: http.StatusBadGateway}, eligible: true},
		{name: "request scoped capacity", err: &UpstreamFailoverError{StatusCode: 529}, eligible: true},
		{name: "provider scoped overload", err: &UpstreamFailoverError{StatusCode: 529}, eligible: true},
		{name: "dedicated same account retry", err: &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, RetryableOnSameAccount: true}, eligible: true},
		{name: "credential disable path", err: &UpstreamFailoverError{StatusCode: http.StatusUnauthorized, Stage: GatewayFailureStageAccountAuth, Scope: GatewayFailureScopeAccount}, eligible: true},
		{name: "client request", err: &UpstreamFailoverError{StatusCode: http.StatusBadRequest}, eligible: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, eligible := classifyOpenAIAPIKeyHealthFailure(tt.err)
			require.Equal(t, tt.eligible, eligible)
		})
	}
}

func TestUpstreamFailureBreakerDefaultEnabled(t *testing.T) {
	settings := NewSettingService(&openAIAPIKeyHealthSettingRepo{}, &config.Config{})
	cache := &openAIAPIKeyHealthCacheStub{tripped: true}
	svc := NewRateLimitService(&openAIAPIKeyHealthAccountRepo{}, nil, &config.Config{}, nil, cache)
	svc.SetSettingService(settings)
	svc.SetOpenAIAPIKeyHealthCache(cache)

	require.True(t, svc.ObserveOpenAIAPIKeyHealthFailure(context.Background(), openAIHealthPoolAccount(), &UpstreamFailoverError{StatusCode: http.StatusBadGateway}))
	require.Equal(t, 1, cache.recordCalls)
}

func TestUpstreamFailureBreakerCoversNonOpenAIAccounts(t *testing.T) {
	cache := &openAIAPIKeyHealthCacheStub{tripped: true}
	repo := &openAIAPIKeyHealthAccountRepo{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, cache)
	svc.SetSettingService(NewSettingService(&openAIAPIKeyHealthSettingRepo{}, &config.Config{}))
	svc.SetOpenAIAPIKeyHealthCache(cache)

	account := &Account{ID: 43, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}
	require.True(t, svc.ObserveUpstreamFailure(context.Background(), account, http.StatusServiceUnavailable, nil))
	require.Equal(t, 1, cache.recordCalls)
	require.Equal(t, 1, cache.window)
	require.Equal(t, 2, cache.threshold)
	require.Equal(t, 1, repo.setCalls)
	require.WithinDuration(t, time.Now().Add(time.Minute), repo.until, 2*time.Second)
}

func TestOpenAIAPIKeyHealthBreakerTripsPersistedAndRuntimeState(t *testing.T) {
	encoded, err := json.Marshal(OpenAIAPIKeyHealthBreakerSettings{Enabled: true, WindowMinutes: 1, FailureThreshold: 3, CooldownMinutes: 5})
	require.NoError(t, err)
	settings := NewSettingService(&openAIAPIKeyHealthSettingRepo{value: string(encoded)}, &config.Config{})
	cache := &openAIAPIKeyHealthCacheStub{tripped: true}
	repo := &openAIAPIKeyHealthAccountRepo{}
	blocker := &openAIAPIKeyHealthRuntimeBlocker{}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, cache)
	svc.SetSettingService(settings)
	svc.SetOpenAIAPIKeyHealthCache(cache)
	svc.SetAccountRuntimeBlocker(blocker)
	account := openAIHealthPoolAccount()

	require.True(t, svc.ObserveOpenAIAPIKeyHealthFailure(context.Background(), account, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte(`{"error":"upstream"}`)}))
	require.Equal(t, 1, cache.recordCalls)
	require.Equal(t, 1, cache.setCalls)
	require.Equal(t, 1, repo.setCalls)
	require.Equal(t, 1, blocker.calls)
	require.NotNil(t, account.TempUnschedulableUntil)
	require.Contains(t, repo.reason, openAIAPIKeyHealthBreakerReason)
}

func TestOpenAIAPIKeyHealthSuccessDoesNotTouchSettingsOrCache(t *testing.T) {
	encoded, err := json.Marshal(OpenAIAPIKeyHealthBreakerSettings{Enabled: true, WindowMinutes: 1, FailureThreshold: 3, CooldownMinutes: 5})
	require.NoError(t, err)
	settingRepo := &openAIAPIKeyHealthSettingRepo{value: string(encoded)}
	settings := NewSettingService(settingRepo, &config.Config{})
	cache := &openAIAPIKeyHealthCacheStub{}
	svc := NewRateLimitService(&openAIAPIKeyHealthAccountRepo{}, nil, &config.Config{}, nil, cache)
	svc.SetSettingService(settings)
	svc.SetOpenAIAPIKeyHealthCache(cache)

	svc.ObserveOpenAIAPIKeyHealthSuccess(context.Background(), openAIHealthPoolAccount())
	svc.ObserveOpenAIAPIKeyHealthSuccess(context.Background(), &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeAPIKey})
	require.Zero(t, settingRepo.getCalls)
	require.Zero(t, cache.recordCalls)
}
