package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIWSNextAttemptMessageUsesCurrentTurnPayload(t *testing.T) {
	firstMessage := []byte(`{"type":"response.create","input":"first"}`)
	currentTurn := []byte(`{"type":"response.create","input":"turn-281"}`)

	next, ok := openAIWSNextAttemptMessage(firstMessage, currentTurn, true)

	require.True(t, ok)
	require.Equal(t, currentTurn, next)
	next[0] = 'x'
	require.Equal(t, byte('{'), currentTurn[0], "retry payload must be cloned")
}

func TestOpenAIWSNextAttemptMessageRejectsMissingCurrentTurnPayload(t *testing.T) {
	next, ok := openAIWSNextAttemptMessage([]byte(`{"type":"response.create"}`), nil, true)

	require.False(t, ok)
	require.Nil(t, next)
}

func TestOpenAIWSNextAttemptMessageKeepsInitialMessageForFirstTurnFailover(t *testing.T) {
	firstMessage := []byte(`{"type":"response.create","input":"first"}`)

	next, ok := openAIWSNextAttemptMessage(firstMessage, nil, false)

	require.True(t, ok)
	require.Equal(t, firstMessage, next)
}

func TestOpenAIWSNextAttemptMessageRejectsUnreconstructableCurrentTurn(t *testing.T) {
	for _, retry := range []string{
		`{"type":"response.create","previous_response_id":"resp_old","input":"delta"}`,
		`{"type":"response.create","input":[{"type":"compaction","encrypted_content":"opaque"},{"role":"user","content":"delta"}]}`,
		`{"type":"response.create","input":[{"type":"item_reference","id":"call_old"},{"type":"function_call_output","call_id":"call_old","output":"done"}]}`,
	} {
		next, ok := openAIWSNextAttemptMessage([]byte(`{"type":"response.create","input":"first"}`), []byte(retry), true)
		require.False(t, ok)
		require.Nil(t, next)
	}
}
