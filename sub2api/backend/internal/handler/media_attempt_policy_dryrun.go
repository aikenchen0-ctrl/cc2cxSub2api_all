package handler

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/attemptpolicy"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

// logMediaAttemptPolicyDryRun records what the shared attempt policy would do
// without changing the caller's existing failover behavior. It is intentionally
// best-effort: logging must never block or fail a media request.
func logMediaAttemptPolicyDryRun(
	reqLog *zap.Logger,
	logPrefix string,
	endpoint attemptpolicy.EndpointKind,
	failoverErr *service.UpstreamFailoverError,
	requestSent bool,
	responseStarted bool,
	sameAccountRepairs int,
) {
	if reqLog == nil {
		return
	}
	decision := service.ObserveAttemptPolicy(endpoint, failoverErr, requestSent, responseStarted, sameAccountRepairs)
	fields := []zap.Field{
		zap.String("endpoint_kind", string(endpoint)),
		zap.String("failure_class", string(decision.Class)),
		zap.String("attempt_action", string(decision.Action)),
		zap.Bool("penalize_account", decision.PenalizeAccount),
		zap.Duration("cooldown", decision.Cooldown),
		zap.Int("repair_limit", decision.RepairLimit),
		zap.Duration("repair_delay", decision.RepairDelay),
		zap.Int("same_account_repairs", sameAccountRepairs),
		zap.Bool("request_sent", requestSent),
		zap.Bool("response_started", responseStarted),
	}
	if failoverErr != nil {
		fields = append(fields,
			zap.Int("upstream_status", failoverErr.StatusCode),
			zap.String("failover_reason", string(failoverErr.Reason)),
			zap.String("request_phase", string(failoverErr.RequestPhase)),
		)
	}
	reqLog.Info(logPrefix+".attempt_policy_dry_run", fields...)
}

// logAttemptPolicyDryRun is the endpoint-agnostic observation entry point for
// text and streaming paths. It intentionally never sleeps or selects an account.
func logAttemptPolicyDryRun(
	reqLog *zap.Logger,
	logPrefix string,
	endpoint attemptpolicy.EndpointKind,
	failoverErr *service.UpstreamFailoverError,
	requestSent bool,
	responseStarted bool,
	sameAccountRepairs int,
) {
	if reqLog == nil {
		return
	}
	decision := service.ObserveAttemptPolicy(endpoint, failoverErr, requestSent, responseStarted, sameAccountRepairs)
	fields := []zap.Field{
		zap.String("endpoint_kind", string(endpoint)),
		zap.String("failure_class", string(decision.Class)),
		zap.String("attempt_action", string(decision.Action)),
		zap.Bool("penalize_account", decision.PenalizeAccount),
		zap.Duration("cooldown", decision.Cooldown),
		zap.Int("repair_limit", decision.RepairLimit),
		zap.Duration("repair_delay", decision.RepairDelay),
		zap.Int("same_account_repairs", sameAccountRepairs),
		zap.Bool("request_sent", requestSent),
		zap.Bool("response_started", responseStarted),
	}
	if failoverErr != nil {
		fields = append(fields,
			zap.Int("upstream_status", failoverErr.StatusCode),
			zap.String("failover_reason", string(failoverErr.Reason)),
			zap.String("request_phase", string(failoverErr.RequestPhase)),
		)
	}
	reqLog.Info(logPrefix+".attempt_policy_dry_run", fields...)
}

func grokMediaAttemptPolicyEndpoint(endpoint service.GrokMediaEndpoint) attemptpolicy.EndpointKind {
	switch {
	case endpoint.IsVideoLookupRequest():
		return attemptpolicy.EndpointVideoLookup
	case endpoint == service.GrokMediaEndpointVideosGenerations:
		return attemptpolicy.EndpointVideoCreate
	default:
		return attemptpolicy.EndpointImageSync
	}
}

// attemptPolicyEndpointForResponses distinguishes an explicit image-generation
// tool request from ordinary text. The image-tool path can recover a rejected
// cipher by replaying with a repaired body, while plain text may not be safe to
// rebuild when remote history is referenced.
func attemptPolicyEndpointForResponses(c *gin.Context, body []byte) attemptpolicy.EndpointKind {
	if c == nil {
		return attemptpolicy.EndpointText
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/images/") {
		if service.OpenAIAsyncImagesEndpointFromContext(c.Request.Context()) {
			return attemptpolicy.EndpointImageAsync
		}
		return attemptpolicy.EndpointImageSync
	}
	if service.OpenAIImagesEndpointFromContext(c.Request.Context()) {
		if service.OpenAIAsyncImagesEndpointFromContext(c.Request.Context()) {
			return attemptpolicy.EndpointImageAsync
		}
		return attemptpolicy.EndpointImageSync
	}
	if service.IsExplicitImageGenerationIntent(c.Request.URL.Path, "", body) {
		return attemptpolicy.EndpointResponsesImageTool
	}
	return attemptpolicy.EndpointText
}

// mediaAttemptAction is the handler-facing subset of an attempt-policy decision.
// Keeping the execution loop behind this seam makes timeout, cancellation, and
// switch budgets testable without a real upstream.
type mediaAttemptAction struct {
	Decision      attemptpolicy.Decision
	RepairAllowed bool
	Stop          bool
	Ambiguous     bool
}

func decideMediaAttempt(
	endpoint attemptpolicy.EndpointKind,
	failoverErr *service.UpstreamFailoverError,
	requestSent, responseStarted bool,
	sameAccountRepairs int,
) mediaAttemptAction {
	decision := service.ObserveAttemptPolicy(endpoint, failoverErr, requestSent, responseStarted, sameAccountRepairs)
	action := mediaAttemptAction{Decision: decision}
	switch decision.Action {
	case attemptpolicy.ActionRepairSameAccount:
		action.RepairAllowed = true
	case attemptpolicy.ActionStopRequest:
		action.Stop = true
	case attemptpolicy.ActionStopAmbiguous:
		action.Stop = true
		action.Ambiguous = true
	case attemptpolicy.ActionCancel:
		action.Stop = true
	}
	return action
}

// applyMediaPolicyClientResponse copies a policy-defined client status/message
// onto the failover error. Provider-specific messages already present win, so
// the policy never hides a more precise upstream explanation.
func applyMediaPolicyClientResponse(failoverErr *service.UpstreamFailoverError, decision attemptpolicy.Decision) {
	if failoverErr == nil {
		return
	}
	if failoverErr.ClientStatusCode <= 0 && decision.ClientStatus > 0 {
		failoverErr.ClientStatusCode = decision.ClientStatus
	}
	if strings.TrimSpace(failoverErr.ClientMessage) == "" && strings.TrimSpace(decision.ClientMessage) != "" {
		failoverErr.ClientMessage = decision.ClientMessage
	}
}

// waitForMediaRepair waits for the policy-provided delay. It returns false when
// the request context is canceled so callers stop without writing a failover
// response for a client that is already gone.
func waitForMediaRepair(ctx context.Context, delay time.Duration) bool {
	if ctx == nil {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	return sleepWithContext(ctx, delay)
}
