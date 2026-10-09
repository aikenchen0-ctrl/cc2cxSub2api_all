//go:build unit

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type cooldownPersistenceBlockingRepo struct {
	mockAccountRepoForGemini
	mu           sync.Mutex
	calls        int
	entered      chan struct{}
	release      chan struct{}
	ignoreCancel bool
	failure      error
}

func (r *cooldownPersistenceBlockingRepo) SetModelRateLimit(ctx context.Context, _ int64, _ string, _ time.Time, _ ...string) error {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	r.entered <- struct{}{}
	if r.ignoreCancel {
		<-r.release
		return nil
	}
	select {
	case <-r.release:
		return r.failure
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestOpenAICooldownPersistence_FailureRetainsMemoryAndSuppressesImmediateRetry(t *testing.T) {
	repo := &cooldownPersistenceBlockingRepo{entered: make(chan struct{}, 1), release: make(chan struct{}), failure: context.DeadlineExceeded}
	close(repo.release)
	deferred := &DeferredService{}
	t.Cleanup(func() { deferred.cooldownQueue.stop(time.Second) })
	gateway := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}, deferredService: deferred}
	state := gateway.getOpenAIAccountModelTransientState()
	now := time.Now()
	state.recordCooldown(1, "gpt-test", now, now.Add(time.Minute))
	gateway.enqueueOpenAICooldownPersistence(state, 1, "gpt-test", now)
	require.Eventually(t, func() bool {
		entry := cooldownPersistenceEntry(state, 1, "gpt-test")
		return entry.cooldownPersistingUntil.IsZero() && entry.cooldownPersistRetryAt.After(now)
	}, time.Second, time.Millisecond)
	for range 500 {
		gateway.enqueueOpenAICooldownPersistence(state, 1, "gpt-test", now)
	}
	repo.mu.Lock()
	require.Equal(t, 1, repo.calls)
	repo.mu.Unlock()
	require.True(t, state.isBlocked(1, "gpt-test", now))
}

func cooldownPersistenceEntry(state *openAIAccountModelTransientState, accountID int64, model string) openAIAccountModelTransientEntry {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.entries[openAIAccountModelKey{AccountID: accountID, Model: model}]
}

func TestOpenAICooldownPersistence_FullQueueAndStopReleaseClaims(t *testing.T) {
	repo := &cooldownPersistenceBlockingRepo{entered: make(chan struct{}, 2), release: make(chan struct{})}
	deferred := &DeferredService{}
	t.Cleanup(func() { deferred.cooldownQueue.stop(time.Second) })
	gateway := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}, deferredService: deferred}
	state := gateway.getOpenAIAccountModelTransientState()
	now, model := time.Now(), "gpt-test"
	submit := func(id int64) {
		state.recordCooldown(id, model, now, now.Add(time.Minute))
		gateway.enqueueOpenAICooldownPersistence(state, id, model, now)
	}
	for id := int64(1); id <= 2; id++ {
		submit(id)
	}
	for range 2 {
		select {
		case <-repo.entered:
		case <-time.After(time.Second):
			t.Fatal("后台工作协程未领取任务")
		}
	}
	for id := int64(3); id <= 66; id++ {
		submit(id)
	}
	require.Len(t, deferred.cooldownQueue.jobs, openAICooldownPersistenceBacklog)
	started := time.Now()
	submit(67)
	require.Less(t, time.Since(started), 100*time.Millisecond)
	entry := cooldownPersistenceEntry(state, 67, model)
	require.True(t, entry.cooldownPersistingUntil.IsZero())
	require.True(t, entry.cooldownPersistRetryAt.After(now))
	require.True(t, state.isBlocked(67, model, now))

	deferred.Stop()
	select {
	case <-deferred.cooldownQueue.done:
	default:
		t.Fatal("服务关闭后后台工作协程未退出")
	}
	for id := int64(1); id <= 66; id++ {
		require.True(t, cooldownPersistenceEntry(state, id, model).cooldownPersistingUntil.IsZero(), "account=%d", id)
		require.True(t, state.isBlocked(id, model, now))
	}
	repo.mu.Lock()
	require.Equal(t, 2, repo.calls)
	repo.mu.Unlock()
	submit(68)
	require.True(t, cooldownPersistenceEntry(state, 68, model).cooldownPersistingUntil.IsZero())
	deferred.Stop()
}

