package main

import (
	"encoding/base64"
	"net/http"
	"strings"
)

const maxAgentAvatarUploadBytes = 100 * 1024

var allowedAgentAvatarMediaTypes = map[string]struct{}{
	"image/gif":  {},
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

func validateAgentAvatarUpload(raw string) bool {
	if raw == "" {
		return true
	}
	if strings.ContainsAny(raw, "\r\n\x00") {
		return false
	}
	header, encoded, ok := strings.Cut(raw, ",")
	if !ok || encoded == "" {
		return false
	}
	header = strings.ToLower(strings.TrimSpace(header))
	if !strings.HasPrefix(header, "data:") || !strings.HasSuffix(header, ";base64") {
		return false
	}
	mediaType := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64"))
	if _, ok := allowedAgentAvatarMediaTypes[mediaType]; !ok {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	return err == nil && len(decoded) > 0 && len(decoded) <= maxAgentAvatarUploadBytes
}

func (s *Server) handleAgentProfileAvatar(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	tenant, session, _, ok := s.requireTenantSession(w, r, requestID)
	if !ok {
		return
	}
	if strings.TrimSpace(session.AccessToken) == "" {
		s.writeError(w, http.StatusForbidden, requestID, "MAIN_SESSION_REQUIRED", "sign in with your main-site account to edit your avatar")
		return
	}
	var payload struct {
		AvatarURL *string `json:"avatar_url"`
	}
	if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
		return
	}
	if payload.AvatarURL == nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_AVATAR", "avatar_url is required")
		return
	}
	avatarURL := strings.TrimSpace(*payload.AvatarURL)
	if !validateAgentAvatarUpload(avatarURL) {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_AVATAR", "avatar must be a supported inline image no larger than 100KB")
		return
	}
	if _, err := s.currentSessionUser(r.Context(), session); err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	refreshedSession, err := s.store.LoadSession(session.ID)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "AgentAPI session is no longer valid")
		return
	}
	profile, err := s.main.UpdateUserAvatar(r.Context(), refreshedSession.AccessToken, avatarURL)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	s.recordTenantAudit(tenant.AgentID, "user", session.MainUserID, "profile_avatar_update", "user", session.MainUserID, requestID, "success", "")
	s.writeData(w, http.StatusOK, requestID, safeAgentProfile(profile, session.MainUserID, true))
}
