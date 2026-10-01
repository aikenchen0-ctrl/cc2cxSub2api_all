package main

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type announcementInput struct {
	Title      string `json:"title"`
	Content    string `json:"content"`
	Status     string `json:"status"`
	NotifyMode string `json:"notify_mode"`
	StartsAt   string `json:"starts_at"`
	EndsAt     string `json:"ends_at"`
}

func parseAnnouncementTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func validateAnnouncementInput(input announcementInput) (announcementInput, time.Time, time.Time, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Content = strings.TrimSpace(input.Content)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.NotifyMode = strings.ToLower(strings.TrimSpace(input.NotifyMode))
	if input.Status == "" {
		input.Status = "draft"
	}
	if input.NotifyMode == "" {
		input.NotifyMode = "silent"
	}
	if input.Title == "" || len([]rune(input.Title)) > 160 {
		return input, time.Time{}, time.Time{}, errors.New("title must contain 1 to 160 characters")
	}
	if input.Content == "" || len([]rune(input.Content)) > 20000 {
		return input, time.Time{}, time.Time{}, errors.New("content must contain 1 to 20000 characters")
	}
	if input.Status != "draft" && input.Status != "active" && input.Status != "archived" {
		return input, time.Time{}, time.Time{}, errors.New("status must be draft, active, or archived")
	}
	if input.NotifyMode != "silent" && input.NotifyMode != "popup" {
		return input, time.Time{}, time.Time{}, errors.New("notify_mode must be silent or popup")
	}
	startsAt, err := parseAnnouncementTime(input.StartsAt)
	if err != nil {
		return input, time.Time{}, time.Time{}, errors.New("starts_at must be an RFC3339 timestamp")
	}
	endsAt, err := parseAnnouncementTime(input.EndsAt)
	if err != nil {
		return input, time.Time{}, time.Time{}, errors.New("ends_at must be an RFC3339 timestamp")
	}
	if !startsAt.IsZero() && !endsAt.IsZero() && !endsAt.After(startsAt) {
		return input, time.Time{}, time.Time{}, errors.New("ends_at must be later than starts_at")
	}
	return input, startsAt, endsAt, nil
}

func announcementPathID(path, prefix, suffix string) (int64, bool) {
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return 0, false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	raw, err := url.PathUnescape(strings.Trim(raw, "/"))
	if err != nil || raw == "" || strings.Contains(raw, "/") {
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0
}

func (s *Server) handleAgentAnnouncements(w http.ResponseWriter, r *http.Request, requestID string) {
	tenant, session, _, ok := s.requireTenantSession(w, r, requestID)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	const base = "/api/v1/agent/announcements"
	if r.URL.Path == base {
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported announcement operation")
			return
		}
		items, err := s.store.UserAnnouncements(tenant.AgentID, session.MainUserID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load announcements")
			return
		}
		unread := 0
		for _, item := range items {
			if item.ReadAt == "" {
				unread++
			}
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items, "total": len(items), "unread": unread})
		return
	}
	if r.URL.Path == base+"/read-all" && r.Method == http.MethodPost {
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		if err := s.store.MarkAllAnnouncementsRead(tenant.AgentID, session.MainUserID); err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to mark announcements as read")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"read": true})
		return
	}
	if id, matched := announcementPathID(r.URL.Path, base+"/", "/read"); matched && r.Method == http.MethodPost {
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		if err := s.store.MarkAnnouncementRead(tenant.AgentID, session.MainUserID, id); err != nil {
			if errors.Is(err, errNotFound) {
				s.writeError(w, http.StatusNotFound, requestID, "ANNOUNCEMENT_NOT_FOUND", "announcement not found")
			} else {
				s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to mark announcement as read")
			}
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"id": id, "read": true})
		return
	}
	s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "announcement route not found")
}

func (s *Server) handleAgentAdminAnnouncements(w http.ResponseWriter, r *http.Request, requestID string, tenant TenantContext, session Session) {
	w.Header().Set("Cache-Control", "no-store")
	const base = "/api/v1/agent/admin/announcements"
	if r.URL.Path == base {
		switch r.Method {
		case http.MethodGet:
			items, err := s.store.AdminAnnouncements(tenant.AgentID)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load announcements")
				return
			}
			s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items, "total": len(items)})
		case http.MethodPost:
			if !sameOrigin(r) {
				s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
				return
			}
			var payload announcementInput
			if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
				return
			}
			payload, startsAt, endsAt, err := validateAnnouncementInput(payload)
			if err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_ANNOUNCEMENT", err.Error())
				return
			}
			item, err := s.store.CreateAnnouncement(tenant.AgentID, payload.Title, payload.Content, payload.Status, payload.NotifyMode, startsAt, endsAt)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to create announcement")
				return
			}
			s.recordTenantAudit(tenant.AgentID, "agent_admin", session.MainUserID, "announcement.create", "agent_announcement", strconv.FormatInt(item.ID, 10), requestID, "success", "")
			s.writeData(w, http.StatusCreated, requestID, item)
		default:
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported announcement operation")
		}
		return
	}
	id, matched := announcementPathID(r.URL.Path, base+"/", "")
	if !matched {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "announcement route not found")
		return
	}
	if !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	switch r.Method {
	case http.MethodPut, http.MethodPatch:
		var payload announcementInput
		if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
			return
		}
		payload, startsAt, endsAt, err := validateAnnouncementInput(payload)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_ANNOUNCEMENT", err.Error())
			return
		}
		item, err := s.store.UpdateAnnouncement(tenant.AgentID, id, payload.Title, payload.Content, payload.Status, payload.NotifyMode, startsAt, endsAt)
		if err != nil {
			if errors.Is(err, errNotFound) {
				s.writeError(w, http.StatusNotFound, requestID, "ANNOUNCEMENT_NOT_FOUND", "announcement not found")
			} else {
				s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to update announcement")
			}
			return
		}
		s.recordTenantAudit(tenant.AgentID, "agent_admin", session.MainUserID, "announcement.update", "agent_announcement", strconv.FormatInt(id, 10), requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, item)
	case http.MethodDelete:
		if err := s.store.DeleteAnnouncement(tenant.AgentID, id); err != nil {
			if errors.Is(err, errNotFound) {
				s.writeError(w, http.StatusNotFound, requestID, "ANNOUNCEMENT_NOT_FOUND", "announcement not found")
			} else {
				s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to delete announcement")
			}
			return
		}
		s.recordTenantAudit(tenant.AgentID, "agent_admin", session.MainUserID, "announcement.delete", "agent_announcement", strconv.FormatInt(id, 10), requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, map[string]any{"id": id, "deleted": true})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported announcement operation")
	}
}
