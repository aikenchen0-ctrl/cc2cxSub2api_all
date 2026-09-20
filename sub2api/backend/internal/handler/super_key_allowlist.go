package handler

import "github.com/Wei-Shaw/sub2api/internal/service"

func apiKeyEnforcesModelAllowlist(apiKey *service.APIKey) bool {
	return apiKey != nil && !apiKey.IsSuper() && apiKey.Group != nil && apiKey.Group.ModelAllowlistEnabled()
}
