package main

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/mail"
	"strings"
)

var errInvalidBalanceNotifyResponse = errors.New("invalid main-site balance notification response")

type agentNotifyEmailEntry struct {
	Email    string `json:"email"`
	Disabled bool   `json:"disabled"`
	Verified bool   `json:"verified"`
}

type agentBalanceNotifySettings struct {
	FeatureEnabled         bool                    `json:"feature_enabled"`
	SystemDefaultThreshold float64                 `json:"system_default_threshold"`
	Enabled                bool                    `json:"enabled"`
	Threshold              *float64                `json:"threshold"`
	ExtraEmails            []agentNotifyEmailEntry `json:"extra_emails"`
}

func (s *Server) handleAgentBalanceNotify(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	if strings.TrimSpace(session.AccessToken) == "" {
		s.writeError(w, http.StatusForbidden, requestID, "MAIN_SESSION_REQUIRED", "sign in with your main-site account to manage balance notifications")
		return
	}

	operation := strings.TrimPrefix(r.URL.Path, "/api/v1/agent/balance-notify")
	operation = strings.TrimPrefix(operation, "/")
	if !validBalanceNotifyRoute(operation, r.Method) {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if r.Method != http.MethodGet && !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
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
	session = refreshed

	if operation == "" && r.Method == http.MethodGet {
		profile, profileErr := s.main.UserProfile(r.Context(), session.AccessToken)
		if profileErr != nil {
			s.writeMainError(w, requestID, profileErr)
			return
		}
		publicSettings, settingsErr := s.main.PublicSettings(r.Context())
		if settingsErr != nil {
			s.writeMainError(w, requestID, settingsErr)
			return
		}
		settings, parseErr := safeBalanceNotifySettings(profile, publicSettings)
		if parseErr != nil {
			s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_RESPONSE_INVALID", "main-site balance notification settings are invalid")
			return
		}
		s.writeData(w, http.StatusOK, requestID, settings)
		return
	}

	var raw json.RawMessage
	switch operation {
	case "":
		var payload struct {
			Enabled   *bool    `json:"enabled"`
			Threshold *float64 `json:"threshold"`
		}
		if err := decodeJSON(r, &payload, maxJSONBody); err != nil || (payload.Enabled == nil && payload.Threshold == nil) {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "provide a balance notification setting to update")
			return
		}
		if payload.Threshold != nil && (!finiteNonNegative(*payload.Threshold) || *payload.Threshold > 1_000_000_000) {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_THRESHOLD", "balance notification threshold must be a finite non-negative amount")
			return
		}
		raw, err = s.main.UserUpdateBalanceNotify(r.Context(), session.AccessToken, payload.Enabled, payload.Threshold)
	case "send-code":
		email, valid := decodeBalanceNotifyEmail(s, w, r, requestID)
		if !valid {
			return
		}
		raw, err = s.main.UserNotifyEmailSendCode(r.Context(), session.AccessToken, email, r.Header.Get("Accept-Language"))
	case "verify":
		var payload struct {
			Email string `json:"email"`
			Code  string `json:"code"`
		}
		if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "invalid notification email verification request")
			return
		}
		payload.Email = normalizeNotifyEmail(payload.Email)
		payload.Code = strings.TrimSpace(payload.Code)
		if !validNotifyEmail(payload.Email) || !sixDigits(payload.Code) {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_VERIFICATION", "a valid email and six-digit code are required")
			return
		}
		raw, err = s.main.UserNotifyEmailVerify(r.Context(), session.AccessToken, payload.Email, payload.Code)
	case "toggle":
		var payload struct {
			Email    string `json:"email"`
			Disabled *bool  `json:"disabled"`
		}
		if err := decodeJSON(r, &payload, maxJSONBody); err != nil || payload.Disabled == nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "email and disabled state are required")
			return
		}
		payload.Email = normalizeNotifyEmail(payload.Email)
		if !validNotifyEmail(payload.Email) {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_EMAIL", "a valid notification email is required")
			return
		}
		raw, err = s.main.UserNotifyEmailToggle(r.Context(), session.AccessToken, payload.Email, *payload.Disabled)
	case "email":
		email, valid := decodeBalanceNotifyEmail(s, w, r, requestID)
		if !valid {
			return
		}
		raw, err = s.main.UserNotifyEmailRemove(r.Context(), session.AccessToken, email)
	}
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}

	// UpdateProfile and toggle return a full main-site profile; other actions
	// return success only. In both cases emit only the notification allowlist.
	if operation == "" || operation == "toggle" {
		publicSettings, settingsErr := s.main.PublicSettings(r.Context())
		if settingsErr != nil {
			s.writeMainError(w, requestID, settingsErr)
			return
		}
		profileSettings, parseErr := safeBalanceNotifySettings(raw, publicSettings)
		if parseErr != nil {
			s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_RESPONSE_INVALID", "main-site balance notification response is invalid")
			return
		}
		s.writeData(w, http.StatusOK, requestID, profileSettings)
		return
	}
	s.writeData(w, http.StatusOK, requestID, map[string]bool{"success": true})
}

