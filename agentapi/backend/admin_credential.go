package main

import (
	"os"
	"strings"
)

// Keep existing deployments compatible, but never silently select a legacy
// key when an explicit new credential source is configured but unreadable.
func mainAdminCredential() string {
	for _, name := range []string{"SUB2API_ADMIN_API_KEY", "SUB2API_ADMIN_KEY"} {
		if path := strings.TrimSpace(os.Getenv(name + "_FILE")); path != "" {
			data, err := os.ReadFile(path)
			if err != nil {
				return ""
			}
			return strings.TrimSpace(string(data))
		}
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}
