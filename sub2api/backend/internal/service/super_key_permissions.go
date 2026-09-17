package service

// superKeyCanUseGroup keeps Super Keys independent from user group bindings
// while preserving the safety requirement that disabled groups are unusable.
func superKeyCanUseGroup(group *Group) bool {
	return group != nil && group.IsActive()
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
