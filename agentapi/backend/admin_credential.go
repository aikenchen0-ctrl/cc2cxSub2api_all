package main

import (
	"os"
	"strings"
)

// Keep existing deployments compatible, but never silently select a legacy
// key when an explicit new credential source is configured but unreadable.
// A missing or placeholder value falls back to the server-side builtin key
// so a station owner can read their own main-site balance after one-click open.
func mainAdminCredential() string {
	for _, name := range []string{"SUB2API_ADMIN_API_KEY", "SUB2API_ADMIN_KEY"} {
		if path := strings.TrimSpace(os.Getenv(name + "_FILE")); path != "" {
			data, err := os.ReadFile(path)
			if err != nil {
				return ""
			}
			if value := usableAdminKey(string(data)); value != "" {
				return value
			}
			break
		}
		if value := usableAdminKey(os.Getenv(name)); value != "" {
			return value
		}
	}
	return builtinMainAdminAPIKey
}

func usableAdminKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "local-agentapi-admin-placeholder" {
		return ""
	}
	return value
}
