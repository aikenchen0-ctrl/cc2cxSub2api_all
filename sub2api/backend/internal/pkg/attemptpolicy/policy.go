// Package attemptpolicy contains side-effect-free decisions for upstream
// request attempts. Callers provide observable request state and receive one
// explicit action; this package never performs retries, sleeps, or I/O.
package attemptpolicy

import (
	"net/http"
	"time"
)

// EndpointKind identifies the business semantics of an upstream request.
// The zero value is treated as EndpointText for backward compatibility.
type EndpointKind string

const (
	EndpointText               EndpointKind = "text"
	EndpointResponsesImageTool EndpointKind = "responses_image_tool"
	EndpointImageSync          EndpointKind = "image_sync"
	EndpointImageAsync         EndpointKind = "image_async"
	EndpointVideoCreate        EndpointKind = "video_create"
	EndpointVideoLookup        EndpointKind = "video_lookup"
	EndpointVideoContent       EndpointKind = "video_content"
)

// FailureClass is a stable, loggable classification of an upstream failure.
type FailureClass string

const (
	FailureRequestInvalid       FailureClass = "request_invalid"
	FailureAccountAuth          FailureClass = "account_auth"
	FailureAccountQuota         FailureClass = "account_quota"
	FailureAccountState         FailureClass = "account_state"
	FailureTransientSameAccount FailureClass = "transient_same_account"
	FailureCapacityRequestScope FailureClass = "capacity_request_scoped"
	FailureTransportBeforeSend  FailureClass = "transport_before_send"
	FailureTransportAfterSend   FailureClass = "transport_after_send"
	FailureCipherOrSignature    FailureClass = "cipher_or_signature"
	FailureResponseStarted      FailureClass = "response_started"
	FailureAmbiguousMediaCreate FailureClass = "ambiguous_media_create"
)

// AttemptAction is the only next step a caller may take for an attempt.
type AttemptAction string

const (
	ActionRepairSameAccount AttemptAction = "repair_same_account"
	ActionSwitchAccount     AttemptAction = "switch_account"
	ActionStopRequest       AttemptAction = "stop_request"
	ActionStopAmbiguous     AttemptAction = "stop_ambiguous"
	ActionCancel            AttemptAction = "cancel"
)

const (
	defaultAuthCooldown      = 60 * time.Second
	defaultQuotaCooldown     = 60 * time.Second
	defaultTransientCooldown = 30 * time.Second
	defaultRepairDelay       = 250 * time.Millisecond
	shortRetryAfterLimit     = 2 * time.Second

	textRepairMax        = 3
	mediaRepairMax       = 1
	videoLookupRepairMax = 2
)

// Observation is the request-local state available when classifying a failed
// attempt. It deliberately contains values only, so classification can be
// tested without constructing gateway services or fake HTTP transports.
type Observation struct {
	Endpoint EndpointKind

	StatusCode      int
	RetryAfter      time.Duration
	RetryAfterKnown bool
	QuotaResetAfter time.Duration

	RequestSent       bool
	ResponseStarted   bool
	AmbiguousPossible bool
	CanReplay         bool

	AccountAuthFailure     bool
	AccountQuotaFailure    bool
	AccountStateFailure    bool
	RequestInvalid         bool
	CipherOrSignature      bool
	TransportAfterSend     bool
	RequestScopedTransient bool
	RetryableOnSameAccount bool
	SameAccountRepairs     int
}

// Decision describes the next action and the account-health effects of a
// failure. A non-positive Cooldown means no account cooldown is recorded.
type Decision struct {
	Class           FailureClass
	Action          AttemptAction
	PenalizeAccount bool
	Cooldown        time.Duration
	RepairLimit     int
	RepairDelay     time.Duration
	ClientStatus    int
	ClientMessage   string
}

