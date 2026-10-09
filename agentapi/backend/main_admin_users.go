package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// The main administrator credential is deployment-only. Never accept an
// arbitrary admin path/payload from a tenant, or use this key for inference.
func (c *MainClient) CreateMainUser(ctx context.Context, email, password, username, marker string) (json.RawMessage, error) {
	if c.cfg.MainAdminAPIKey == "" {
		return nil, &MainAPIError{Status: 503, Code: "MAIN_ADMIN_NOT_CONFIGURED", Message: "main user provisioning is not configured"}
	}
	body, err := json.Marshal(map[string]any{"email": email, "password": password, "username": username, "role": "user", "balance": 0, "notes": marker})
	if err != nil {
		return nil, err
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("x-api-key", c.cfg.MainAdminAPIKey)
	response, raw, err := c.request(ctx, http.MethodPost, c.endpoint("/admin/users"), body, headers)
	if err != nil {
		return nil, &MainAPIError{Status: 503, Code: "MAIN_REGISTRATION_UNAVAILABLE", Message: "registration is temporarily unavailable"}
	}
	data, err := unwrapMainResponse(response.StatusCode, raw)
	if err != nil {
		// Admin diagnostics may contain internal account/configuration details.
		// Preserve only useful public status classes, never upstream text/code.
		var upstream *MainAPIError
		if errors.As(err, &upstream) {
			switch upstream.Status {
			case 400, 422:
				return nil, &MainAPIError{Status: 400, Code: "REGISTRATION_REJECTED", Message: "registration details were rejected; check your email and password"}
			case 409:
				return nil, &MainAPIError{Status: 409, Code: "REGISTRATION_CONFLICT", Message: "registration could not be completed; if already registered, try signing in"}
			case 429:
				return nil, &MainAPIError{Status: 429, Code: "REGISTRATION_RATE_LIMITED", Message: "too many registration attempts; try again later"}
			}
		}
		return nil, &MainAPIError{Status: 503, Code: "MAIN_REGISTRATION_UNAVAILABLE", Message: "registration is temporarily unavailable"}
	}
	var profile struct {
		ID    json.RawMessage `json:"id"`
		Email string          `json:"email"`
		Role  string          `json:"role"`
	}
	if json.Unmarshal(data, &profile) != nil {
		return nil, invalidCreatedUser()
	}
	id := jsonID(rawJSONAny(profile.ID))
	parsedID, parseErr := strconv.ParseInt(id, 10, 64)
	if parseErr != nil || parsedID <= 0 || profile.Role != "user" || !strings.EqualFold(strings.TrimSpace(profile.Email), strings.TrimSpace(email)) {
		return nil, invalidCreatedUser()
	}
	return data, nil
}

func invalidCreatedUser() error {
	return &MainAPIError{Status: 502, Code: "UPSTREAM_IDENTITY_MISMATCH", Message: "main user creation returned an unexpected identity"}
}

// AdminReadUserBalance reads one main-site user's balance with the deployment
// admin key. It is read-only: the key never leaves the server, and the caller
// must already have proven the user belongs to the current tenant or session.
func (c *MainClient) AdminReadUserBalance(ctx context.Context, mainUserID string) (MainUserResult, error) {
	mainUserID = strings.TrimSpace(mainUserID)
	parsedID, err := strconv.ParseInt(mainUserID, 10, 64)
	if err != nil || parsedID <= 0 {
		return MainUserResult{}, &MainAPIError{Status: http.StatusBadRequest, Code: "INVALID_USER", Message: "invalid user"}
	}
	path, err := adminUserPath(mainUserID, "")
	if err != nil {
		return MainUserResult{}, err
	}
	data, err := c.adminGet(ctx, path)
	if err != nil || mainUserIDFromJSON(data) != mainUserID {
		return MainUserResult{}, &MainAPIError{Status: http.StatusBadGateway, Code: "MAIN_BALANCE_UNAVAILABLE", Message: "main balance lookup is temporarily unavailable"}
	}
	balance, frozen, positive, err := decodeMainBalanceFacts(data)
	if err != nil {
		return MainUserResult{}, &MainAPIError{Status: http.StatusBadGateway, Code: "MAIN_BALANCE_UNAVAILABLE", Message: "main balance lookup returned an invalid balance"}
	}
	return MainUserResult{ID: mainUserID, Balance: balance, FrozenBalance: frozen, BalancePositive: positive}, nil
}

func (c *MainClient) AdminAddUserBalance(ctx context.Context, mainUserID string, amount float64, notes string) error {
	path, err := adminUserPath(mainUserID, "/balance")
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.cfg.MainAdminAPIKey) == "" {
		return &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_ADMIN_NOT_CONFIGURED", Message: "main administrator key is not configured"}
	}
	headers := make(http.Header)
	headers.Set("Accept", "application/json")
	headers.Set("Content-Type", "application/json")
	headers.Set("x-api-key", c.cfg.MainAdminAPIKey)
	payload, err := json.Marshal(map[string]any{"balance": amount, "operation": "add", "notes": notes})
	if err != nil {
		return err
	}
	response, raw, err := c.request(ctx, http.MethodPost, c.endpoint(path), payload, headers)
	if err != nil {
		return &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_BALANCE_UNAVAILABLE", Message: "main user recharge record is temporarily unavailable"}
	}
	if _, err := unwrapMainResponse(response.StatusCode, raw); err != nil {
		return &MainAPIError{Status: http.StatusBadGateway, Code: "MAIN_BALANCE_UNAVAILABLE", Message: "main user recharge record was rejected"}
	}
	return nil
}

func (s *Server) lookupDisplayBalance(ctx context.Context, mainUserID string) (int64, int64, bool) {
	if result, err := s.main.AdminReadUserBalance(ctx, mainUserID); err == nil {
		return result.Balance, result.FrozenBalance, true
	}
	if result, err := s.main.SatelliteUserBalance(ctx, mainUserID); err == nil {
		return result.Balance, result.FrozenBalance, true
	}
	return 0, 0, false
}
