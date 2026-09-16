//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIsOpenAIErrorChannelEligible(t *testing.T) {
	t.Run("error but still enabled openai account is eligible", func(t *testing.T) {
		account := &Account{Platform: PlatformOpenAI, Status: StatusError, Schedulable: true}
		require.False(t, account.IsSchedulable())
		require.True(t, account.IsOpenAIErrorChannelEligible())
		require.True(t, account.IsSchedulableForModel("gpt-5.6-sol"))
	})

	t.Run("user-disabled error account stays out", func(t *testing.T) {
		account := &Account{Platform: PlatformOpenAI, Status: StatusError, Schedulable: false}
		require.False(t, account.IsOpenAIErrorChannelEligible())
		require.False(t, account.IsSchedulableForModel("gpt-5.6-sol"))
	})

	t.Run("anthropic error account is not auto-recovered", func(t *testing.T) {
		account := &Account{Platform: PlatformAnthropic, Status: StatusError, Schedulable: true}
		require.False(t, account.IsOpenAIErrorChannelEligible())
	})

	t.Run("temp unschedulable window does not block same-group failover", func(t *testing.T) {
		until := time.Now().Add(time.Minute)
		account := &Account{
			Platform:               PlatformOpenAI,
			Status:                 StatusError,
			Schedulable:            true,
			TempUnschedulableUntil: &until,
		}
		require.True(t, account.IsOpenAIErrorChannelEligible())
		require.True(t, account.IsSchedulableForModel("gpt-5.6-sol"))
	})
}

func TestMergeOpenAIAccountsByID(t *testing.T) {
	merged := mergeOpenAIAccountsByID(
		[]Account{{ID: 53, Status: StatusActive}},
		[]Account{{ID: 53, Status: StatusActive}, {ID: 1, Status: StatusError}, {ID: 2, Status: StatusError}},
	)
	require.Equal(t, []int64{53, 1, 2}, []int64{merged[0].ID, merged[1].ID, merged[2].ID})
}
