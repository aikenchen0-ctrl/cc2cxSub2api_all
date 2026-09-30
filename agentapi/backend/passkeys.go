package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxPasskeyBody = 64 << 10

type agentPasskeyConfig struct {
	Enabled         bool   `json:"enabled"`
	Configured      bool   `json:"configured"`
	SupportedOrigin bool   `json:"supported_origin"`
	RPID            string `json:"rp_id,omitempty"`
}

type upstreamPasskeySettings struct {
	Enabled    bool     `json:"passkey_enabled"`
	Configured bool     `json:"passkey_configured"`
	RPID       string   `json:"passkey_rp_id"`
	Origins    []string `json:"passkey_rp_origins"`
}

type passkeyOptionsView struct {
	SessionToken string          `json:"session_token"`
	Options      json.RawMessage `json:"options"`
}

type passkeyCredentialView struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	Backup     bool       `json:"backup"`
}

type passkeyFinishInput struct {
	SessionToken string          `json:"session_token"`
	Name         string          `json:"name,omitempty"`
	Credential   json.RawMessage `json:"credential"`
}

func (s *Server) handleAgentPasskeyConfig(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	config, err := s.agentPasskeyConfig(r)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	s.writeData(w, http.StatusOK, requestID, config)
}

func (s *Server) authPasskeyLoginBegin(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.requireAgentPasskeyOrigin(w, r, requestID) {
		return
	}
	var input struct {
		TurnstileToken        string `json:"turnstile_token"`
		TencentCaptchaTicket  string `json:"tencent_captcha_ticket"`
		TencentCaptchaRandstr string `json:"tencent_captcha_randstr"`
	}
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &input, 16<<10); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "invalid passkey request")
			return
		}
	}
	data, err := s.main.PasskeyLoginBegin(r.Context(), input)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	view, err := decodePasskeyOptions(data)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_PASSKEY_INVALID", "main site returned invalid passkey options")
		return
	}
	s.writeData(w, http.StatusOK, requestID, view)
}

func (s *Server) authPasskeyLoginFinish(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.requireAgentPasskeyOrigin(w, r, requestID) {
		return
	}
	var input passkeyFinishInput
	if err := decodeJSON(r, &input, maxPasskeyBody); err != nil || strings.TrimSpace(input.SessionToken) == "" || !validPasskeyCredential(input.Credential) {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PASSKEY_RESPONSE", "invalid passkey response")
		return
	}
	auth, err := s.main.PasskeyLoginFinish(r.Context(), map[string]any{
		"session_token": input.SessionToken,
		"credential":    input.Credential,
	})
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	s.establishSession(w, r, requestID, auth, false)
}

func (s *Server) handleAgentPasskeys(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "ORIGIN_REJECTED", "request origin is not allowed")
		return
	}
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	if strings.TrimSpace(session.AccessToken) == "" {
		s.writeError(w, http.StatusForbidden, requestID, "MAIN_USER_TOKEN_REQUIRED", "sign in with your main-site email and password to manage passkeys")
		return
	}
	if !s.requireAgentPasskeyOrigin(w, r, requestID) {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agent/passkeys")
	switch {
	case path == "" && r.Method == http.MethodGet:
		data, err := s.main.Passkeys(r.Context(), session.AccessToken)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		var credentials []passkeyCredentialView
		if err := json.Unmarshal(data, &credentials); err != nil {
			s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_PASSKEY_INVALID", "main site returned invalid passkey credentials")
			return
		}
		s.writeData(w, http.StatusOK, requestID, credentials)
	case path == "/register/begin" && r.Method == http.MethodPost:
		var input struct {
			Password string `json:"password"`
		}
		if decodeJSON(r, &input, 16<<10) != nil || input.Password == "" || len(input.Password) > 1024 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "current password is required")
			return
		}
		data, err := s.main.PasskeyRegisterBegin(r.Context(), session.AccessToken, input)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		view, err := decodePasskeyOptions(data)
		if err != nil {
			s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_PASSKEY_INVALID", "main site returned invalid passkey options")
			return
		}
		s.writeData(w, http.StatusOK, requestID, view)
	case path == "/register/finish" && r.Method == http.MethodPost:
		var input passkeyFinishInput
		if decodeJSON(r, &input, maxPasskeyBody) != nil || strings.TrimSpace(input.SessionToken) == "" || len(strings.TrimSpace(input.Name)) > 100 || !validPasskeyCredential(input.Credential) {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PASSKEY_RESPONSE", "invalid passkey response")
			return
		}
		data, err := s.main.PasskeyRegisterFinish(r.Context(), session.AccessToken, map[string]any{
			"session_token": input.SessionToken,
			"name":          strings.TrimSpace(input.Name),
			"credential":    input.Credential,
		})
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		var credential passkeyCredentialView
		if json.Unmarshal(data, &credential) != nil || credential.ID <= 0 {
			s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_PASSKEY_INVALID", "main site returned an invalid passkey credential")
			return
		}
		s.writeData(w, http.StatusOK, requestID, credential)
	case strings.HasPrefix(path, "/"):
		idText := strings.TrimPrefix(path, "/")
		if strings.Contains(idText, "/") {
			s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "passkey endpoint not found")
			return
		}
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PASSKEY_ID", "invalid passkey ID")
			return
		}
		switch r.Method {
		case http.MethodPatch:
			var input struct {
				Name string `json:"name"`
			}
			if decodeJSON(r, &input, 16<<10) != nil || strings.TrimSpace(input.Name) == "" || len(strings.TrimSpace(input.Name)) > 100 {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "passkey name is required")
				return
			}
			if _, err := s.main.PasskeyRename(r.Context(), session.AccessToken, id, map[string]string{"name": strings.TrimSpace(input.Name)}); err != nil {
				s.writeMainError(w, requestID, err)
				return
			}
			s.writeData(w, http.StatusOK, requestID, map[string]bool{"success": true})
		case http.MethodDelete:
			var input struct {
				Password string `json:"password"`
			}
			if decodeJSON(r, &input, 16<<10) != nil || input.Password == "" || len(input.Password) > 1024 {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "current password is required")
				return
			}
			if _, err := s.main.PasskeyDelete(r.Context(), session.AccessToken, id, input); err != nil {
				s.writeMainError(w, requestID, err)
				return
			}
			s.writeData(w, http.StatusOK, requestID, map[string]bool{"success": true})
		default:
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		}
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "passkey endpoint not found")
	}
}

