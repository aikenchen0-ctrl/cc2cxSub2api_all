package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 覆盖真实 legacy 选号流程，保留资格过滤与原等待计划，仅替代仓储和并发缓存。
func TestFastFailoverStickyCapacitySpillover(t *testing.T) {
	for _, batch := range []bool{false, true} {
		mode := "without_load_batch"
		if batch {
			mode = "with_load_batch"
		}
		for _, scenario := range []string{"idle_backup", "all_full", "excluded_backup", "wrong_group", "unsupported_model", "model_cooldown", "unflagged", "oauth"} {
			t.Run(mode+"/"+scenario, func(t *testing.T) {
				groupID := int64(3131)
				accounts := []Account{
					{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
						Schedulable: true, Concurrency: 6, Priority: 1, GroupIDs: []int64{groupID},
						Credentials: map[string]any{"fast_failover": true}},
					{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
						Schedulable: true, Concurrency: 6, Priority: 2, GroupIDs: []int64{groupID}},
				}
				excluded := map[int64]struct{}{}
				acquire := map[int64]bool{1: false, 2: true}
				switch scenario {
				case "all_full":
					acquire[2] = false
				case "excluded_backup":
					excluded[2] = struct{}{}
				case "wrong_group":
					accounts[1].GroupIDs = []int64{9999}
				case "unsupported_model":
					accounts[1].Credentials = map[string]any{"model_mapping": map[string]any{"other-model": "other-model"}}
				case "model_cooldown":
					accounts[1].Extra = map[string]any{"model_rate_limits": map[string]any{
						"test-model": map[string]any{"rate_limit_reset_at": time.Now().Add(time.Minute).UTC().Format(time.RFC3339)},
					}}
				case "unflagged":
					accounts[0].Credentials["fast_failover"] = false
				case "oauth":
					accounts[0].Type = AccountTypeOAuth
				}
				session := "capacity-test"
				cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:" + session: 1}}
				load := map[int64]*AccountLoadInfo{
					1: {AccountID: 1, LoadRate: 100, CurrentConcurrency: 6},
					2: {AccountID: 2, LoadRate: 0},
				}
				if scenario == "all_full" {
					load[2].LoadRate = 100
					load[2].CurrentConcurrency = 6
				}
				var acquired []int64
				slots := schedulerTestConcurrencyCache{acquireResults: acquire, waitCounts: map[int64]int{1: 0}, loadMap: load, acquiredIDs: &acquired}
				cfg := &config.Config{RunMode: config.RunModeStandard}
				cfg.Gateway.Scheduling.LoadBatchEnabled = batch
				cfg.Gateway.Scheduling.StickySessionMaxWaiting = 3
				cfg.Gateway.Scheduling.StickySessionWaitTimeout = 30 * time.Second
				cfg.Gateway.Scheduling.FallbackWaitTimeout = 30 * time.Second
				gateway := &OpenAIGatewayService{cfg: cfg, cache: cache,
					accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}},
					concurrencyService: NewConcurrencyService(slots)}
				selection, err := gateway.SelectAccountWithLoadAwareness(context.Background(), &groupID, session, "test-model", excluded)
				require.NoError(t, err)
				require.NotNil(t, selection)
				if scenario == "idle_backup" {
					require.Equal(t, int64(2), selection.Account.ID)
					require.True(t, selection.Acquired)
					require.Nil(t, selection.WaitPlan)
					if selection.ReleaseFunc != nil {
						selection.ReleaseFunc()
					}
				} else {
					require.Equal(t, int64(1), selection.Account.ID)
					require.False(t, selection.Acquired)
					require.NotNil(t, selection.WaitPlan)
					require.Equal(t, 30*time.Second, selection.WaitPlan.Timeout)
					if scenario != "all_full" {
						require.NotContains(t, acquired, int64(2), "不合资格的备用不得抢槽，非快速账号沿用原排队策略")
					}
				}
				require.Equal(t, int64(1), cache.sessionBindings["openai:"+session], "临时分流不应改写长期会话绑定")
			})
		}
	}
}
