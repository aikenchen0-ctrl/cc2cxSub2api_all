package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
)

var agentBindingCodePattern = regexp.MustCompile(`^[0-9]{6}$`)

var errInvalidAgentIdentityBindings = errors.New("invalid upstream identity bindings")

var agentBindingProviders = []string{"email", "linuxdo", "oidc", "wechat", "dingtalk"}

type agentIdentityBinding struct {
	Provider    string `json:"provider"`
	Bound       bool   `json:"bound"`
	BoundCount  int    `json:"bound_count"`
	CanBind     bool   `json:"can_bind"`
	CanUnbind   bool   `json:"can_unbind"`
	DisplayName string `json:"display_name,omitempty"`
	SubjectHint string `json:"subject_hint,omitempty"`
	Note        string `json:"note,omitempty"`
}

type agentIdentityBindingsResponse struct {
	Items                 []agentIdentityBinding `json:"items"`
	CanEdit               bool                   `json:"can_edit"`
	OAuthBindingSupported bool                   `json:"oauth_binding_supported"`
	Profile               map[string]any         `json:"profile,omitempty"`
}

type agentOAuthBindingStartResponse struct {
	Provider     string `json:"provider"`
	AuthorizeURL string `json:"authorize_url"`
	Method       string `json:"method"`
}

func safeAgentIdentityBindings(raw []byte, mainUserID string, includeProfile bool) (agentIdentityBindingsResponse, error) {
	var profile map[string]json.RawMessage
	if err := json.Unmarshal(raw, &profile); err != nil {
		return agentIdentityBindingsResponse{}, err
	}
	bindings := map[string]json.RawMessage{}
	for _, key := range []string{"auth_bindings", "identity_bindings", "identities"} {
		if len(bindings) > 0 {
			break
		}
		_ = json.Unmarshal(profile[key], &bindings)
	}
	items := make([]agentIdentityBinding, 0, len(agentBindingProviders))
	for _, provider := range agentBindingProviders {
		item := agentIdentityBinding{Provider: provider}
		rawBinding := bindings[provider]
		var boolean bool
		if json.Unmarshal(rawBinding, &boolean) == nil {
			item.Bound = boolean
			if boolean {
				item.BoundCount = 1
			}
		} else {
			var upstream struct {
				Bound       bool   `json:"bound"`
				BoundCount  int    `json:"bound_count"`
				CanBind     bool   `json:"can_bind"`
				CanUnbind   bool   `json:"can_unbind"`
				DisplayName string `json:"display_name"`
				SubjectHint string `json:"subject_hint"`
				Note        string `json:"note"`
			}
			if len(rawBinding) > 0 && json.Unmarshal(rawBinding, &upstream) != nil {
				return agentIdentityBindingsResponse{}, errInvalidAgentIdentityBindings
			}
			item.Bound = upstream.Bound
			item.BoundCount = upstream.BoundCount
			item.CanBind = upstream.CanBind
			item.CanUnbind = upstream.CanUnbind
			item.DisplayName = cleanAgentBindingText(upstream.DisplayName, 320)
			item.SubjectHint = cleanAgentBindingText(upstream.SubjectHint, 320)
			item.Note = cleanAgentBindingText(upstream.Note, 512)
		}
		items = append(items, item)
	}
	result := agentIdentityBindingsResponse{Items: items, CanEdit: true, OAuthBindingSupported: true}
	if includeProfile {
		result.Profile = safeAgentProfile(raw, mainUserID, true)
	}
	return result, nil
}

func cleanAgentBindingText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > limit || strings.ContainsAny(value, "\r\n\x00") {
		return ""
	}
	return value
}

