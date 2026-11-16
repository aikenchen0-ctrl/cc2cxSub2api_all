package attemptpolicy

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClassifyHardAccountFailuresSwitchWithoutRepair(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			got := Classify(Observation{Endpoint: EndpointImageSync, StatusCode: status, CanReplay: true})
			require.Equal(t, ActionSwitchAccount, got.Action)
			require.True(t, got.PenalizeAccount)
			require.Equal(t, defaultAuthCooldown, got.Cooldown)
			require.Zero(t, got.RepairLimit)
		})
	}
}

func TestClassifyShortRateLimitRepairsWithoutPenalty(t *testing.T) {
	got := Classify(Observation{
		Endpoint:        EndpointImageSync,
		StatusCode:      http.StatusTooManyRequests,
		RetryAfter:      500 * time.Millisecond,
		RetryAfterKnown: true,
		CanReplay:       true,
	})
	require.Equal(t, FailureTransientSameAccount, got.Class)
	require.Equal(t, ActionRepairSameAccount, got.Action)
	require.False(t, got.PenalizeAccount)
	require.Zero(t, got.Cooldown)
}

func TestClassifyLongRateLimitSwitchesAndCoolsAccount(t *testing.T) {
	got := Classify(Observation{
		Endpoint:        EndpointImageSync,
		StatusCode:      http.StatusTooManyRequests,
		RetryAfter:      30 * time.Second,
		RetryAfterKnown: true,
		CanReplay:       true,
	})
	require.Equal(t, FailureAccountQuota, got.Class)
	require.Equal(t, ActionSwitchAccount, got.Action)
	require.True(t, got.PenalizeAccount)
	require.Equal(t, defaultQuotaCooldown, got.Cooldown)
}

func TestClassifyQuotaUsesKnownResetWithinCap(t *testing.T) {
	got := Classify(Observation{
		Endpoint:        EndpointImageSync,
		StatusCode:      http.StatusTooManyRequests,
		RetryAfter:      20 * time.Second,
		RetryAfterKnown: true,
		QuotaResetAfter: 15 * time.Second,
		CanReplay:       true,
	})
	require.Equal(t, ActionSwitchAccount, got.Action)
	require.Equal(t, 15*time.Second, got.Cooldown)

	got = Classify(Observation{
		Endpoint:        EndpointImageSync,
		StatusCode:      http.StatusTooManyRequests,
		RetryAfter:      2 * time.Hour,
		RetryAfterKnown: true,
		QuotaResetAfter: 2 * time.Hour,
		CanReplay:       true,
	})
	require.Equal(t, defaultQuotaCooldown, got.Cooldown)
}

func TestClassifyTransientServerFailureRepairsThenSwitches(t *testing.T) {
	first := Classify(Observation{Endpoint: EndpointImageSync, StatusCode: http.StatusServiceUnavailable, CanReplay: true})
	require.Equal(t, ActionRepairSameAccount, first.Action)
	require.False(t, first.PenalizeAccount)

	second := Classify(Observation{
		Endpoint:           EndpointImageSync,
		StatusCode:         http.StatusServiceUnavailable,
		CanReplay:          true,
		SameAccountRepairs: 1,
	})
	require.Equal(t, ActionSwitchAccount, second.Action)
	require.True(t, second.PenalizeAccount)
	require.Equal(t, defaultTransientCooldown, second.Cooldown)
}

func TestClassifyRequestScopedTransientSwitchDoesNotPenalize(t *testing.T) {
	got := Classify(Observation{
		Endpoint:               EndpointImageSync,
		StatusCode:             http.StatusServiceUnavailable,
		CanReplay:              true,
		RequestScopedTransient: true,
		SameAccountRepairs:     1,
	})
	require.Equal(t, ActionSwitchAccount, got.Action)
	require.Equal(t, FailureCapacityRequestScope, got.Class)
	require.False(t, got.PenalizeAccount)
	require.Zero(t, got.Cooldown)
}

func TestClassifyInvalidRequestStops(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity} {
		got := Classify(Observation{Endpoint: EndpointText, StatusCode: status, CanReplay: true})
		require.Equal(t, FailureRequestInvalid, got.Class)
		require.Equal(t, ActionStopRequest, got.Action)
		require.False(t, got.PenalizeAccount)
	}
}

