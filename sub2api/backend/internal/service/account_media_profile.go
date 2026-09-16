package service

import "strings"

const (
	MediaProtocolProfileExtraKey         = "media_protocol_profile"
	MediaProtocolProfileOpenAICompatible = "openai_compatible"
)

// MediaProtocolProfile is opt-in so existing accounts keep their current
// routing, forwarding and billing behavior after upgrades.
func (a *Account) MediaProtocolProfile() (string, bool) {
	if a == nil {
		return "", false
	}
	profile := strings.ToLower(strings.TrimSpace(a.GetExtraString(MediaProtocolProfileExtraKey)))
	if profile != MediaProtocolProfileOpenAICompatible {
		return "", false
	}
	return profile, true
}
