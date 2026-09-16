package service

import "testing"

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