func (s *Server) requireAgentPasskeyOrigin(w http.ResponseWriter, r *http.Request, requestID string) bool {
	config, err := s.agentPasskeyConfig(r)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return false
	}
	if !config.Enabled {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "PASSKEY_UNAVAILABLE", "passkeys are not configured for this AgentAPI origin")
		return false
	}
	return true
}

func (s *Server) agentPasskeyConfig(r *http.Request) (agentPasskeyConfig, error) {
	raw, err := s.main.PublicSettings(r.Context())
	if err != nil {
		return agentPasskeyConfig{}, err
	}
	var upstream upstreamPasskeySettings
	if err := json.Unmarshal(raw, &upstream); err != nil {
		return agentPasskeyConfig{}, err
	}
	rpID := strings.ToLower(strings.Trim(strings.TrimSpace(upstream.RPID), "."))
	origin := agentRequestOrigin(r)
	supported := validPasskeyRPOrigin(origin, rpID, upstream.Origins)
	return agentPasskeyConfig{
		Enabled:         upstream.Enabled && upstream.Configured && supported,
		Configured:      upstream.Configured,
		SupportedOrigin: supported,
		RPID:            rpID,
	}, nil
}

func agentRequestOrigin(r *http.Request) string {
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && sameOrigin(r) {
		if parsed, err := url.Parse(origin); err == nil {
			return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if forwarded := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	return strings.ToLower(scheme + "://" + strings.TrimSpace(r.Host))
}

func validPasskeyRPOrigin(origin, rpID string, allowed []string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Host == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if rpID == "" || (host != rpID && !strings.HasSuffix(host, "."+rpID)) {
		return false
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(host)) {
		return false
	}
	canonical := strings.ToLower(parsed.Scheme + "://" + parsed.Host)
	for _, candidate := range allowed {
		candidateURL, candidateErr := url.Parse(strings.TrimSpace(candidate))
		if candidateErr == nil && candidateURL.User == nil && (candidateURL.Path == "" || candidateURL.Path == "/") && candidateURL.RawQuery == "" && candidateURL.Fragment == "" && strings.ToLower(candidateURL.Scheme+"://"+candidateURL.Host) == canonical {
			return true
		}
	}
	return false
}

func decodePasskeyOptions(data json.RawMessage) (passkeyOptionsView, error) {
	var raw struct {
		SessionToken string          `json:"session_token"`
		Options      json.RawMessage `json:"options"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || strings.TrimSpace(raw.SessionToken) == "" || !json.Valid(raw.Options) || string(raw.Options) == "null" {
		return passkeyOptionsView{}, errors.New("invalid passkey options")
	}
	return passkeyOptionsView{SessionToken: raw.SessionToken, Options: raw.Options}, nil
}

func validPasskeyCredential(raw json.RawMessage) bool {
	if len(raw) == 0 || !json.Valid(raw) || len(raw) > maxPasskeyBody {
		return false
	}
	var credential struct {
		ID       string          `json:"id"`
		RawID    string          `json:"rawId"`
		Type     string          `json:"type"`
		Response json.RawMessage `json:"response"`
	}
	if json.Unmarshal(raw, &credential) != nil {
		return false
	}
	return strings.TrimSpace(credential.ID) != "" && strings.TrimSpace(credential.RawID) != "" && credential.Type == "public-key" && json.Valid(credential.Response) && string(credential.Response) != "null"
}
