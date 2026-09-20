package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// SuperKeyCrossGroup reports whether this request may pick upstream accounts
// from every active group instead of a single bound group.
func SuperKeyCrossGroup(ctx context.Context) bool {
	return ctx != nil && ctx.Value(ctxkey.SuperAPIKey) == true
}

// superKeyCanUseGroup keeps Super Keys independent from user group bindings
// while preserving the safety requirement that disabled groups are unusable.
func superKeyCanUseGroup(group *Group) bool {
	return group != nil && group.IsActive()
}

// superKeyCanScheduleAccount keeps cross-group scheduling limited to accounts
// that are ungrouped or still attached to at least one active group. Account
// snapshots normally hydrate Groups/AccountGroups; when a grouped snapshot has
// no group metadata at all, fail closed so a stale sticky binding cannot revive
// an account whose group was disabled or deleted.
func superKeyCanScheduleAccount(account *Account) bool {
	if account == nil {
		return false
	}
	if len(account.GroupIDs) == 0 && len(account.AccountGroups) == 0 && len(account.Groups) == 0 {
		return true
	}
	for _, group := range account.Groups {
		if superKeyCanUseGroup(group) {
			return true
		}
	}
	for _, membership := range account.AccountGroups {
		if superKeyCanUseGroup(membership.Group) {
			return true
		}
	}
	return false
}

// ShouldRunMediaModeration keeps content moderation for ordinary keys while
// allowing the administrative Super Key to exercise every media capability.
func ShouldRunMediaModeration(apiKey *APIKey) bool {
	return apiKey == nil || !apiKey.IsSuper()
}

// ShouldEnforceAPIKeyIPRestrictions leaves network ACLs in place for regular
// keys but keeps the administrative Super Key unrestricted.
func ShouldEnforceAPIKeyIPRestrictions(apiKey *APIKey) bool {
	return apiKey == nil || !apiKey.IsSuper()
}

// ShouldEnforceMediaConcurrency keeps global/user media slots for ordinary
// keys while allowing the administrative Super Key to use media endpoints
// without those business concurrency caps.
func ShouldEnforceMediaConcurrency(apiKey *APIKey) bool {
	return apiKey == nil || !apiKey.IsSuper()
}
