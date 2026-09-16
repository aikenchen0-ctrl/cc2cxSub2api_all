package antigravity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiUsageMapping_NoCacheCreationTokens(t *testing.T) {
	const body = `{"candidates":[{"content":{"parts":[{"text":"hi"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"cachedContentTokenCount":30,"thoughtsTokenCount":5}}`
	_, usage, err := TransformGeminiToClaude([]byte(body), "gemini-3.1-pro-preview")
	require.NoError(t, err)
	require.Equal(t, 70, usage.InputTokens)
	require.Equal(t, 25, usage.OutputTokens)
	require.Equal(t, 30, usage.CacheReadInputTokens)
	require.Zero(t, usage.CacheCreationInputTokens)

	p := NewStreamingProcessor("gemini-3.1-pro-preview")
	out := p.ProcessLine(`data: {"response":` + body + `}`)
	require.Contains(t, string(out), `"message_start"`)
	require.NotContains(t, string(out), `"cache_creation_input_tokens"`)
	_, streamUsage := p.Finish()
	require.Equal(t, 70, streamUsage.InputTokens)
	require.Equal(t, 30, streamUsage.CacheReadInputTokens)
	require.Zero(t, streamUsage.CacheCreationInputTokens)
	require.True(t, strings.Contains(string(out), `"input_tokens"`))
}
