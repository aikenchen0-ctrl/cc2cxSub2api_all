package main

import (
	"net/http"
	"strings"
	"time"
)

// AgentAdminProvisioningView exposes only this AgentAPI instance's lifecycle
// state. The main-site control-plane credential, deployment paths and other
// agents are deliberately absent from the browser DTO.
type AgentAdminProvisioningView struct {
	AgentID            string `json:"agent_id"`
	Domain             string `json:"domain"`
	DisplayName        string `json:"display_name"`
	SiteName           string `json:"site_name"`
	SiteLogo           string `json:"site_logo,omitempty"`
	OwnerMainUserID    string `json:"owner_main_user_id"`
	ConfiguredStatus   string `json:"configured_status"`
	RuntimeStatus      string `json:"runtime_status"`
	ControlEnabled     bool   `json:"control_enabled"`
	ControlAvailable   bool   `json:"control_available"`
	Ready              bool   `json:"ready"`
	LastCheckedAt      string `json:"last_checked_at,omitempty"`
	StaleAfterSeconds  int64  `json:"stale_after_seconds"`
	BillingMode        string `json:"billing_mode"`
	SatelliteSlug      string `json:"satellite_slug"`
	LifecycleAuthority string `json:"lifecycle_authority"`
	ManagementScope    string `json:"management_scope"`
}

func (s *Server) handleAgentAdminProvisioning(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "agent lifecycle is controlled by the main site")
		return
	}
	agent, err := s.store.Agent(s.cfg.AgentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load agent provisioning status")
		return
	}
	now := s.store.clock().UTC()
	runtimeStatus, checkedAt, available := s.provisioningSnapshot(now)
	staleAfter := s.cfg.ProvisioningControlStaleAfter
	if staleAfter <= 0 {
		staleAfter = 90 * time.Second
	}
	lastCheckedAt := ""
	if !checkedAt.IsZero() {
		lastCheckedAt = checkedAt.UTC().Format(time.RFC3339)
	}
	configuredStatus := strings.TrimSpace(agent.Status)
	if configuredStatus == "" {
		configuredStatus = "unknown"
	}
	view := AgentAdminProvisioningView{
		AgentID:            agent.ID,
		Domain:             agent.Domain,
		DisplayName:        agent.Name,
		SiteName:           agent.SiteName,
		SiteLogo:           agent.SiteLogo,
		OwnerMainUserID:    agent.OwnerMainUserID,
		ConfiguredStatus:   configuredStatus,
		RuntimeStatus:      runtimeStatus,
		ControlEnabled:     s.cfg.ProvisioningControlEnabled,
		ControlAvailable:   available,
		Ready:              available && runtimeStatus == "active" && configuredStatus == "active",
		LastCheckedAt:      lastCheckedAt,
		StaleAfterSeconds:  int64(staleAfter / time.Second),
		BillingMode:        agent.BillingMode,
		SatelliteSlug:      s.cfg.SatelliteSlug,
		LifecycleAuthority: "sub2api_main",
		ManagementScope:    "current_agent_read_only",
	}
	s.writeData(w, http.StatusOK, requestID, view)
}