func validAgentBindingEmail(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 320 || strings.ContainsAny(value, "\r\n\x00") {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && strings.EqualFold(address.Address, value)
}

func (s *Server) handleAgentProfileBindings(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	tenant, session, _, ok := s.requireTenantSession(w, r, requestID)
	if !ok {
		return
	}
	if strings.TrimSpace(session.AccessToken) == "" {
		s.writeError(w, http.StatusForbidden, requestID, "MAIN_SESSION_REQUIRED", "sign in with your main-site account to manage sign-in methods")
		return
	}
	if r.Method != http.MethodGet && !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agent/profile/bindings")
	operation := ""
	var sendCodePayload struct {
		Email string `json:"email"`
	}
	var bindEmailPayload struct {
		Email      string `json:"email"`
		VerifyCode string `json:"verify_code"`
		Password   string `json:"password"`
	}
	provider := ""
	switch {
	case r.Method == http.MethodGet && path == "":
		operation = "list"
	case r.Method == http.MethodPost && path == "/email/send-code":
		if decodeJSON(r, &sendCodePayload, 16<<10) != nil || !validAgentBindingEmail(sendCodePayload.Email) {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_EMAIL", "a valid email address is required")
			return
		}
		operation = "send_email_code"
	case r.Method == http.MethodPost && path == "/email":
		if decodeJSON(r, &bindEmailPayload, 16<<10) != nil || !validAgentBindingEmail(bindEmailPayload.Email) || !agentBindingCodePattern.MatchString(bindEmailPayload.VerifyCode) || bindEmailPayload.Password == "" || len(bindEmailPayload.Password) > 1024 || strings.ContainsAny(bindEmailPayload.Password, "\x00") {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_BINDING_REQUEST", "email, six-digit verification code and current password are required")
			return
		}
		operation = "bind_email"
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/start"):
		provider = strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/start")
		if provider != "linuxdo" && provider != "oidc" && provider != "wechat" && provider != "dingtalk" {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PROVIDER", "unsupported identity provider")
			return
		}
		operation = "start_oauth_binding"
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/"):
		provider = strings.TrimPrefix(path, "/")
		if provider == "email" || (provider != "linuxdo" && provider != "oidc" && provider != "wechat" && provider != "dingtalk") {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PROVIDER", "unsupported identity provider")
			return
		}
		operation = "unbind"
	default:
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	if _, err := s.currentSessionUser(r.Context(), session); err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	refreshed, err := s.store.LoadSession(session.ID)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "AgentAPI session is no longer valid")
		return
	}

	switch operation {
	case "list":
		raw, upstreamErr := s.main.UserProfile(r.Context(), refreshed.AccessToken)
		if upstreamErr != nil {
			s.writeMainError(w, requestID, upstreamErr)
			return
		}
		result, parseErr := safeAgentIdentityBindings(raw, session.MainUserID, false)
		if parseErr != nil {
			s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_PROFILE_INVALID", "main-site identity bindings are invalid")
			return
		}
		s.writeData(w, http.StatusOK, requestID, result)
	case "send_email_code":
		if _, upstreamErr := s.main.UserSendEmailBindingCode(r.Context(), refreshed.AccessToken, strings.TrimSpace(sendCodePayload.Email), r.Header.Get("Accept-Language")); upstreamErr != nil {
			s.writeMainError(w, requestID, upstreamErr)
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"success": true})
	case "bind_email":
		raw, upstreamErr := s.main.UserBindEmailIdentity(r.Context(), refreshed.AccessToken, strings.TrimSpace(bindEmailPayload.Email), bindEmailPayload.VerifyCode, bindEmailPayload.Password)
		if upstreamErr != nil {
			s.writeMainError(w, requestID, upstreamErr)
			return
		}
		result, parseErr := safeAgentIdentityBindings(raw, session.MainUserID, true)
		if parseErr != nil {
			s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_PROFILE_INVALID", "main-site identity bindings are invalid")
			return
		}
		s.recordTenantAudit(tenant.AgentID, "user", session.MainUserID, "profile_email_binding_update", "user", session.MainUserID, requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, result)
	case "start_oauth_binding":
		start, upstreamErr := s.main.SatelliteOAuthBindingStart(r.Context(), session.MainUserID, provider)
		if upstreamErr != nil {
			s.writeMainError(w, requestID, upstreamErr)
			return
		}
		authorizeURL, urlErr := s.absoluteMainOAuthBindingURL(start.AuthorizeURL)
		if urlErr != nil {
			s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_BINDING_START_INVALID", "main-site binding start returned an invalid URL")
			return
		}
		s.writeData(w, http.StatusOK, requestID, agentOAuthBindingStartResponse{
			Provider: provider, AuthorizeURL: authorizeURL, Method: http.MethodGet,
		})
	case "unbind":
		if _, upstreamErr := s.main.UserUnbindIdentity(r.Context(), refreshed.AccessToken, provider); upstreamErr != nil {
			s.writeMainError(w, requestID, upstreamErr)
			return
		}
		_ = s.store.DeleteSession(session.ID)
		s.clearSessionCookie(w)
		s.recordTenantAudit(tenant.AgentID, "user", session.MainUserID, "profile_identity_unbind", "identity_provider", provider, requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, map[string]any{"success": true, "reauthenticate": true})
	}
}

func (s *Server) absoluteMainOAuthBindingURL(raw string) (string, error) {
	base, ok := publicMainOrigin(s.cfg.PublicMainURL)
	if !ok {
		return "", errors.New("public main origin is not configured")
	}
	relative, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || relative.IsAbs() || relative.Host != "" || relative.User != nil || relative.Fragment != "" || !strings.HasPrefix(relative.Path, "/api/v1/auth/oauth/") || !strings.HasSuffix(relative.Path, "/bind/start") {
		return "", errors.New("invalid oauth binding URL")
	}
	base.Path = relative.Path
	base.RawQuery = relative.RawQuery
	return base.String(), nil
}
