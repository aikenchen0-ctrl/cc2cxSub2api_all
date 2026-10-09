package service

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnalyzeOpenAIContinuationSafety(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		canReplay bool
		canStrip  bool
	}{
		{name: "plain_text", body: `{"input":"hello"}`, canReplay: true},
		{name: "plain_history", body: `{"input":[{"role":"user","content":"remember alpha"},{"role":"assistant","content":"saved"},{"role":"user","content":"repeat"}]}`, canReplay: true},
		{name: "previous_response_delta", body: `{"previous_response_id":"resp_old","input":"repeat"}`},
		{name: "previous_response_with_apparent_history", body: `{"previous_response_id":"resp_old","input":[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"},{"role":"user","content":"repeat"}]}`},
		{name: "duplicate_previous_response", body: `{"previous_response_id":"","previous_response_id":"resp_old","input":"hello"}`},
		{name: "duplicate_input", body: `{"input":"hello","input":[{"type":"compaction","encrypted_content":"opaque"}]}`},
		{name: "duplicate_conversation", body: `{"conversation":null,"conversation":"conv_old","input":"hello"}`},
		{name: "invalid_previous_response_type", body: `{"previous_response_id":false,"input":"hello"}`},
		{name: "reasoning_with_plain_history", body: `{"input":[{"type":"message","role":"user","content":"hello"},{"type":"reasoning","encrypted_content":"opaque"},{"type":"message","role":"assistant","content":"hi"}]}`, canReplay: true, canStrip: true},
		{name: "reasoning_without_history", body: `{"input":[{"type":"reasoning","encrypted_content":"opaque"}]}`},
		{name: "reasoning_object_without_history", body: `{"input":{"type":"reasoning","encrypted_content":"opaque"}}`},
		{name: "compaction_only", body: `{"input":[{"type":"compaction","encrypted_content":"opaque"}]}`},
		{name: "compaction_with_new_user_message", body: `{"input":[{"type":"compaction","encrypted_content":"opaque"},{"role":"user","content":"repeat"}]}`},
		{name: "compaction_summary", body: `{"input":{"type":"compaction_summary","encrypted_content":"opaque"}}`},
		{name: "item_reference", body: `{"input":[{"type":"item_reference","id":"item_old"},{"role":"user","content":"repeat"}]}`},
		{name: "tool_pair", body: `{"input":[{"type":"function_call","call_id":"call_a","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call_a","output":"ok"}]}`, canReplay: true},
		{name: "tool_pair_with_reasoning", body: `{"input":[{"type":"reasoning","encrypted_content":"opaque"},{"type":"function_call","call_id":"call_a","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call_a","output":"ok"}]}`, canReplay: true, canStrip: true},
		{name: "tool_output_only", body: `{"input":[{"type":"function_call_output","call_id":"call_a","output":"ok"}]}`},
		{name: "tool_reference_only", body: `{"input":[{"type":"item_reference","id":"call_a"},{"type":"function_call_output","call_id":"call_a","output":"ok"}]}`},
		{name: "remote_file", body: `{"input":[{"role":"user","content":[{"type":"input_file","file_id":"file_old"}]}]}`},
		{name: "inline_file", body: `{"input":[{"role":"user","content":[{"type":"input_file","file_data":"data:application/pdf;base64,AA=="}]}]}`, canReplay: true},
		{name: "conversation_reference", body: `{"conversation":"conv_old","input":"hello"}`},
		{name: "conversation_object", body: `{"conversation":{"id":"conv_old"},"input":"hello"}`},
		{name: "null_conversation", body: `{"conversation":null,"input":"hello"}`, canReplay: true},
		{name: "unknown_encrypted_item", body: `{"input":[{"type":"vendor_state","encrypted_content":"opaque"},{"role":"user","content":"hello"}]}`},
		{name: "invalid_json", body: `{"input":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			original := bytes.Clone(body)
			got := AnalyzeOpenAIContinuationSafety(body)
			require.Equal(t, tc.canReplay, got.CanReplayAcrossAccounts)
			require.Equal(t, tc.canStrip, got.CanStripEncryptedReasoning)
			require.Equal(t, original, body)
		})
	}
}

func TestAnalyzeOpenAIContinuationSafetyReportsState(t *testing.T) {
	got := AnalyzeOpenAIContinuationSafety([]byte(`{"previous_response_id":" resp_old ","input":[{"type":"reasoning","encrypted_content":"r"},{"type":"compaction","encrypted_content":"c"},{"type":"item_reference","id":"item_old"},{"role":"user","content":"hello"}]}`))
	require.Equal(t, "resp_old", got.PreviousResponseID)
	require.True(t, got.HasOpaqueCompaction)
	require.True(t, got.HasRemoteReferences)
	require.True(t, got.HasEncryptedReasoning)
	require.True(t, got.HasPlaintextInput)
	require.False(t, got.CanReplayAcrossAccounts)
}

func TestOpenAIContinuationSafetyRejectedReasoningRecovery(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"previous_response_id":"resp_old","input":[{"type":"reasoning","encrypted_content":"r"}]}`, true},
		{`{"input":[{"type":"reasoning","encrypted_content":"r"},{"type":"input_text","text":"hello"}]}`, true},
		{`{"input":[{"type":"reasoning","encrypted_content":"r"}]}`, false},
		{`{"input":[{"type":"reasoning","encrypted_content":"r"},{"type":"function_call_output","call_id":"call_missing","output":"done"}]}`, false},
		{`{"previous_response_id":"resp_old","input":[{"type":"reasoning","encrypted_content":"r"},{"type":"compaction","encrypted_content":"c"}]}`, false},
	} {
		require.Equal(t, tc.want, AnalyzeOpenAIContinuationSafety([]byte(tc.body)).CanRecoverRejectedEncryptedReasoning())
	}
}

