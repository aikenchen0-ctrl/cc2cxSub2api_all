//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRepository_SetModelRateLimit_APIKey429MaximumDeadline(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "apikey-429-max-deadline", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-only-key"},
	})
	base := time.Now().UTC().Truncate(time.Second)
	short, long := base.Add(time.Minute), base.Add(time.Hour)
	deadline := func(model string) string {
		current, err := repo.GetByID(ctx, account.ID)
		require.NoError(t, err)
		limits, ok := current.Extra["model_rate_limits"].(map[string]any)
		require.True(t, ok)
		entry, ok := limits[model].(map[string]any)
		require.True(t, ok)
		value, ok := entry["rate_limit_reset_at"].(string)
		require.True(t, ok)
		return value
	}
	// 无论较短响应何时返回，都不能覆盖已经写入的更长冷却。
	require.NoError(t, repo.SetModelRateLimit(ctx, account.ID, "gpt-6.1-sol", long, "openai_apikey_429"))
	require.NoError(t, repo.SetModelRateLimit(ctx, account.ID, "gpt-6.1-sol", short, "openai_apikey_429"))
	require.Equal(t, long.Format(time.RFC3339), deadline("gpt-6.1-sol"))
	require.NoError(t, repo.SetModelRateLimit(ctx, account.ID, "gpt-6.1-sol", long.Add(time.Hour), "openai_apikey_429"))
	require.Equal(t, long.Add(time.Hour).Format(time.RFC3339), deadline("gpt-6.1-sol"))
	// 其他模型独立保存；其他原因仍保留既有覆盖语义。
	require.NoError(t, repo.SetModelRateLimit(ctx, account.ID, "gpt-6-astra", short, "openai_apikey_429"))
	require.Equal(t, short.Format(time.RFC3339), deadline("gpt-6-astra"))
	require.NoError(t, repo.SetModelRateLimit(ctx, account.ID, "gpt-6.1-sol", short, "model_not_found"))
	require.Equal(t, short.Format(time.RFC3339), deadline("gpt-6.1-sol"))
}