func validBalanceNotifyRoute(operation, method string) bool {
	switch operation {
	case "":
		return method == http.MethodGet || method == http.MethodPut
	case "send-code", "verify":
		return method == http.MethodPost
	case "toggle":
		return method == http.MethodPut
	case "email":
		return method == http.MethodDelete
	default:
		return false
	}
}

func safeBalanceNotifySettings(profileRaw, settingsRaw []byte) (agentBalanceNotifySettings, error) {
	var profile struct {
		Enabled     bool                    `json:"balance_notify_enabled"`
		Threshold   *float64                `json:"balance_notify_threshold"`
		ExtraEmails []agentNotifyEmailEntry `json:"balance_notify_extra_emails"`
	}
	var settings struct {
		FeatureEnabled bool    `json:"balance_low_notify_enabled"`
		Threshold      float64 `json:"balance_low_notify_threshold"`
	}
	if json.Unmarshal(profileRaw, &profile) != nil || json.Unmarshal(settingsRaw, &settings) != nil || !finiteNonNegative(settings.Threshold) || settings.Threshold > 1_000_000_000 {
		return agentBalanceNotifySettings{}, errInvalidBalanceNotifyResponse
	}
	if len(profile.ExtraEmails) > 3 || (profile.Threshold != nil && !finiteNonNegative(*profile.Threshold)) {
		return agentBalanceNotifySettings{}, errInvalidBalanceNotifyResponse
	}
	clean := make([]agentNotifyEmailEntry, 0, len(profile.ExtraEmails))
	seen := map[string]struct{}{}
	for _, entry := range profile.ExtraEmails {
		entry.Email = normalizeNotifyEmail(entry.Email)
		if !validNotifyEmail(entry.Email) {
			return agentBalanceNotifySettings{}, errInvalidBalanceNotifyResponse
		}
		if _, exists := seen[entry.Email]; exists {
			return agentBalanceNotifySettings{}, errInvalidBalanceNotifyResponse
		}
		seen[entry.Email] = struct{}{}
		clean = append(clean, entry)
	}
	return agentBalanceNotifySettings{FeatureEnabled: settings.FeatureEnabled, SystemDefaultThreshold: settings.Threshold, Enabled: profile.Enabled, Threshold: profile.Threshold, ExtraEmails: clean}, nil
}

func decodeBalanceNotifyEmail(s *Server, w http.ResponseWriter, r *http.Request, requestID string) (string, bool) {
	var payload struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "invalid notification email request")
		return "", false
	}
	email := normalizeNotifyEmail(payload.Email)
	if !validNotifyEmail(email) {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_EMAIL", "a valid notification email is required")
		return "", false
	}
	return email, true
}

func normalizeNotifyEmail(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func validNotifyEmail(value string) bool {
	if value == "" || len(value) > 254 || strings.ContainsAny(value, "\r\n\x00") {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && strings.EqualFold(parsed.Address, value) && strings.Contains(value, "@")
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}