func TestOpenAICooldownPersistence_NoDeferredReleasesClaim(t *testing.T) {
	repo := &cooldownPersistenceBlockingRepo{}
	gateway := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
	state := gateway.getOpenAIAccountModelTransientState()
	now, model := time.Now(), "gpt-test"
	state.recordCooldown(1, model, now, now.Add(time.Minute))
	gateway.enqueueOpenAICooldownPersistence(state, 1, model, now)
	require.True(t, cooldownPersistenceEntry(state, 1, model).cooldownPersistingUntil.IsZero())
	require.True(t, state.isBlocked(1, model, now))
	require.Zero(t, repo.calls)
}

func TestOpenAICooldownPersistence_StopIsBoundedWhenRepositoryIgnoresCancel(t *testing.T) {
	repo := &cooldownPersistenceBlockingRepo{entered: make(chan struct{}, 2), release: make(chan struct{}), ignoreCancel: true}
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(repo.release) }) })
	deferred := &DeferredService{}
	gateway := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}, deferredService: deferred}
	state := gateway.getOpenAIAccountModelTransientState()
	now := time.Now()
	for id := int64(1); id <= 2; id++ {
		state.recordCooldown(id, "gpt-test", now, now.Add(time.Minute))
		gateway.enqueueOpenAICooldownPersistence(state, id, "gpt-test", now)
		select {
		case <-repo.entered:
		case <-time.After(time.Second):
			t.Fatal("后台写入未启动")
		}
	}
	state.recordCooldown(3, "gpt-test", now, now.Add(time.Minute))
	gateway.enqueueOpenAICooldownPersistence(state, 3, "gpt-test", now)
	started := time.Now()
	deferred.cooldownQueue.stop(20 * time.Millisecond)
	require.Less(t, time.Since(started), 300*time.Millisecond)
	require.True(t, cooldownPersistenceEntry(state, 3, "gpt-test").cooldownPersistingUntil.IsZero())
	require.False(t, deferred.cooldownQueue.trySubmit(openAICooldownPersistenceTask{}))
	release.Do(func() { close(repo.release) })
	select {
	case <-deferred.cooldownQueue.done:
	case <-time.After(time.Second):
		t.Fatal("数据库解除阻塞后后台协程未退出")
	}
	require.True(t, cooldownPersistenceEntry(state, 1, "gpt-test").cooldownPersistingUntil.IsZero())
}

func TestOpenAICooldownPersistence_StopBeforeStartDoesNotCreateWorkers(t *testing.T) {
	deferred := &DeferredService{}
	deferred.Stop()
	require.False(t, deferred.cooldownQueue.trySubmit(openAICooldownPersistenceTask{}))
	require.Nil(t, deferred.cooldownQueue.jobs)
	require.Nil(t, deferred.cooldownQueue.done)
}

func TestOpenAICooldownPersistence_ClientCancellationDoesNotCancelBackgroundWrite(t *testing.T) {
	repo := &fastFailoverConcurrent429Repo{firstEntered: make(chan struct{}), releaseFirst: make(chan struct{})}
	close(repo.releaseFirst)
	deferred := &DeferredService{}
	t.Cleanup(func() { deferred.cooldownQueue.stop(time.Second) })
	gateway := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}, deferredService: deferred}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.True(t, gateway.cooldownOpenAIAPIKeyRateLimit(ctx, fastFailoverPoolAccount(), nil, "gpt-test"))
	require.Eventually(t, func() bool {
		calls, until := repo.snapshot()
		return calls == 1 && until.After(time.Now())
	}, time.Second, time.Millisecond)
}
