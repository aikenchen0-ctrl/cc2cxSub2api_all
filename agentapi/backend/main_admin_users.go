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
