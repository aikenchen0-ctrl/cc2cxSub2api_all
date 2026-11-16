package service

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/attemptpolicy"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	mediaSameAccountRetryMax = 1
	// videoLookupSameAccountRetryMax matches the shared policy: a status poll is
	// read-only, so two short same-owner repairs are safe and avoid needless
	// ownership errors when the task's account has a momentary blip.
	videoLookupSameAccountRetryMax = 2
	mediaSameAccountRetryDelay     = 500 * time.Millisecond
	mediaRateLimitMaxRetryDelay    = 2 * time.Second
)

// MediaVideoCreateAmbiguousReason marks a video-create attempt whose outcome is
// unknown because the request reached the network path but no response arrived.
// Replaying it on the same or another account could create and bill a duplicate
// asynchronous task, so failover must stop here.
const MediaVideoCreateAmbiguousReason = GatewayFailureReason("media_video_create_ambiguous_after_send")

const MediaVideoCreateAmbiguousClientMessage = "Video creation status is unknown after an upstream connection interruption; check the video list before retrying to avoid a duplicate task"

// isMediaTransientServerFailure covers failures that can recover after a short
// wait on the same upstream account. It deliberately excludes auth/quota errors,
// which need a credential or account switch rather than another replay.
func isMediaTransientServerFailure(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout, http.StatusTooEarly:
		return true
	default:
		return statusCode >= 500 && statusCode <= 599
	}
}

func isMediaTransientFailure(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || isMediaTransientServerFailure(statusCode)
}

// applyMediaSameAccountRetryPolicy gives media requests a bounded same-account
// replay before rotating credentials. This preserves sticky upstream cache and
// avoids account churn for short 429/5xx/timeout blips. Account/auth failures
// never consume that budget, and fast-failover accounts keep their explicit
// switch-immediately policy.
func applyMediaSameAccountRetryPolicy(account *Account, failoverErr *UpstreamFailoverError, allowSameAccount bool) {
	if failoverErr == nil {
		return
	}
	if account != nil && account.ShouldFailoverImmediately(failoverErr.StatusCode) {
		failoverErr.RetryableOnSameAccount = false
		return
	}
	switch failoverErr.StatusCode {
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed:
		failoverErr.RetryableOnSameAccount = false
		return
	}
	if !allowSameAccount {
		failoverErr.RetryableOnSameAccount = false
		return
	}
	// A plain 429 normally parks the account until its quota reset. Replaying
	// before that boundary only burns the retry budget and delays the healthy
	// account switch. Keep the replay only when the service already classified
	// the 429 as short capacity pressure, or when Retry-After is short enough
	// to wait out in-request.
	if failoverErr.StatusCode == http.StatusTooManyRequests && !failoverErr.RetryableOnSameAccount {
		resetAt := parseRetryAfterResetTime(failoverErr.ResponseHeaders, time.Now())
		if resetAt == nil {
			failoverErr.RetryableOnSameAccount = false
			return
		}
		wait := time.Until(*resetAt)
		if wait <= 0 {
			wait = 0
		}
		if wait > mediaRateLimitMaxRetryDelay {
			failoverErr.RetryableOnSameAccount = false
			return
		}
		if wait > mediaSameAccountRetryDelay {
			failoverErr.SameAccountRetryDelay = wait
		}
	}
	if !isMediaTransientFailure(failoverErr.StatusCode) {
		return
	}
	failoverErr.RetryableOnSameAccount = true
	if isMediaTransientServerFailure(failoverErr.StatusCode) {
		failoverErr.RequestScopedTransient = true
	}
	failoverErr.SameAccountRetryMax = mediaSameAccountRetryMax
	if failoverErr.SameAccountRetryDelay <= 0 {
		failoverErr.SameAccountRetryDelay = mediaSameAccountRetryDelay
	}
}

// applyVideoLookupSameAccountRetryPolicy raises only the read-only video lookup
// budget. It never enables a cross-account query; the handler still pins the
// owner account. Keeping this separate prevents create/edit/extend requests
// from inheriting a replay budget.
func applyVideoLookupSameAccountRetryPolicy(failoverErr *UpstreamFailoverError, allowSameAccount bool) {
	if failoverErr == nil || !allowSameAccount || !failoverErr.RetryableOnSameAccount {
		return
	}
	switch failoverErr.StatusCode {
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed:
		return
	}
	if !isMediaTransientFailure(failoverErr.StatusCode) {
		return
	}
	failoverErr.SameAccountRetryMax = videoLookupSameAccountRetryMax
	if failoverErr.SameAccountRetryDelay <= 0 {
		failoverErr.SameAccountRetryDelay = mediaSameAccountRetryDelay
	}
}

func newMediaEmptyOutputFailoverError(statusCode int, body []byte) *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode:             statusCode,
		ResponseBody:           body,
		RequestPhase:           UpstreamRequestPhaseSent,
		RetryableOnSameAccount: true,
		RequestScopedTransient: true,
		SameAccountRetryMax:    mediaSameAccountRetryMax,
		SameAccountRetryDelay:  mediaSameAccountRetryDelay,
	}
}

// ApplyMediaAccountCooldown persists a policy-requested cooldown for media
// failures that cannot go through the ordinary scheduler-result path.
// Request-scoped and non-penalizing decisions never touch account state.
func (s *OpenAIGatewayService) ApplyMediaAccountCooldown(account *Account, decision attemptpolicy.Decision) bool {
	if s == nil || account == nil || s.accountRepo == nil {
		return false
	}
	if !decision.PenalizeAccount || decision.Cooldown <= 0 {
		return false
	}
	until := time.Now().Add(decision.Cooldown)
	if _, written := s.blockAccountSchedulingLocked(account, until, "media_attempt_policy"); written {
		// The in-memory block already prevents immediate selection; persistence
		// makes it survive process restarts and other replicas.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, "media_attempt_policy"); err != nil {
			logger.L().Warn("media_attempt_policy.persist_cooldown_failed",
				zap.Int64("account_id", account.ID),
				zap.Error(err))
		}
		return true
	}
	return false
}
