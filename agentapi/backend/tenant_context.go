package main

import (
	"errors"
	"net"
	"net/http"
	"strings"
)

const (
	tenantRoleOwner  = "owner"
	tenantRoleMember = "member"

	tenantSourceSession = "session"
	tenantSourceAPIKey  = "api_key"
	// Legacy sessions are accepted only when an operator explicitly configured
	// AGENT_ID for a single-tenant migration. Shared production has no fallback.
	tenantSourceLegacy = "legacy"
)

// TenantContext is the only request-level source of tenant identity. It must
// be derived from a server-issued session or a stored AgentAPI API key, never
// from a browser-supplied query parameter or header.
type TenantContext struct {
	AgentID    string
	MainUserID string
	Role       string
	Source     string
}

func normalizeTenantRole(role string) string {
	if strings.EqualFold(strings.TrimSpace(role), tenantRoleOwner) {
		return tenantRoleOwner
	}
	return tenantRoleMember
}

func (s *Server) legacyTenantRole(mainUserID string) string {
	if ownerID := strings.TrimSpace(s.cfg.OwnerMainUserID); ownerID != "" && strings.TrimSpace(mainUserID) == ownerID {
		return tenantRoleOwner
	}
	return tenantRoleMember
}

func (s *Server) tenantRole(agent AgentView, mainUserID string) string {
	if ownerID := strings.TrimSpace(agent.OwnerMainUserID); ownerID != "" && strings.TrimSpace(mainUserID) == ownerID {
		return tenantRoleOwner
	}
	return tenantRoleMember
}

func normalizeTenantHost(rawHost string) (string, bool) {
	value := strings.TrimSpace(strings.ToLower(rawHost))
	if value == "" || strings.ContainsAny(value, "/\\?#") {
		return "", false
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	} else if strings.Count(value, ":") == 1 {
		return "", false
	}
	value = strings.TrimSuffix(strings.Trim(value, "[]"), ".")
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return "", false
	}
	return value, true
}

// isSharedHost reports whether a request arrived through the common AgentAPI
// entry point. A shared host is never a tenant's custom domain, even if an old
// single-tenant database row still contains the same host from before the
// shared-runtime migration.
func (s *Server) isSharedHost(rawHost string) bool {
	host, ok := normalizeTenantHost(rawHost)
	if !ok {
		return false
	}
	for _, configured := range s.cfg.SharedHosts {
		if sharedHost, valid := normalizeTenantHost(configured); valid && host == sharedHost {
			return true
		}
	}
	return false
}

// requestAgent resolves a request to a tenant using only server-bound state:
// an AgentAPI session or an exact persisted host mapping. An explicitly
// configured AGENT_ID may provide single-tenant migration compatibility, but
// shared production has no process-wide fallback. Once a process serves
// multiple active tenants, an unbound request is ambiguous and must fail.
func (s *Server) requestAgent(r *http.Request) (AgentView, error) {
	var sessionAgentID string
	if tenant, _, _, ok := s.loadTenantSession(r); ok {
		sessionAgentID = tenant.AgentID
	}

	host, hostOK := normalizeTenantHost(r.Host)
	if hostOK && !s.isSharedHost(r.Host) {
		if agent, err := s.store.AgentByDomain(host); err == nil {
			if sessionAgentID != "" && sessionAgentID != agent.ID {
				return AgentView{}, errNotFound
			}
			return agent, nil
		} else if !errors.Is(err, errNotFound) {
			return AgentView{}, err
		}
	}
	if sessionAgentID != "" {
		return s.store.Agent(sessionAgentID)
	}

	compatibilityAgentID := strings.TrimSpace(s.cfg.AgentID)
	configuredHost, configured := normalizeTenantHost(s.cfg.AgentDomain)
	if compatibilityAgentID != "" && hostOK && configured && host == configuredHost {
		return s.store.Agent(s.cfg.AgentID)
	}
	if compatibilityAgentID != "" && strings.TrimSpace(s.cfg.AgentDomain) == "" {
		agents, err := s.store.ActiveAgents()
		if err != nil {
			return AgentView{}, err
		}
		// A disabled legacy single-tenant site must still serve its public
		// settings so the UI can explain that it is unavailable. With no active
		// tenant there is no competing destination.
		if len(agents) == 0 {
			return s.store.Agent(s.cfg.AgentID)
		}
		if len(agents) == 1 && agents[0].ID == strings.TrimSpace(s.cfg.AgentID) {
			return agents[0], nil
		}
	}
	return AgentView{}, errNotFound
}

func (s *Server) tenantContextFromSession(session Session) (TenantContext, bool) {
	agentID := strings.TrimSpace(session.AgentID)
	source := tenantSourceSession
	if agentID == "" {
		// Old cookies are usable only during an explicit AGENT_ID migration.
		// With no compatibility tenant configured, shared production rejects
		// them rather than assigning a tenant implicitly.
		agentID = strings.TrimSpace(s.cfg.AgentID)
		if agentID == "" {
			return TenantContext{}, false
		}
		source = tenantSourceLegacy
	}
	mainUserID := strings.TrimSpace(session.MainUserID)
	if agentID == "" || mainUserID == "" {
		return TenantContext{}, false
	}
	role := strings.TrimSpace(session.Role)
	if source == tenantSourceLegacy || role == "" {
		role = s.legacyTenantRole(mainUserID)
	}
	return TenantContext{AgentID: agentID, MainUserID: mainUserID, Role: normalizeTenantRole(role), Source: source}, true
}
