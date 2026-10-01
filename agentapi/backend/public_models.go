package main

import "net/http"

// handlePublicModels exposes the same tenant-filtered discovery catalog used by
// authenticated /v1/models without accepting a user identity or contacting the
// main site's complete model inventory. Model execution remains authenticated.
func (s *Server) handlePublicModels(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "public model catalog supports GET only")
		return
	}
	if _, err := s.requestAgent(r); err != nil {
		s.writeError(w, http.StatusMisdirectedRequest, requestID, "TENANT_NOT_FOUND", "request host is not assigned to an agent")
		return
	}
	s.writePublicModels(w)
}
