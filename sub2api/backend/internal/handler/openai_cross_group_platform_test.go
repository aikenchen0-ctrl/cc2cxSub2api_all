package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 跨分组 Super Key（卫星代调 / 管理密钥）没有绑定分组，平台必须按请求模型推导，
// 而不是一律回落 openai。否则 grok-* / deepseek-* 等模型永远选不到号。
func TestOpenAICompatibleRequestPlatformCrossGroupUsesModel(t *testing.T) {
	newCtx := func(model string) context.Context {
		ctx := context.WithValue(context.Background(), ctxkey.SuperAPIKey, true)
		if model != "" {
			ctx = context.WithValue(ctx, ctxkey.Model, model)
		}
		return ctx
	}

	// 无绑定分组的 Super Key：按模型推导平台。
	require.Equal(t, service.PlatformGrok, openAICompatibleRequestPlatform(newCtx("grok-4.7"), &service.APIKey{}))
	require.Equal(t, service.PlatformDeepseek, openAICompatibleRequestPlatform(newCtx("deepseek-v4-flash"), &service.APIKey{}))
	require.Equal(t, service.PlatformKimi, openAICompatibleRequestPlatform(newCtx("kimi-k3"), &service.APIKey{}))
	require.Equal(t, service.PlatformOpenAI, openAICompatibleRequestPlatform(newCtx("gpt-6-sol"), &service.APIKey{}))
	// 未知模型保持 openai 默认。
	require.Equal(t, service.PlatformOpenAI, openAICompatibleRequestPlatform(newCtx("llama-4-maverick"), &service.APIKey{}))

	// 非跨分组请求不受影响：仍按分组平台。
	boundCtx := context.WithValue(context.Background(), ctxkey.Model, "grok-4.7")
	bound := &service.APIKey{Group: &service.Group{Platform: service.PlatformOpenAI}}
	require.Equal(t, service.PlatformOpenAI, openAICompatibleRequestPlatform(boundCtx, bound))
}

// 已解析出目标平台（composite 分组）时，推导结果优先，不能被跨分组逻辑覆盖。
func TestOpenAICompatibleRequestPlatformPrefersResolvedTargetPlatform(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.SuperAPIKey, true)
	ctx = context.WithValue(ctx, ctxkey.Model, "grok-4.7")
	ctx = service.WithResolvedTargetPlatform(ctx, service.PlatformDeepseek)
	require.Equal(t, service.PlatformDeepseek, openAICompatibleRequestPlatform(ctx, &service.APIKey{}))
}