func TestOpenAIWSReplayHistoryRequiresKnownParent(t *testing.T) {
	payload := []byte(`{"previous_response_id":"resp_parent","input":"delta"}`)
	replay := []json.RawMessage{
		json.RawMessage(`{"role":"user","content":"remember alpha"}`),
		json.RawMessage(`{"role":"assistant","content":"saved alpha"}`),
		json.RawMessage(`{"role":"user","content":"delta"}`),
	}
	require.False(t, openAIWSReplayHistoryComplete(payload, replay, true, false, "resp_parent"))
	require.False(t, openAIWSReplayHistoryComplete(payload, replay, true, true, "resp_other"))
	require.True(t, openAIWSReplayHistoryComplete(payload, replay, true, true, "resp_parent"))
	require.True(t, openAIWSReplayHistoryComplete([]byte(`{"input":"hello"}`), nil, false, false, ""))
	replay = append(replay, json.RawMessage(`{"type":"compaction","encrypted_content":"opaque"}`))
	require.False(t, openAIWSReplayHistoryComplete(payload, replay, true, true, "resp_parent"))
}

func TestOpenAIWSReplayHistoryRebuildsObservedToolCallWithoutParentID(t *testing.T) {
	payload := []byte(`{"input":{"type":"custom_tool_call_output","call_id":"call_a","output":"done"}}`)
	replay := []json.RawMessage{
		json.RawMessage(`{"role":"user","content":"run command"}`),
		json.RawMessage(`{"type":"custom_tool_call","call_id":"call_a","name":"exec","input":"pwd"}`),
		json.RawMessage(`{"type":"custom_tool_call_output","call_id":"call_a","output":"done"}`),
	}
	require.True(t, openAIWSReplayHistoryComplete(payload, replay, true, true, "resp_parent"))
	require.False(t, openAIWSReplayHistoryComplete(payload, replay, true, false, "resp_parent"))
	require.False(t, openAIWSReplayHistoryComplete(payload, replay[2:], true, true, "resp_parent"))
	replay = append(replay, json.RawMessage(`{"type":"compaction","encrypted_content":"opaque"}`))
	require.False(t, openAIWSReplayHistoryComplete(payload, replay, true, true, "resp_parent"))
}

func TestOpenAIWSCurrentTurnRetryRejectsOpaqueState(t *testing.T) {
	for _, item := range []string{
		`{"type":"compaction","encrypted_content":"opaque"}`,
		`{"type":"item_reference","id":"item_remote"}`,
		`{"type":"function_call_output","call_id":"call_missing","output":"done"}`,
	} {
		replay := []json.RawMessage{
			json.RawMessage(`{"role":"user","content":"hello"}`),
			json.RawMessage(item),
		}
		_, safe, err := buildOpenAIWSCurrentTurnRetryPayload(
			[]byte(`{"previous_response_id":"resp_parent","input":"delta"}`), replay, true, "gpt-5.1")
		require.NoError(t, err)
		require.False(t, safe)
	}
}
