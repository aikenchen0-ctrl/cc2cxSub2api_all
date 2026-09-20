package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func TestSuperKeyCanUseActiveGroupWithoutUserBinding(t *testing.T) {
	group := &Group{Status: StatusActive, IsExclusive: true, SubscriptionType: SubscriptionTypeSubscription}
	if !superKeyCanUseGroup(group) {
		t.Fatal("expected super key to use any active group without user binding")
	}
}

func TestSuperKeyCannotUseInactiveGroup(t *testing.T) {
	if superKeyCanUseGroup(&Group{Status: StatusDisabled}) {
		t.Fatal("inactive groups must remain unavailable")
	}
}

func TestSuperKeyCanScheduleAccountRequiresAnActiveGroup(t *testing.T) {
	if !superKeyCanScheduleAccount(&Account{ID: 1}) {
		t.Fatal("ungrouped accounts should remain available")
	}
	if superKeyCanScheduleAccount(&Account{
		ID:       2,
		GroupIDs: []int64{20},
		Groups:   []*Group{{ID: 20, Status: StatusDisabled}},
	}) {
		t.Fatal("accounts attached only to inactive groups must be rejected")
	}
	if !superKeyCanScheduleAccount(&Account{
		ID:       3,
		GroupIDs: []int64{21, 22},
		Groups:   []*Group{{ID: 21, Status: StatusDisabled}, {ID: 22, Status: StatusActive}},
	}) {
		t.Fatal("an active group should keep a multi-group account available")
	}
}

func TestMediaModerationSkippedOnlyForSuperKey(t *testing.T) {
	super := &APIKey{Key: SuperAPIKeyPrefix + "test", Name: SuperAPIKeyName}
	ordinary := &APIKey{Key: "sk-ordinary", Name: "ordinary"}
	if ShouldRunMediaModeration(super) {
		t.Fatal("super key media requests must bypass moderation")
	}
	if !ShouldRunMediaModeration(ordinary) {
		t.Fatal("ordinary key media requests must still be moderated")
	}
}

func TestSuperKeyBypassesAPIKeyIPRestrictions(t *testing.T) {
	super := &APIKey{Key: SuperAPIKeyPrefix + "test", Name: SuperAPIKeyName}
	ordinary := &APIKey{Key: "sk-ordinary", Name: "ordinary"}
	if ShouldEnforceAPIKeyIPRestrictions(super) {
		t.Fatal("super key IP restrictions must be bypassed")
	}
	if !ShouldEnforceAPIKeyIPRestrictions(ordinary) {
		t.Fatal("ordinary key IP restrictions must remain enforced")
	}
}

func TestSuperKeyBypassesMediaConcurrency(t *testing.T) {
	super := &APIKey{Key: SuperAPIKeyPrefix + "test", Name: SuperAPIKeyName}
	ordinary := &APIKey{Key: "sk-ordinary", Name: "ordinary"}
	if ShouldEnforceMediaConcurrency(super) {
		t.Fatal("super key media concurrency must be bypassed")
	}
	if !ShouldEnforceMediaConcurrency(ordinary) {
		t.Fatal("ordinary key media concurrency must remain enforced")
	}
}

func TestSuperKeyCrossGroup(t *testing.T) {
	if SuperKeyCrossGroup(context.Background()) {
		t.Fatal("ordinary requests must stay inside their group")
	}
	ctx := context.WithValue(context.Background(), ctxkey.SuperAPIKey, true)
	if !SuperKeyCrossGroup(ctx) {
		t.Fatal("satellite Super Key requests must schedule across groups")
	}
}

func TestListSchedulableAccountsSuperKeyIncludesGroupedOpenAIAccounts(t *testing.T) {
	grouped := Account{ID: 11, Platform: PlatformOpenAI, GroupIDs: []int64{9}, AccountGroups: []AccountGroup{{GroupID: 9}}}
	ungrouped := Account{ID: 10, Platform: PlatformOpenAI}
	svc := &OpenAIGatewayService{accountRepo: groupedAwareOpenAIAccountRepo{grouped: grouped, ungrouped: ungrouped}}

	plain, err := svc.listSchedulableAccounts(context.Background(), nil, PlatformOpenAI)
	if err != nil {
		t.Fatalf("ungrouped list: %v", err)
	}
	if len(plain) != 1 || plain[0].ID != ungrouped.ID {
		t.Fatalf("nil group without Super Key must stay ungrouped, got %#v", plain)
	}

	ctx := context.WithValue(context.Background(), ctxkey.SuperAPIKey, true)
	cross, err := svc.listSchedulableAccounts(ctx, nil, PlatformOpenAI)
	if err != nil {
		t.Fatalf("super key list: %v", err)
	}
	if len(cross) != 2 {
		t.Fatalf("satellite Super Key must see grouped OpenAI accounts, got %#v", cross)
	}
}

type groupedAwareOpenAIAccountRepo struct {
	stubOpenAIAccountRepo
	grouped   Account
	ungrouped Account
}

func (r groupedAwareOpenAIAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	if platform != PlatformOpenAI {
		return nil, nil
	}
	return []Account{r.ungrouped, r.grouped}, nil
}

func (r groupedAwareOpenAIAccountRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]Account, error) {
	if platform != PlatformOpenAI {
		return nil, nil
	}
	return []Account{r.ungrouped}, nil
}
