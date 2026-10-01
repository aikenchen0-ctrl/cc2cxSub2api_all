package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type preflightCheck struct {
	Name       string `json:"name"`
	OK         bool   `json:"ok"`
	Code       string `json:"code"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type preflightReport struct {
	OK     bool             `json:"ok"`
	Checks []preflightCheck `json:"checks"`
}

// Deployment-only, read-only probes. No database, model calls, response bodies,
// account details, URLs or raw errors are emitted in the report.
func runPreflight(ctx context.Context, cfg Config) preflightReport {
	report := preflightReport{OK: true}
	add := func(name string, ok bool, code string, status int) {
		report.Checks = append(report.Checks, preflightCheck{name, ok, code, status})
		report.OK = report.OK && ok
	}
	static := func(name string, ok bool) {
		code := "configuration_missing_or_invalid"
		if ok {
			code = "configured_only"
		}
		add(name, ok, code, 0)
	}
	static("public_link", validPreflightURL(cfg.PublicMainURL))
	static("session_secret", !cfg.SessionSecretWeak && len(cfg.SessionSecret) >= 32)
	static("sso_secret", len(cfg.SSOSecret) >= 32)
	static("user_upstream_mode", cfg.BillingMode == "user_upstream")
	probeUserID := strings.TrimSpace(cfg.PreflightMainUserID)
	if probeUserID == "" && strings.TrimSpace(cfg.AgentID) != "" {
		// Explicit single-tenant migration mode may reuse its configured owner.
		// Shared production must configure a separate read-only probe identity.
		probeUserID = strings.TrimSpace(cfg.OwnerMainUserID)
	}
	uid, err := strconv.ParseInt(probeUserID, 10, 64)
	validUser := err == nil && uid > 0
	static("probe_identity", validUser)
	client := NewMainClient(cfg)
	probe := func(name, target string, headers http.Header, ready bool, validate func(json.RawMessage) bool) {
		if !ready || !validUser || !validPreflightURL(target) {
			add(name, false, "configuration_missing_or_invalid", 0)
			return
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			add(name, false, "invalid_endpoint", 0)
			return
		}
		req.Header = headers
		resp, err := client.http.Do(req)
		if err != nil {
			add(name, false, "transport_error", 0)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			add(name, false, "upstream_http_error", resp.StatusCode)
			return
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
		if err != nil || len(body) > 1<<20 || !json.Valid(body) {
			add(name, false, "invalid_response", resp.StatusCode)
			return
		}
		var envelope struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(body, &envelope) != nil {
			add(name, false, "invalid_response", resp.StatusCode)
			return
		}
		if envelope.Code != 0 {
			add(name, false, "upstream_application_error", resp.StatusCode)
			return
		}
		if len(envelope.Data) > 0 {
			body = envelope.Data
		}
		ok := validate(body)
		code := "verified_read_only"
		if !ok {
			code = "invalid_response"
		}
		add(name, ok, code, resp.StatusCode)
	}
	adminHeaders := make(http.Header)
	adminHeaders.Set("x-api-key", cfg.MainAdminAPIKey)
	probe("admin_user_read", client.endpoint("/admin/users/"+url.PathEscape(probeUserID)), adminHeaders,
		cfg.MainAdminAPIKey != "", func(data json.RawMessage) bool { return mainUserIDFromJSON(data) == probeUserID })
	satelliteHeaders := make(http.Header)
	satelliteHeaders.Set("Authorization", "Bearer "+cfg.AppCredential)
	satelliteHeaders.Set("X-Sub2API-Satellite", cfg.SatelliteSlug)
	satelliteHeaders.Set("X-Sub2API-On-Behalf-Of", probeUserID)
	base := strings.TrimSuffix(strings.TrimRight(cfg.MainModelBaseURL, "/"), "/v1")
	probe("satellite_balance_read", base+"/v1/sub2api/balance", satelliteHeaders,
		cfg.AppCredential != "" && cfg.SatelliteSlug == "agentapi", func(data json.RawMessage) bool {
			_, _, err := decodeMainBalance(data)
			return err == nil
		})
	return report
}

func validPreflightURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
