package service

import "testing"

// DetectModelPlatformForCrossGroup 决定跨分组 Super Key（卫星代调）请求落到哪个
// 调度平台。它必须把 Grok / 国产供应商模型映射到对应平台，否则请求会被钉死在
// OpenAI 平台，grok-* / deepseek-* 等模型永远选不到号
// （日志："no available OpenAI accounts supporting model: grok-4.7"）。
func TestDetectModelPlatformForCrossGroup(t *testing.T) {
	cases := []struct {
		model    string
		platform string
		ok       bool
	}{
		{model: "grok-4.7", platform: PlatformGrok, ok: true},
		{model: "grok-4.6", platform: PlatformGrok, ok: true},
		{model: "grok-imagine-image-1.5", platform: PlatformGrok, ok: true},
		{model: "deepseek-v4-flash", platform: PlatformDeepseek, ok: true},
		{model: "deepseek-chat", platform: PlatformDeepseek, ok: true},
		{model: "kimi-k3", platform: PlatformKimi, ok: true},
		{model: "glm-5.3", platform: PlatformZhipu, ok: true},
		{model: "minimax-h3", platform: PlatformMiniMax, ok: true},
		{model: "gpt-6-sol", platform: PlatformOpenAI, ok: true},
		{model: "gpt-5.6-terra", platform: PlatformOpenAI, ok: true},
		{model: "gpt-image-2", platform: PlatformOpenAI, ok: true},
		// 非 OpenAI 兼容调度平台：保持 false，让调用方沿用既有默认。
		{model: "claude-sonnet-5", ok: false},
		{model: "gemini-3.1-pro-high", ok: false},
		{model: "llama-4-maverick", ok: false},
		{model: "", ok: false},
	}

	for _, tc := range cases {
		platform, ok := DetectModelPlatformForCrossGroup(tc.model)
		if ok != tc.ok || platform != tc.platform {
			t.Fatalf("model=%q: got (%q,%v), want (%q,%v)", tc.model, platform, ok, tc.platform, tc.ok)
		}
	}
}
