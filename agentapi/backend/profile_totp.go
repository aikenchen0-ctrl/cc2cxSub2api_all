package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

type agentTOTPStatus struct {
	Enabled        bool   `json:"enabled"`
	EnabledAt      *int64 `json:"enabled_at,omitempty"`
	FeatureEnabled bool   `json:"feature_enabled"`
}

type agentTOTPVerificationMethod struct {
	Method string `json:"method"`
}

type agentTOTPSetup struct {
	Secret     string `json:"secret"`
	QRCodeURL  string `json:"qr_code_url"`
	SetupToken string `json:"setup_token"`
	Countdown  int    `json:"countdown"`
}

func (s *Server) handleAgentTOTP(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	if strings.TrimSpace(session.AccessToken) == "" {
		s.writeError(w, http.StatusForbidden, requestID, "MAIN_SESSION_REQUIRED", "sign in with your main-site account to manage two-factor authentication")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agent/totp/")
	isWrite := r.Method != http.MethodGet
	if isWrite && !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	if !validAgentTOTPRoute(path, r.Method) {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	// Validate/refresh the server-side Sub2API session before every security
	// operation. The refreshed token is never serialized to the browser.
	if _, err := s.currentSessionUser(r.Context(), session); err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	refreshedSession, err := s.store.LoadSession(session.ID)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "AgentAPI session is no longer valid")
		return
	}
	session = refreshedSession

	var raw json.RawMessage
	switch path {
	case "status":
		raw, err = s.main.UserTOTPStatus(r.Context(), session.AccessToken)
	case "verification-method":
		raw, err = s.main.UserTOTPVerificationMethod(r.Context(), session.AccessToken)
	case "send-code":
		raw, err = s.main.UserTOTPSendCode(r.Context(), session.AccessToken)
	case "setup":
		var payload struct {
			EmailCode string `json:"email_code"`
			Password  string `json:"password"`
		}
		if !decodeTOTPVerificationPayload(s, w, r, requestID, &payload) {
			return
		}
		raw, err = s.main.UserTOTPSetup(r.Context(), session.AccessToken, payload.EmailCode, payload.Password)
	case "enable":
		var payload struct {
			Code       string `json:"totp_code"`
			SetupToken string `json:"setup_token"`
		}
		if decodeJSON(r, &payload, maxJSONBody) != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "invalid two-factor setup request")
			return
		}
		payload.Code = strings.TrimSpace(payload.Code)
		payload.SetupToken = strings.TrimSpace(payload.SetupToken)
		if !sixDigits(payload.Code) || invalidSecretValue(payload.SetupToken, 4096) {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_TOTP_SETUP", "a six-digit code and valid setup token are required")
			return
		}
		raw, err = s.main.UserTOTPEnable(r.Context(), session.AccessToken, payload.Code, payload.SetupToken)
	case "disable":
		var payload struct {
			EmailCode string `json:"email_code"`
			Password  string `json:"password"`
		}
		if !decodeTOTPVerificationPayload(s, w, r, requestID, &payload) {
			return
		}
		raw, err = s.main.UserTOTPDisable(r.Context(), session.AccessToken, payload.EmailCode, payload.Password)
	}
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}

	var response any
	switch path {
	case "status":
		var value agentTOTPStatus
		if json.Unmarshal(raw, &value) != nil {
			s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_RESPONSE_INVALID", "main-site two-factor status is invalid")
			return
		}
		response = value
	case "verification-method":
		var value agentTOTPVerificationMethod
		if json.Unmarshal(raw, &value) != nil || (value.Method != "email" && value.Method != "password") {
			s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_RESPONSE_INVALID", "main-site verification method is invalid")
			return
		}
		response = value
	case "setup":
		var value agentTOTPSetup
		if json.Unmarshal(raw, &value) != nil || !validTOTPSetup(value) {
			s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_RESPONSE_INVALID", "main-site two-factor setup response is invalid")
			return
		}
		response = value
	default:
		var value struct {
			Success bool `json:"success"`
		}
		if json.Unmarshal(raw, &value) != nil || !value.Success {
			s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_RESPONSE_INVALID", "main-site two-factor operation response is invalid")
			return
		}
		response = value
	}
	s.writeData(w, http.StatusOK, requestID, response)
}

func validAgentTOTPRoute(path, method string) bool {
	switch path {
	case "status", "verification-method":
		return method == http.MethodGet
	case "send-code", "setup", "enable", "disable":
		return method == http.MethodPost
	default:
		return false
	}
}

func decodeTOTPVerificationPayload(s *Server, w http.ResponseWriter, r *http.Request, requestID string, payload *struct {
	EmailCode string `json:"email_code"`
	Password  string `json:"password"`
}) bool {
	if err := decodeJSON(r, payload, maxJSONBody); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "invalid verification request")
		return false
	}
	payload.EmailCode = strings.TrimSpace(payload.EmailCode)
	if payload.EmailCode != "" && !sixDigits(payload.EmailCode) {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_VERIFICATION", "email verification code must contain six digits")
		return false
	}
	if payload.Password != "" && invalidSecretValue(payload.Password, 1024) {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_VERIFICATION", "password is invalid")
		return false
	}
	if (payload.EmailCode == "") == (payload.Password == "") {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_VERIFICATION", "provide exactly one verification method")
		return false
	}
	return true
}

func sixDigits(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func invalidSecretValue(value string, max int) bool {
	return value == "" || len(value) > max || strings.ContainsAny(value, "\r\n\x00")
}

func validTOTPSetup(value agentTOTPSetup) bool {
	if invalidSecretValue(value.Secret, 512) || invalidSecretValue(value.SetupToken, 4096) || len(value.QRCodeURL) > 4096 || value.Countdown < 0 || value.Countdown > 3600 {
		return false
	}
	parsed, err := url.Parse(value.QRCodeURL)
	return err == nil && parsed.Scheme == "otpauth" && parsed.Host == "totp" && parsed.RawQuery != ""
}
