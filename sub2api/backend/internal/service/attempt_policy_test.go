package service

import (
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/attemptpolicy"
	"github.com/stretchr/testify/require"
)

func TestObserveAttemptPolicy_MapsAuthFailureToSwitch(t *testing.T) {
	got := ObserveAttemptPolicy(attemptpolicy.EndpointImageSync, &UpstreamFailoverError{
		StatusCode: http.StatusUnauthorized,
		Stage:      GatewayFailureStageAccountAuth,
		Scope:      GatewayFailureScopeAccount,
	}, true, false, 0)

	require.Equal(t, attemptpolicy.FailureAccountAuth, got.Class)
	require.Equal(t, attemptpolicy.ActionSwitchAccount, got.Action)
	require.True(t, got.PenalizeAccount)
	require.Equal(t, 60*time.Second, got.Cooldown)
}

func TestObserveAttemptPolicy_MapsShortRateLimitToRepair(t *testing.T) {
	got := ObserveAttemptPolicy(attemptpolicy.EndpointImageSync, &UpstreamFailoverError{
		StatusCode:             http.StatusTooManyRequests,
		RetryableOnSameAccount: true,
		SameAccountRetryDelay:  500 * time.Millisecond,
	}, true, false, 0)

	require.Equal(t, attemptpolicy.FailureTransientSameAccount, got.Class)
	require.Equal(t, attemptpolicy.ActionRepairSameAccount, got.Action)
	require.False(t, got.PenalizeAccount)
}

func TestObserveAttemptPolicy_UsesRetryAfterHeaderForQuotaCooldown(t *testing.T) {
	got := ObserveAttemptPolicy(attemptpolicy.EndpointImageSync, &UpstreamFailoverError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: http.Header{"Retry-After": []string{"10"}},
	}, true, false, 0)

	require.Equal(t, attemptpolicy.FailureAccountQuota, got.Class)
	require.Equal(t, attemptpolicy.ActionSwitchAccount, got.Action)
	require.InDelta(t, 10, got.Cooldown.Seconds(), 1)
}

func TestObserveAttemptPolicy_ShortRetryAfterHeaderRepairs(t *testing.T) {
	got := ObserveAttemptPolicy(attemptpolicy.EndpointImageSync, &UpstreamFailoverError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: http.Header{"Retry-After": []string{"1"}},
	}, true, false, 0)

	require.Equal(t, attemptpolicy.FailureTransientSameAccount, got.Class)
	require.Equal(t, attemptpolicy.ActionRepairSameAccount, got.Action)
}

func TestObserveAttemptPolicy_MapsRequestScopedToNoPenalty(t *testing.T) {
	got := ObserveAttemptPolicy(attemptpolicy.EndpointImageSync, &UpstreamFailoverError{
		StatusCode:             http.StatusServiceUnavailable,
		RetryableOnSameAccount: true,
		RequestScopedTransient: true,
	}, true, false, 1)

	require.Equal(t, attemptpolicy.ActionSwitchAccount, got.Action)
	require.Equal(t, attemptpolicy.FailureCapacityRequestScope, got.Class)
	require.False(t, got.PenalizeAccount)
	require.Zero(t, got.Cooldown)
}

func TestObserveAttemptPolicy_MapsVideoAmbiguousToStop(t *testing.T) {
	got := ObserveAttemptPolicy(attemptpolicy.EndpointVideoCreate, &UpstreamFailoverError{
		StatusCode:        http.StatusBadGateway,
		Reason:            MediaVideoCreateAmbiguousReason,
		NextAccountAction: NextAccountStop,
	}, true, false, 0)

	require.Equal(t, attemptpolicy.FailureAmbiguousMediaCreate, got.Class)
	require.Equal(t, attemptpolicy.ActionStopAmbiguous, got.Action)
}

func TestObserveAttemptPolicy_ResponseStartedStops(t *testing.T) {
	got := ObserveAttemptPolicy(attemptpolicy.EndpointImageSync, &UpstreamFailoverError{
		StatusCode: http.StatusUnauthorized,
	}, true, true, 0)

	require.Equal(t, attemptpolicy.FailureResponseStarted, got.Class)
	require.Equal(t, attemptpolicy.ActionStopRequest, got.Action)
}

func TestObserveAttemptPolicy_CipherFailureRepairsWhenReplayable(t *testing.T) {
	got := ObserveAttemptPolicy(attemptpolicy.EndpointText, &UpstreamFailoverError{
		StatusCode:   http.StatusBadRequest,
		ResponseBody: []byte(`{"error":{"code":"invalid_encrypted_content"}}`),
	}, true, false, 0)

	require.Equal(t, attemptpolicy.FailureCipherOrSignature, got.Class)
	require.Equal(t, attemptpolicy.ActionRepairSameAccount, got.Action)
}

func TestObserveAttemptPolicy_UsesExplicitRequestPhase(t *testing.T) {
	notSent := ObserveAttemptPolicy(attemptpolicy.EndpointImageSync, &UpstreamFailoverError{
		StatusCode:             http.StatusBadGateway,
		RetryableOnSameAccount: true,
		RequestPhase:           UpstreamRequestPhaseNotSent,
	}, true, false, 0)
	require.Equal(t, attemptpolicy.ActionRepairSameAccount, notSent.Action, "caller observation must not override explicit not_sent")

	sent := ObserveAttemptPolicy(attemptpolicy.EndpointImageSync, &UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		RequestPhase: UpstreamRequestPhaseSent,
	}, false, false, 0)
	require.Equal(t, attemptpolicy.FailureTransientSameAccount, sent.Class)
	require.Equal(t, attemptpolicy.ActionRepairSameAccount, sent.Action)

	// Status 0 keeps the legacy transport shape, so an explicitly sent request
	// must be treated as ambiguous rather than silently replayed.
	ambiguous := ObserveAttemptPolicy(attemptpolicy.EndpointVideoCreate, &UpstreamFailoverError{
		StatusCode:   0,
		RequestPhase: UpstreamRequestPhaseSent,
	}, false, false, 0)
	require.Equal(t, attemptpolicy.FailureAmbiguousMediaCreate, ambiguous.Class)
	require.Equal(t, attemptpolicy.ActionStopAmbiguous, ambiguous.Action)
}

func TestAttemptPolicyEndpointForFailover(t *testing.T) {
	require.Equal(t, attemptpolicy.EndpointText, AttemptPolicyEndpointForFailover(nil))
	require.Equal(t, attemptpolicy.EndpointVideoCreate, AttemptPolicyEndpointForFailover(&UpstreamFailoverError{
		Reason: MediaVideoCreateAmbiguousReason,
	}))
	require.Equal(t, attemptpolicy.EndpointText, AttemptPolicyEndpointForFailover(&UpstreamFailoverError{
		StatusCode: http.StatusBadGateway,
	}))
}
