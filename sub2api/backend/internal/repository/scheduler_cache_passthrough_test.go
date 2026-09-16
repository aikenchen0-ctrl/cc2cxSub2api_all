package repository

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterSchedulerExtraPreservesOpenAIPassthroughFlags(t *testing.T) {
	input := map[string]any{
		"openai_passthrough":       true,
		"openai_oauth_passthrough": true,
		"model_mapping":            map[string]any{"only-listed": "only-listed"},
		"credentials":              "must-not-be-projected",
	}

	projected := filterSchedulerExtra(input)
	raw, err := json.Marshal(projected)
	require.NoError(t, err)
	var roundTripped map[string]any
	require.NoError(t, json.Unmarshal(raw, &roundTripped))

	require.Equal(t, true, roundTripped["openai_passthrough"])
	require.Equal(t, true, roundTripped["openai_oauth_passthrough"])
	require.NotContains(t, roundTripped, "credentials")
}
