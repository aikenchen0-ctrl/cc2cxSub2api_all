package main

import (
	"net/http"
	"strconv"
	"strings"
)

// handleAgentAdminUsers exposes only the users already registered to the
// current tenant. It is intentionally read-only: a site owner can understand
// their audience without receiving platform-wide account authority.
func (s *Server) handleAgentAdminUsers(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "user management is read-only")
		return
	}
	page, pageSize := 1, 20
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PAGE", "page must be a positive integer")
			return
		}
		page = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("page_size")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PAGE_SIZE", "page_size must be between 1 and 100")
			return
		}
		pageSize = value
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len(search) > 160 {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_SEARCH", "search is too long")
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && status != "active" && status != "suspended" {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_STATUS", "status is invalid")
		return
	}
	items, total, err := s.store.UsersPageFiltered(tenant.AgentID, pageSize, (page-1)*pageSize, search, status)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load users")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	s.writeData(w, http.StatusOK, requestID, map[string]any{
		"items": items, "total": total, "page": page, "page_size": pageSize,
	})
}
