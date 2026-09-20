package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNewOpenAIContinuation(t *testing.T) {
	plain := NewOpenAIContinuation([]byte(`{"input":[{"type":"message","role":"user","content":"hi"}]}`))
	require.True(t, plain.CanMove)
	require.Empty(t, plain.PreviousResponseID)

	withPrev := NewOpenAIContinuation([]byte(`{"previous_response_id":"resp_1","input":[{"type":"message","content":"hi"}]}`))
	require.True(t, withPrev.CanMove)
	require.Equal(t, "resp_1", withPrev.PreviousResponseID)
	require.False(t, withPrev.BlockAccountSwitch(true))
	require.False(t, withPrev.BlockAccountSwitch(false))
}

func TestOpenAIContinuationBlockAccountSwitch(t *testing.T) {
	body := []byte(`{"previous_response_id":"resp_1","input":[{"type":"function_call_output","call_id":"c1","output":"x"}]}`)
	c := NewOpenAIContinuation(body)
	require.False(t, c.CanMove)
	require.True(t, c.BlockAccountSwitch(false))
	require.False(t, c.BlockAccountSwitch(true))
}

func TestOpenAIContinuationShouldStrip(t *testing.T) {
	canonical := []byte(`{"previous_response_id":"resp_x","input":[{"type":"reasoning","encrypted_content":"x"}]}`)
	c := NewOpenAIContinuation(canonical)
	require.True(t, c.ShouldStrip(OpenAIContinuationStripHint{}))
	require.False(t, c.ShouldStrip(OpenAIContinuationStripHint{StickyPreviousHit: true}))
	require.True(t, c.ShouldStrip(OpenAIContinuationStripHint{StickyPreviousHit: true, FailedOver: true}))

	encryptedOnly := NewOpenAIContinuation([]byte(`{"input":[{"type":"reasoning","encrypted_content":"x"}]}`))
	require.True(t, encryptedOnly.ShouldStrip(OpenAIContinuationStripHint{}))
	require.False(t, encryptedOnly.ShouldStrip(OpenAIContinuationStripHint{StickySessionHit: true}))
}

func TestSanitizeOpenAISwitchedAccountContinuation(t *testing.T) {
	canonical := []byte(`{"model":"gpt-5.1","previous_response_id":"resp_old","input":[` +
		`{"type":"message","role":"user","content":"hello"},` +
		`{"type":"reasoning","id":"rs_1","encrypted_content":"ENC","summary":[{"type":"summary_text","text":"thinking"}]},` +
		`{"type":"message","role":"assistant","content":"hi"}` +
		`]}`)

	out, changed, err := SanitizeOpenAISwitchedAccountContinuation(canonical)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(out, "previous_response_id").Exists())
	require.False(t, gjson.GetBytes(out, "input.#(encrypted_content)").Exists())
	require.Equal(t, int64(2), gjson.GetBytes(out, "input.#").Int())
	require.Equal(t, "hello", gjson.GetBytes(out, "input.0.content").String())
	require.Equal(t, "hi", gjson.GetBytes(out, "input.1.content").String())
	require.True(t, OpenAIBodyHasEncryptedContinuation(canonical))
	require.False(t, OpenAIBodyHasEncryptedContinuation(out))
}

func TestSanitizeOpenAISwitchedAccountContinuation_DropsEncryptedCompaction(t *testing.T) {
	canonical := []byte(`{"previous_response_id":"resp_old","input":[` +
		`{"type":"message","role":"user","content":"hello"},` +
		`{"type":"compaction","encrypted_content":"COMPACT_ENC"},` +
		`{"type":"message","role":"assistant","content":"hi"}` +
		`]}`)
	out, changed, err := SanitizeOpenAISwitchedAccountContinuation(canonical)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(out, "previous_response_id").Exists())
	require.Equal(t, int64(2), gjson.GetBytes(out, "input.#").Int())
	require.Equal(t, "hello", gjson.GetBytes(out, "input.0.content").String())
	require.Equal(t, "hi", gjson.GetBytes(out, "input.1.content").String())
}

func TestStickyAccountNeedsEviction(t *testing.T) {
	until := time.Now().Add(time.Minute)
	manual := &Account{Schedulable: false, Status: StatusActive}
	temp := &Account{Schedulable: true, Status: StatusActive, TempUnschedulableUntil: &until}
	ok := &Account{Schedulable: true, Status: StatusActive}
	req := OpenAIAccountScheduleRequest{}
	require.True(t, stickyAccountNeedsEviction(nil, req))
	require.True(t, stickyAccountNeedsEviction(manual, req))
	require.True(t, stickyAccountNeedsEviction(temp, req))
	require.False(t, stickyAccountNeedsEviction(ok, req))
	require.False(t, stickyAccountNeedsEviction(manual, OpenAIAccountScheduleRequest{ForceStickyAccount: true}))
}
