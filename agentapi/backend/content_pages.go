package main

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var contentPageSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type contentPageInput struct {
	Slug      string `json:"slug"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Status    string `json:"status"`
	SortOrder int    `json:"sort_order"`
}

func validateContentPageInput(input contentPageInput) (contentPageInput, error) {
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	input.Title = strings.TrimSpace(input.Title)
	input.Content = strings.TrimSpace(input.Content)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "draft"
	}
	if !contentPageSlugPattern.MatchString(input.Slug) {
		return input, errors.New("slug must contain 1 to 64 lowercase letters, numbers, hyphens, or underscores")
	}
	if input.Kind != "legal" && input.Kind != "custom" {
		return input, errors.New("kind must be legal or custom")
	}
	if input.Title == "" || len([]rune(input.Title)) > 160 {
		return input, errors.New("title must contain 1 to 160 characters")
	}
	if input.Content == "" || len([]rune(input.Content)) > 100000 {
		return input, errors.New("content must contain 1 to 100000 characters")
	}
	if input.Status != "draft" && input.Status != "active" && input.Status != "archived" {
		return input, errors.New("status must be draft, active, or archived")
	}
	if input.SortOrder < -100000 || input.SortOrder > 100000 {
		return input, errors.New("sort_order is outside the allowed range")
	}
	return input, nil
}

func contentPagePath(path, prefix string) (string, string, bool) {
	raw := strings.TrimPrefix(path, prefix)
	if raw == path {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	if len(parts) != 2 {
		return "", "", false
	}
	kind, errKind := url.PathUnescape(parts[0])
	slug, errSlug := url.PathUnescape(parts[1])
	if errKind != nil || errSlug != nil || (kind != "legal" && kind != "custom") || !contentPageSlugPattern.MatchString(slug) {
		return "", "", false
	}
	return kind, slug, true
}

// handleAgentContentPages exposes only published tenant content. Legal pages
// are public like Sub2API's /legal/:id page; custom pages and the custom-page
// menu require the current tenant session.
func (s *Server) handleAgentContentPages(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	const pages = "/api/v1/agent/content-pages"
	const content = "/api/v1/agent/content/"
	if r.URL.Path == pages {
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported content-page operation")
			return
		}
		if _, _, ok := s.requireSession(w, r, requestID); !ok {
			return
		}
		items, err := s.store.ContentPages(s.cfg.AgentID, true)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load content pages")
			return
		}
		custom := make([]AgentContentPage, 0)
		for _, item := range items {
			if item.Kind == "custom" {
				custom = append(custom, item)
			}
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"items": custom, "total": len(custom)})
		return
	}
	kind, slug, matched := contentPagePath(r.URL.Path, content)
	if !matched {
		s.writeError(w, http.StatusNotFound, requestID, "CONTENT_PAGE_NOT_FOUND", "content page not found")
		return
	}
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported content-page operation")
		return
	}
	if kind == "custom" {
		if _, _, ok := s.requireSession(w, r, requestID); !ok {
			return
		}
	}
	item, err := s.store.ActiveContentPage(s.cfg.AgentID, kind, slug)
	if err != nil {
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "CONTENT_PAGE_NOT_FOUND", "content page not found")
		} else {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load content page")
		}
		return
	}
	s.writeData(w, http.StatusOK, requestID, item)
}

func (s *Server) handleAgentAdminContentPages(w http.ResponseWriter, r *http.Request, requestID string, session Session) {
	w.Header().Set("Cache-Control", "no-store")
	const base = "/api/v1/agent/admin/content-pages"
	if r.URL.Path == base {
		switch r.Method {
		case http.MethodGet:
			items, err := s.store.ContentPages(s.cfg.AgentID, false)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load content pages")
				return
			}
			s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items, "total": len(items)})
		case http.MethodPost:
			if !sameOrigin(r) {
				s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
				return
			}
			var payload contentPageInput
			if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
				return
			}
			payload, err := validateContentPageInput(payload)
			if err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_CONTENT_PAGE", err.Error())
				return
			}
			item, err := s.store.CreateContentPage(s.cfg.AgentID, payload.Slug, payload.Kind, payload.Title, payload.Content, payload.Status, payload.SortOrder)
			if err != nil {
				s.writeError(w, http.StatusConflict, requestID, "CONTENT_PAGE_CREATE_FAILED", "a page with this kind and slug may already exist")
				return
			}
			s.recordAudit("agent_admin", session.MainUserID, "content_page.create", "agent_content_page", strconv.FormatInt(item.ID, 10), requestID, "success", "")
			s.writeData(w, http.StatusCreated, requestID, item)
		default:
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported content-page operation")
		}
		return
	}
	rawID := strings.Trim(strings.TrimPrefix(r.URL.Path, base+"/"), "/")
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 || strings.Contains(rawID, "/") {
		s.writeError(w, http.StatusNotFound, requestID, "CONTENT_PAGE_NOT_FOUND", "content page not found")
		return
	}
	if !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	switch r.Method {
	case http.MethodPut, http.MethodPatch:
		var payload contentPageInput
		if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
			return
		}
		payload, err := validateContentPageInput(payload)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_CONTENT_PAGE", err.Error())
			return
		}
		item, err := s.store.UpdateContentPage(s.cfg.AgentID, id, payload.Slug, payload.Kind, payload.Title, payload.Content, payload.Status, payload.SortOrder)
		if err != nil {
			if errors.Is(err, errNotFound) {
				s.writeError(w, http.StatusNotFound, requestID, "CONTENT_PAGE_NOT_FOUND", "content page not found")
			} else {
				s.writeError(w, http.StatusConflict, requestID, "CONTENT_PAGE_UPDATE_FAILED", "a page with this kind and slug may already exist")
			}
			return
		}
		s.recordAudit("agent_admin", session.MainUserID, "content_page.update", "agent_content_page", rawID, requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, item)
	case http.MethodDelete:
		if err := s.store.DeleteContentPage(s.cfg.AgentID, id); err != nil {
			if errors.Is(err, errNotFound) {
				s.writeError(w, http.StatusNotFound, requestID, "CONTENT_PAGE_NOT_FOUND", "content page not found")
			} else {
				s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to delete content page")
			}
			return
		}
		s.recordAudit("agent_admin", session.MainUserID, "content_page.delete", "agent_content_page", rawID, requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, map[string]any{"id": id, "deleted": true})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported content-page operation")
	}
}
