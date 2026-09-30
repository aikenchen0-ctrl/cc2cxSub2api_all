package main

import (
	"fmt"
	"strings"
)

// AgentHomeSettings is tenant-owned presentation configuration. It never
// changes Sub2API's settings and contains no upstream credentials.
type AgentHomeSettings struct {
	SiteSubtitle       string `json:"site_subtitle"`
	CompactHomeEnabled bool   `json:"compact_home_enabled"`
	HomeContent        string `json:"home_content"`
}

func validateAgentHomeSettings(home AgentHomeSettings) error {
	if len([]rune(home.SiteSubtitle)) > 500 {
		return fmt.Errorf("site_subtitle must be at most 500 characters")
	}
	for _, r := range home.SiteSubtitle {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' || r == 0x7f {
			return fmt.Errorf("site_subtitle contains a control character")
		}
	}
	content := strings.TrimSpace(home.HomeContent)
	if len([]rune(content)) > 100000 || strings.ContainsRune(content, '\x00') {
		return fmt.Errorf("home_content must be at most 100000 characters and contain no null bytes")
	}
	// HTML is displayed in a sandboxed iframe by AgentAPI's UI, preserving
	// full-document styles without granting tenant content application access.
	if content == "" || strings.HasPrefix(content, "<") {
		return nil
	}
	if err := validateBrandDocURL(content); err != nil {
		return fmt.Errorf("home_content must be HTML or an http/https URL without credentials")
	}
	return nil
}

func agentBrandingData(agent AgentView) map[string]any {
	return map[string]any{
		"name": agent.Name, "site_name": agent.SiteName, "site_logo": agent.SiteLogo,
		"doc_url": agent.DocURL, "contact_info": agent.ContactInfo,
		"site_subtitle": agent.SiteSubtitle, "compact_home_enabled": agent.CompactHomeEnabled,
		"home_content": agent.HomeContent,
	}
}
