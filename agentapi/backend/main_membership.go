package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

// EnsureAgentTenantMember records a station-domain registration on the main
// site. The application credential stays server-side; the main site accepts
// only a member role and never lets AgentAPI change a tenant owner.
func (c *MainClient) EnsureAgentTenantMember(ctx context.Context, agentID, mainUserID string) error {
	agentID = strings.TrimSpace(agentID)
	mainUserID = strings.TrimSpace(mainUserID)
	userID, err := strconv.ParseInt(mainUserID, 10, 64)
	if !validSharedAgentID(agentID) || err != nil || userID <= 0 {
		return &MainAPIError{Status: http.StatusBadRequest, Code: "INVALID_MEMBERSHIP", Message: "invalid AgentAPI tenant membership"}
	}
	path := "/auth/satellite/" + c.cfg.SatelliteSlug + "/agent-membership"
	_, err = c.satelliteAuthJSON(ctx, path, map[string]any{
		"agent_id": agentID, "main_user_id": userID,
	}, "", "AGENT_MEMBERSHIP_UNAVAILABLE")
	return err
}
