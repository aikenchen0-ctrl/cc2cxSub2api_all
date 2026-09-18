package handler

import (
	"net/url"
	"os"
	"strings"
)

// projectLink resolves the deployment-specific public URL for a sibling app.
// Both PROJECT_LINK and project_link are accepted to match existing .env files.
func projectLink(project, fallback string) string {
	for _, key := range []string{strings.ToUpper(project) + "_LINK", strings.ToLower(project) + "_link"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			if !strings.Contains(value, "://") {
				value = "https://" + value
			}
			if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
				return strings.TrimRight(value, "/")
			}
		}
	}
	return fallback
}
