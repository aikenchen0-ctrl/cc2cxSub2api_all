package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/attemptpolicy"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestLogMediaAttemptPolicyDryRunRecordsDecision(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	reqLog := zap.New(core)

	logMediaAttemptPolicyDryRun(
		reqLog,
		"openai.images",
		attemptpolicy.EndpointImageSync,
		&service.UpstreamFailoverError{
			StatusCode:             http.StatusServiceUnavailable,
			RetryableOnSameAccount: true,
		},
		false,
		false,
		0,
	)

	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	require.Equal(t, "openai.images.attempt_policy_dry_run", entry.Message)
	fields := entry.ContextMap()
	require.Equal(t, string(attemptpolicy.EndpointImageSync), fields["endpoint_kind"])
	require.Equal(t, string(attemptpolicy.FailureTransientSameAccount), fields["failure_class"])
	require.Equal(t, string(attemptpolicy.ActionRepairSameAccount), fields["attempt_action"])
	require.Equal(t, false, fields["penalize_account"])
	require.Equal(t, 250*time.Millisecond, fields["repair_delay"])
}

func TestLogMediaAttemptPolicyDryRunNilLoggerIsNoop(t *testing.T) {
	require.NotPanics(t, func() {
		logMediaAttemptPolicyDryRun(nil, "openai.images", attemptpolicy.EndpointImageSync, nil, false, false, 0)
	})
}

func TestGrokMediaAttemptPolicyEndpoint(t *testing.T) {
	require.Equal(t, attemptpolicy.EndpointVideoLookup, grokMediaAttemptPolicyEndpoint(service.GrokMediaEndpointVideoStatus))
	require.Equal(t, attemptpolicy.EndpointVideoLookup, grokMediaAttemptPolicyEndpoint(service.GrokMediaEndpointVideoContent))
	require.Equal(t, attemptpolicy.EndpointVideoCreate, grokMediaAttemptPolicyEndpoint(service.GrokMediaEndpointVideosGenerations))
	require.Equal(t, attemptpolicy.EndpointImageSync, grokMediaAttemptPolicyEndpoint(service.GrokMediaEndpointImagesGenerations))
}

func TestDecideMediaAttemptRepairsOnlyWithinEndpointBudget(t *testing.T) {
	image := decideMediaAttempt(attemptpolicy.EndpointImageSync, &service.UpstreamFailoverError{
		StatusCode:             http.StatusServiceUnavailable,
		RetryableOnSameAccount: true,
	}, true, false, 0)
	require.True(t, image.RepairAllowed)
	require.False(t, image.Stop)

	imageExhausted := decideMediaAttempt(attemptpolicy.EndpointImageSync, &service.UpstreamFailoverError{
		StatusCode:             http.StatusServiceUnavailable,
		RetryableOnSameAccount: true,
	}, true, false, 1)
	require.False(t, imageExhausted.RepairAllowed)
	require.False(t, imageExhausted.Stop)
	require.Equal(t, attemptpolicy.ActionSwitchAccount, imageExhausted.Decision.Action)
}

func TestDecideMediaAttemptVideoLookupUsesTwoRepairsAndNeverSwitches(t *testing.T) {
	for repairs := 0; repairs < 2; repairs++ {
		action := decideMediaAttempt(attemptpolicy.EndpointVideoLookup, &service.UpstreamFailoverError{
			StatusCode:             http.StatusServiceUnavailable,
			RetryableOnSameAccount: true,
		}, true, false, repairs)
		require.True(t, action.RepairAllowed, "repairs=%d", repairs)
	}

	exhausted := decideMediaAttempt(attemptpolicy.EndpointVideoLookup, &service.UpstreamFailoverError{
		StatusCode:             http.StatusServiceUnavailable,
		RetryableOnSameAccount: true,
	}, true, false, 2)
	require.False(t, exhausted.RepairAllowed)
	require.Equal(t, attemptpolicy.ActionSwitchAccount, exhausted.Decision.Action)
}

func TestDecideMediaAttemptVideoCreateAmbiguousStops(t *testing.T) {
	action := decideMediaAttempt(attemptpolicy.EndpointVideoCreate, &service.UpstreamFailoverError{
		StatusCode:        http.StatusBadGateway,
		Reason:            service.MediaVideoCreateAmbiguousReason,
		NextAccountAction: service.NextAccountStop,
	}, true, false, 0)
	require.True(t, action.Stop)
	require.True(t, action.Ambiguous)
	require.False(t, action.RepairAllowed)
}

func TestDecideMediaAttemptResponseStartedStops(t *testing.T) {
	action := decideMediaAttempt(attemptpolicy.EndpointImageSync, &service.UpstreamFailoverError{
		StatusCode: http.StatusUnauthorized,
	}, true, true, 0)
	require.True(t, action.Stop)
	require.False(t, action.RepairAllowed)
}

func TestApplyMediaPolicyClientResponsePreservesExplicitUpstreamMessage(t *testing.T) {
	failoverErr := &service.UpstreamFailoverError{}
	decision := attemptpolicy.Decision{
		ClientStatus:  http.StatusBadGateway,
		ClientMessage: "policy message",
	}
	applyMediaPolicyClientResponse(failoverErr, decision)
	require.Equal(t, http.StatusBadGateway, failoverErr.ClientStatusCode)
	require.Equal(t, "policy message", failoverErr.ClientMessage)

	failoverErr = &service.UpstreamFailoverError{
		ClientStatusCode: http.StatusServiceUnavailable,
		ClientMessage:    "upstream-specific",
	}
	applyMediaPolicyClientResponse(failoverErr, attemptpolicy.Decision{
		ClientStatus:  http.StatusBadGateway,
		ClientMessage: "policy message",
	})
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.ClientStatusCode)
	require.Equal(t, "upstream-specific", failoverErr.ClientMessage)
}