// QuotaCooldown returns the account cooldown for an account quota failure.
// A known reset time is honored up to the policy cap; otherwise the default
// quota cooldown applies.
func QuotaCooldown(obs Observation) time.Duration {
	if obs.QuotaResetAfter > defaultQuotaCooldown {
		return defaultQuotaCooldown
	}
	if obs.QuotaResetAfter > 0 {
		return obs.QuotaResetAfter
	}
	return defaultQuotaCooldown
}

// Classify maps one observed failure to one action. The order of checks is
// intentional: irreversible output wins over every retry rule, and ambiguous
// video creation wins over ordinary transport recovery.
func Classify(obs Observation) Decision {
	endpoint := normalizeEndpoint(obs.Endpoint)
	limit := RepairLimit(endpoint)

	if obs.ResponseStarted {
		return Decision{
			Class:        FailureResponseStarted,
			Action:       ActionStopRequest,
			RepairLimit:  limit,
			RepairDelay:  defaultRepairDelay,
			ClientStatus: http.StatusBadGateway,
		}
	}

	if endpoint == EndpointVideoCreate && obs.AmbiguousPossible {
		return Decision{
			Class:         FailureAmbiguousMediaCreate,
			Action:        ActionStopAmbiguous,
			RepairLimit:   0,
			RepairDelay:   defaultRepairDelay,
			ClientStatus:  http.StatusBadGateway,
			ClientMessage: "Video creation status is unknown after an upstream connection interruption; check the video list before retrying to avoid a duplicate task",
		}
	}

	if obs.AccountAuthFailure || obs.AccountStateFailure || isAccountHardStatus(obs.StatusCode) {
		class := FailureAccountAuth
		if obs.AccountStateFailure || obs.StatusCode == http.StatusForbidden {
			class = FailureAccountState
		}
		return Decision{
			Class:           class,
			Action:          ActionSwitchAccount,
			PenalizeAccount: true,
			Cooldown:        defaultAuthCooldown,
			RepairLimit:     0,
			RepairDelay:     defaultRepairDelay,
			ClientStatus:    http.StatusBadGateway,
		}
	}

	if obs.AccountQuotaFailure {
		return quotaSwitchDecision(QuotaCooldown(obs))
	}

	if obs.StatusCode == http.StatusTooManyRequests {
		if retryAfterIsShort(obs) && obs.CanReplay {
			return repairDecision(FailureTransientSameAccount, endpoint, obs, false)
		}
		return quotaSwitchDecision(QuotaCooldown(obs))
	}

	if obs.RequestInvalid || isRequestInvalidStatus(obs.StatusCode) {
		if obs.CipherOrSignature && obs.CanReplay && hasRepairBudget(obs, limit) {
			return repairDecision(FailureCipherOrSignature, endpoint, obs, false)
		}
		return Decision{
			Class:        FailureRequestInvalid,
			Action:       ActionStopRequest,
			RepairLimit:  limit,
			RepairDelay:  defaultRepairDelay,
			ClientStatus: http.StatusBadRequest,
		}
	}

	if obs.CipherOrSignature {
		if !obs.CanReplay || !hasRepairBudget(obs, limit) {
			return Decision{
				Class:        FailureCipherOrSignature,
				Action:       ActionStopRequest,
				RepairLimit:  limit,
				RepairDelay:  defaultRepairDelay,
				ClientStatus: http.StatusBadRequest,
			}
		}
		return repairDecision(FailureCipherOrSignature, endpoint, obs, false)
	}

	if obs.TransportAfterSend {
		if endpoint == EndpointVideoCreate {
			return Decision{
				Class:         FailureAmbiguousMediaCreate,
				Action:        ActionStopAmbiguous,
				RepairLimit:   0,
				RepairDelay:   defaultRepairDelay,
				ClientStatus:  http.StatusBadGateway,
				ClientMessage: "Video creation status is unknown after an upstream connection interruption; check the video list before retrying to avoid a duplicate task",
			}
		}
		if !obs.CanReplay {
			return Decision{
				Class:        FailureTransportAfterSend,
				Action:       ActionStopRequest,
				RepairLimit:  limit,
				RepairDelay:  defaultRepairDelay,
				ClientStatus: http.StatusBadGateway,
			}
		}
		return repairOrSwitch(FailureTransportAfterSend, endpoint, obs)
	}

	if isTransientStatus(obs.StatusCode) || obs.RetryableOnSameAccount {
		class := FailureTransientSameAccount
		if obs.RequestScopedTransient {
			class = FailureCapacityRequestScope
		}
		if !obs.CanReplay {
			return Decision{
				Class:        class,
				Action:       ActionStopRequest,
				RepairLimit:  limit,
				RepairDelay:  defaultRepairDelay,
				ClientStatus: http.StatusBadGateway,
			}
		}
		return repairOrSwitch(class, endpoint, obs)
	}

	return Decision{
		Class:        FailureRequestInvalid,
		Action:       ActionStopRequest,
		RepairLimit:  limit,
		RepairDelay:  defaultRepairDelay,
		ClientStatus: http.StatusBadRequest,
	}
}