func TestClassifyResponseStartedStopsEvenForHardFailure(t *testing.T) {
	got := Classify(Observation{
		Endpoint:        EndpointImageSync,
		StatusCode:      http.StatusUnauthorized,
		ResponseStarted: true,
		CanReplay:       true,
	})
	require.Equal(t, FailureResponseStarted, got.Class)
	require.Equal(t, ActionStopRequest, got.Action)
	require.False(t, got.PenalizeAccount)
}

func TestClassifyVideoCreateAmbiguousStopsBeforeRetry(t *testing.T) {
	got := Classify(Observation{
		Endpoint:          EndpointVideoCreate,
		StatusCode:        http.StatusBadGateway,
		RequestSent:       true,
		AmbiguousPossible: true,
		CanReplay:         true,
	})
	require.Equal(t, FailureAmbiguousMediaCreate, got.Class)
	require.Equal(t, ActionStopAmbiguous, got.Action)
	require.Zero(t, got.Cooldown)
}

func TestClassifyVideoCreateExplicitTransientUsesNoSameAccountRepairs(t *testing.T) {
	got := Classify(Observation{
		Endpoint:    EndpointVideoCreate,
		StatusCode:  http.StatusServiceUnavailable,
		RequestSent: true,
		CanReplay:   true,
	})
	require.Equal(t, ActionSwitchAccount, got.Action)
	require.True(t, got.PenalizeAccount)
}

func TestClassifyVideoLookupUsesLargerRepairBudget(t *testing.T) {
	for repairs := 0; repairs < videoLookupRepairMax; repairs++ {
		got := Classify(Observation{
			Endpoint:           EndpointVideoLookup,
			StatusCode:         http.StatusServiceUnavailable,
			CanReplay:          true,
			SameAccountRepairs: repairs,
		})
		require.Equal(t, ActionRepairSameAccount, got.Action)
	}

	got := Classify(Observation{
		Endpoint:           EndpointVideoLookup,
		StatusCode:         http.StatusServiceUnavailable,
		CanReplay:          true,
		SameAccountRepairs: videoLookupRepairMax,
	})
	require.Equal(t, ActionSwitchAccount, got.Action)
}

func TestClassifyCipherFailureRepairsOnlyWhenReplayIsSafe(t *testing.T) {
	repairable := Classify(Observation{
		Endpoint:          EndpointText,
		StatusCode:        http.StatusBadRequest,
		CipherOrSignature: true,
		CanReplay:         true,
	})
	require.Equal(t, FailureCipherOrSignature, repairable.Class)
	require.Equal(t, ActionRepairSameAccount, repairable.Action)

	unrecoverable := Classify(Observation{
		Endpoint:          EndpointText,
		StatusCode:        http.StatusBadRequest,
		CipherOrSignature: true,
		CanReplay:         false,
	})
	require.Equal(t, ActionStopRequest, unrecoverable.Action)
	require.False(t, unrecoverable.PenalizeAccount)
}

func TestClassifyTransportAfterSend(t *testing.T) {
	t.Run("video create is ambiguous", func(t *testing.T) {
		got := Classify(Observation{
			Endpoint:           EndpointVideoCreate,
			RequestSent:        true,
			TransportAfterSend: true,
			CanReplay:          true,
		})
		require.Equal(t, ActionStopAmbiguous, got.Action)
	})

	t.Run("text without replay stops", func(t *testing.T) {
		got := Classify(Observation{
			Endpoint:           EndpointText,
			RequestSent:        true,
			TransportAfterSend: true,
			CanReplay:          false,
		})
		require.Equal(t, ActionStopRequest, got.Action)
	})

	t.Run("replayable media repairs", func(t *testing.T) {
		got := Classify(Observation{
			Endpoint:           EndpointImageSync,
			RequestSent:        true,
			TransportAfterSend: true,
			CanReplay:          true,
		})
		require.Equal(t, ActionRepairSameAccount, got.Action)
	})
}

func TestRepairLimit(t *testing.T) {
	require.Equal(t, 0, RepairLimit(EndpointVideoCreate))
	require.Equal(t, videoLookupRepairMax, RepairLimit(EndpointVideoLookup))
	require.Equal(t, mediaRepairMax, RepairLimit(EndpointImageSync))
	require.Equal(t, textRepairMax, RepairLimit(EndpointText))
}
