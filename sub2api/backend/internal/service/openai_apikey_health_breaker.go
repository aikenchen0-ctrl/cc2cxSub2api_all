package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const openAIAPIKeyHealthBreakerReason = "openai_apikey_health_breaker"

func isOpenAIAPIKeyHealthBreakerAccount(account *Account) bool {
	return account != nil && account.ID > 0
}

func classifyOpenAIAPIKeyHealthFailure(err error) (int, []byte, bool) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return 0, nil, false
	}

	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		// Every upstream failure participates in the short breaker, including
		// 4xx, 429, 503 and request/provider-scoped overload responses.
		if failoverErr.StatusCode > 0 {
			return failoverErr.StatusCode, failoverErr.ResponseBody, true
		}
		return http.StatusBadGateway, failoverErr.ResponseBody, true
	}

	var imageErr *OpenAIImagesUpstreamError
	if errors.As(err, &imageErr) {
		if imageErr.StatusCode > 0 {
			return imageErr.StatusCode, []byte(strings.TrimSpace(imageErr.Message)), true
		}
	}
	return 0, nil, false
}

// ObserveUpstreamFailure records failures for non-OpenAI gateway paths. OpenAI
// requests use ObserveOpenAIAPIKeyHealthFailure from their scheduler result
// path, so they are excluded here to avoid double counting.
func (s *RateLimitService) ObserveUpstreamFailure(ctx context.Context, account *Account, statusCode int, responseBody []byte) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if account == nil || account.Platform == PlatformOpenAI {
		return false
	}
	return s.observeAccountHealthFailure(ctx, account, &UpstreamFailoverError{
		StatusCode: statusCode, ResponseBody: responseBody,
	})
}

func (s *RateLimitService) ObserveOpenAIAPIKeyHealthFailure(ctx context.Context, account *Account, upstreamErr error) bool {
	if account == nil || account.Platform != PlatformOpenAI {
		return false
	}
	return s.observeAccountHealthFailure(ctx, account, upstreamErr)
}

func (s *RateLimitService) observeAccountHealthFailure(ctx context.Context, account *Account, upstreamErr error) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if s == nil || s.openAIAPIKeyHealth == nil || s.accountRepo == nil || !isOpenAIAPIKeyHealthBreakerAccount(account) {
		return false
	}
	statusCode, responseBody, eligible := classifyOpenAIAPIKeyHealthFailure(upstreamErr)
	if !eligible {
		return false
	}
	const (
		failureWindowMinutes = 1
		failureThreshold     = 2
		cooldownMinutes      = 1
	)
	count, tripped, err := s.openAIAPIKeyHealth.RecordOpenAIAPIKeyHealthFailure(ctx, account.ID, failureWindowMinutes, failureThreshold)
	if err != nil {
		logger.L().Warn("openai.apikey_health_breaker_record_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return false
	}
	if !tripped {
		return false
	}

	now := time.Now()
	until := now.Add(cooldownMinutes * time.Minute)
	state := &TempUnschedState{
		UntilUnix:            until.Unix(),
		TriggeredAtUnix:      now.Unix(),
		StatusCode:           statusCode,
		MatchedKeyword:       openAIAPIKeyHealthBreakerReason,
		RuleIndex:            -1,
		ErrorMessage:         truncateTempUnschedMessage(responseBody, tempUnschedMessageMaxBytes),
		TriggerCount:         count,
		TriggerThreshold:     failureThreshold,
		TriggerWindowMinutes: failureWindowMinutes,
	}
	reasonBytes, _ := json.Marshal(state)
	reason := string(reasonBytes)
	if reason == "" {
		reason = fmt.Sprintf("%s: %d failures in %d minute(s)", openAIAPIKeyHealthBreakerReason, count, failureWindowMinutes)
	}

	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := s.accountRepo.SetTempUnschedulable(persistCtx, account.ID, until, reason); err != nil {
		logger.L().Warn("openai.apikey_health_breaker_persist_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		// Keep the current process fail-closed even when the database write is
		// temporarily unavailable. A later request can retry persistence after
		// the one-minute in-memory cooldown expires.
		account.TempUnschedulableUntil = &until
		account.TempUnschedulableReason = reason
		s.notifyAccountSchedulingBlocked(account, until, openAIAPIKeyHealthBreakerReason)
		return true
	}

	if account.TempUnschedulableUntil == nil || account.TempUnschedulableUntil.Before(until) {
		account.TempUnschedulableUntil = &until
		account.TempUnschedulableReason = reason
	}
	s.notifyAccountSchedulingBlocked(account, until, openAIAPIKeyHealthBreakerReason)
	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.SetTempUnsched(persistCtx, account.ID, state); err != nil {
			logger.L().Warn("openai.apikey_health_breaker_cache_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		}
	}
	logger.L().Warn("openai.apikey_health_breaker_tripped",
		zap.Int64("account_id", account.ID),
		zap.Int64("failure_count", count),
		zap.Int("failure_threshold", failureThreshold),
		zap.Int("window_minutes", failureWindowMinutes),
		zap.Int("cooldown_minutes", cooldownMinutes),
		zap.Int("upstream_status", statusCode),
		zap.Time("until", until),
	)
	return true
}

func (s *RateLimitService) ObserveOpenAIAPIKeyHealthSuccess(context.Context, *Account) {
	// Health failures are accumulated in a rolling time window. A success does
	// not reset that window and must not add a Redis round trip to the hot path.
}