// RepairLimit returns the maximum same-account repairs for an endpoint.
func RepairLimit(endpoint EndpointKind) int {
	switch normalizeEndpoint(endpoint) {
	case EndpointVideoCreate:
		return 0
	case EndpointVideoLookup, EndpointVideoContent:
		return videoLookupRepairMax
	case EndpointText, EndpointResponsesImageTool:
		return textRepairMax
	default:
		return mediaRepairMax
	}
}

func normalizeEndpoint(endpoint EndpointKind) EndpointKind {
	if endpoint == "" {
		return EndpointText
	}
	return endpoint
}

func repairOrSwitch(class FailureClass, endpoint EndpointKind, obs Observation) Decision {
	limit := RepairLimit(endpoint)
	if hasRepairBudget(obs, limit) {
		return repairDecision(class, endpoint, obs, false)
	}
	penalize := !obs.RequestScopedTransient
	return Decision{
		Class:           class,
		Action:          ActionSwitchAccount,
		PenalizeAccount: penalize,
		Cooldown:        cooldownFor(penalize, defaultTransientCooldown),
		RepairLimit:     limit,
		RepairDelay:     defaultRepairDelay,
		ClientStatus:    http.StatusBadGateway,
	}
}

func repairDecision(class FailureClass, endpoint EndpointKind, obs Observation, penalize bool) Decision {
	return Decision{
		Class:           class,
		Action:          ActionRepairSameAccount,
		PenalizeAccount: penalize,
		RepairLimit:     RepairLimit(endpoint),
		RepairDelay:     defaultRepairDelay,
	}
}

func quotaSwitchDecision(cooldown time.Duration) Decision {
	return Decision{
		Class:           FailureAccountQuota,
		Action:          ActionSwitchAccount,
		PenalizeAccount: true,
		Cooldown:        cooldown,
		RepairLimit:     0,
		RepairDelay:     defaultRepairDelay,
		ClientStatus:    http.StatusTooManyRequests,
	}
}

func hasRepairBudget(obs Observation, limit int) bool {
	if limit <= 0 {
		return false
	}
	return obs.SameAccountRepairs < limit
}

func retryAfterIsShort(obs Observation) bool {
	if !obs.RetryAfterKnown {
		return false
	}
	// Header parsing yields whole seconds; allow a small tolerance so an exact
	// boundary (e.g. Retry-After: 2) is not misclassified by transit time.
	return obs.RetryAfter >= 0 && obs.RetryAfter <= shortRetryAfterLimit+100*time.Millisecond
}

func cooldownFor(penalize bool, cooldown time.Duration) time.Duration {
	if !penalize {
		return 0
	}
	return cooldown
}

func isAccountHardStatus(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden:
		return true
	default:
		return false
	}
}

func isRequestInvalidStatus(status int) bool {
	return status == http.StatusBadRequest || status == http.StatusUnprocessableEntity
}

func isTransientStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooEarly:
		return true
	default:
		return status >= http.StatusInternalServerError && status <= 599
	}
}
