package service

import (
	"bytes"
	"strings"

	"github.com/tidwall/gjson"
)

// OpenAIToolContinuationCannotSwitchMessage is returned when a function_call_output
// turn cannot migrate to another local account.
const OpenAIToolContinuationCannotSwitchMessage = "function_call_output continuation cannot switch accounts"

// OpenAIContinuation is the request-scoped continuation policy. HTTP and WS
// handlers share it so account-switch rules stay in one place.
type OpenAIContinuation struct {
	PreviousResponseID string
	CanMove            bool
	hasEncrypted       bool
}

func NewOpenAIContinuation(body []byte) OpenAIContinuation {
	coverage := AnalyzeToolCallOutputContextCoverageBytes(body)
	return OpenAIContinuation{
		PreviousResponseID: strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String()),
		CanMove:            !coverage.HasFunctionCallOutput || coverage.ContextCoversAllCallIDs,
		hasEncrypted:       OpenAIBodyHasEncryptedContinuation(body),
	}
}

// BlockAccountSwitch is true when this turn is pinned to the previous_response
// account (incomplete tool chain) but scheduling did not hit that account.
func (c OpenAIContinuation) BlockAccountSwitch(stickyPreviousHit bool) bool {
	return c.PreviousResponseID != "" && !c.CanMove && !stickyPreviousHit
}

// OpenAIContinuationStripHint is the scheduling outcome the continuation policy
// needs. Handlers pass the decision; they do not interpret encrypted blobs.
type OpenAIContinuationStripHint struct {
	StickyPreviousHit bool
	StickySessionHit  bool
	FailedOver        bool
}

func NewOpenAIContinuationStripHint(decision OpenAIAccountScheduleDecision, failedOver bool) OpenAIContinuationStripHint {
	return OpenAIContinuationStripHint{
		StickyPreviousHit: decision.StickyPreviousHit,
		StickySessionHit:  decision.StickySessionHit,
		FailedOver:        failedOver,
	}
}

// ShouldStrip reports whether opaque continuation state must be removed before
// forwarding this attempt. Account-id changes are handled inside
// OpenAIForwardAttemptState.Prepare so callers do not track lastAccountID.
func (c OpenAIContinuation) ShouldStrip(hint OpenAIContinuationStripHint) bool {
	if hint.FailedOver {
		return true
	}
	if c.PreviousResponseID != "" && !hint.StickyPreviousHit {
		return true
	}
	if !hint.StickyPreviousHit && !hint.StickySessionHit && c.hasEncrypted {
		return true
	}
	return false
}

// OpenAIForwardAttemptState derives each forward body from an immutable canonical
// payload. It owns passthrough-boundary sanitization and local-account switches.
type OpenAIForwardAttemptState struct {
	passthroughSeen bool
	lastAccountID   int64
}

// Prepare returns the body for this account attempt. canonical is never mutated.
func (s *OpenAIForwardAttemptState) Prepare(canonical []byte, account *Account, stripContinuation bool) (body []byte, stripped bool) {
	if s == nil || account == nil {
		return canonical, false
	}
	currentPassthrough := account.IsOpenAIPassthroughEnabled()
	if currentPassthrough {
		s.passthroughSeen = true
	}
	crossMode := s.passthroughSeen && !currentPassthrough
	switchedAccount := s.lastAccountID != 0 && s.lastAccountID != account.ID
	strip := crossMode || stripContinuation || switchedAccount
	s.lastAccountID = account.ID
	if !strip {
		return canonical, false
	}
	sanitized, changed, err := SanitizeOpenAISwitchedAccountContinuation(canonical)
	if err != nil || !changed {
		return canonical, false
	}
	return sanitized, true
}

// SanitizeOpenAISwitchedAccountContinuation drops previous_response_id and
// encrypted reasoning/compaction items so a different local account can continue
// from plaintext input. Thinking summaries and chat messages are kept.
func SanitizeOpenAISwitchedAccountContinuation(body []byte) (sanitized []byte, changed bool, err error) {
	if len(body) == 0 {
		return body, false, nil
	}
	stripped := RemovePreviousResponseIDFromBody(body)
	idChanged := !bytes.Equal(stripped, body)
	reasoning, reasoningChanged, reasoningErr := SanitizeOpenAICrossModeFailoverReasoning(stripped)
	if reasoningErr != nil {
		return body, false, reasoningErr
	}
	if reasoningChanged {
		return reasoning, true, nil
	}
	if idChanged {
		return stripped, true, nil
	}
	return body, false, nil
}

// OpenAIBodyHasEncryptedContinuation reports whether input carries opaque
// encrypted reasoning/compaction blobs that cannot move across accounts.
func OpenAIBodyHasEncryptedContinuation(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	found := false
	gjson.GetBytes(body, "input").ForEach(func(_, item gjson.Result) bool {
		if item.Get("encrypted_content").Exists() {
			found = true
			return false
		}
		return true
	})
	return found
}
