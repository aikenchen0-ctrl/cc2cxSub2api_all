package service

import (
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/attemptpolicy"
)

// AttemptPolicyEndpointForFailover classifies a failover error shape for
// observation. It is deliberately conservative: unknown or media-specific
// signals stay on the text policy until the caller can prove a narrower
// endpoint kind.
func AttemptPolicyEndpointForFailover(err *UpstreamFailoverError) attemptpolicy.EndpointKind {
	if err == nil {
		return attemptpolicy.EndpointText
	}
	switch err.Reason {
	case MediaVideoCreateAmbiguousReason:
		return attemptpolicy.EndpointVideoCreate
	default:
		return attemptpolicy.EndpointText
	}
}

// ObserveAttemptPolicy maps the existing failover sentinel into the shared,
// side-effect-free policy. Callers use this only for dry-run logging until the
// corresponding loop has been migrated to the policy decision.
func ObserveAttemptPolicy(
	endpoint attemptpolicy.EndpointKind,
	err *UpstreamFailoverError,
	requestSent bool,
	responseStarted bool,
	sameAccountRepairs int,
) attemptpolicy.Decision {
	requestSentKnown := false
	if err != nil {
		switch err.RequestPhase {
		case UpstreamRequestPhaseNotSent:
			requestSent = false
			requestSentKnown = true
		case UpstreamRequestPhaseSent:
			requestSent = true
			requestSentKnown = true
		}
	}
	obs := attemptpolicy.Observation{
		Endpoint:           endpoint,
		RequestSent:        requestSent,
		ResponseStarted:    responseStarted,
		SameAccountRepairs: sameAccountRepairs,
		CanReplay:          !responseStarted,
	}
	if err == nil {
		return attemptpolicy.Classify(obs)
	}

	obs.StatusCode = err.StatusCode
	obs.RetryableOnSameAccount = err.RetryableOnSameAccount
	obs.RequestScopedTransient = err.RequestScopedTransient
	obs.AccountAuthFailure = err.IsCredentialFailure()
	obs.AccountStateFailure = err.Scope == GatewayFailureScopeAccount && err.StatusCode == http.StatusForbidden
	obs.UpstreamModelMiss = attemptpolicy.IsUpstreamModelMissStatus(err.StatusCode)
	obs.RequestInvalid = !err.ShouldRetryNextAccount() ||
		err.StatusCode == http.StatusBadRequest || err.StatusCode == http.StatusUnprocessableEntity
	obs.CipherOrSignature = isCipherOrSignatureFailoverReason(err.Reason) ||
		containsCipherOrSignatureError(err.ResponseBody)
	// status 0 is the legacy transport-failure shape. Prefer the explicit local
	// phase when present; otherwise use the caller's observation.
	if requestSentKnown {
		obs.TransportAfterSend = err.RequestPhase == UpstreamRequestPhaseSent && err.StatusCode == 0
	} else {
		obs.TransportAfterSend = requestSent && err.StatusCode == 0
	}
	if err.SameAccountRetryDelay > 0 {
		obs.RetryAfter = err.SameAccountRetryDelay
		obs.RetryAfterKnown = true
	}
	if resetAt := parseRetryAfterResetTime(err.ResponseHeaders, time.Now()); resetAt != nil {
		resetAfter := time.Until(*resetAt)
		if resetAfter > 0 {
			obs.QuotaResetAfter = resetAfter
			if !obs.RetryAfterKnown && err.StatusCode == http.StatusTooManyRequests {
				obs.RetryAfter = resetAfter
				obs.RetryAfterKnown = true
			}
		}
	}
	// A short Retry-After is actionable in-request even when the upstream did
	// not pre-classify the 429 as same-account retryable. Let the shared policy
	// see the delay and decide; a long/absent reset still becomes quota.
	obs.AccountQuotaFailure = err.StatusCode == http.StatusTooManyRequests &&
		!err.RetryableOnSameAccount && !obs.RetryAfterKnown
	if endpoint == attemptpolicy.EndpointVideoCreate && err.Reason == MediaVideoCreateAmbiguousReason {
		obs.AmbiguousPossible = true
		obs.CanReplay = false
	}
	return attemptpolicy.Classify(obs)
}

func isCipherOrSignatureFailoverReason(reason GatewayFailureReason) bool {
	switch string(reason) {
	case "invalid_encrypted_content", "thinking_signature", "signature_error", "cipher_or_signature":
		return true
	default:
		return false
	}
}

func containsCipherOrSignatureError(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "invalid_encrypted_content") ||
		(strings.Contains(lower, "encrypted_content") && strings.Contains(lower, "invalid")) ||
		(strings.Contains(lower, "thinking") && strings.Contains(lower, "signature"))
}
