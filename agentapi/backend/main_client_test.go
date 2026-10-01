package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPasswordRecoveryUsesOnlySatelliteApplicationCredential(t *testing.T) {
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/api/v1/settings/public" {
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-Sub2API-Satellite") != "" {
				t.Errorf("public settings unexpectedly received credentials: %v", r.Header)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"password_reset_enabled": true}))
			return
		}
		if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
			t.Errorf("satellite password recovery headers are invalid: %v", r.Header)
		}
		if r.Header.Get("X-Sub2API-On-Behalf-Of") != "" || r.Header.Get("X-API-Key") != "" || r.Header.Get("Cookie") != "" {
			t.Errorf("user, admin, or browser credentials leaked to password recovery: %v", r.Header)
		}
		if r.Header.Get("Accept-Language") != "zh-CN" {
			t.Errorf("locale was not forwarded safely: %q", r.Header.Get("Accept-Language"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode password recovery payload: %v", err)
		}
		if body["email"] != "user@example.com" {
			t.Errorf("unexpected password recovery payload: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"message": "ok"}))
	}))
	defer upstream.Close()

	client := NewMainClient(Config{
		MainAPIBaseURL: upstream.URL + "/api/v1", MainRequestTimeout: time.Second,
		AppCredential: "app-secret", SatelliteSlug: "agentapi",
	})
	if _, err := client.PublicSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ForgotPassword(context.Background(), map[string]any{"email": "user@example.com"}, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ResetPassword(context.Background(), map[string]any{"email": "user@example.com", "token": "one-time", "new_password": "secret123"}, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/api/v1/settings/public",
		"/api/v1/auth/satellite/agentapi/forgot-password",
		"/api/v1/auth/satellite/agentapi/reset-password",
	}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("unexpected password recovery paths: got %v want %v", paths, want)
	}
}

func TestRegistrationEmailVerificationUsesOnlySatelliteApplicationCredential(t *testing.T) {
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
			t.Errorf("satellite verification headers are invalid: %v", r.Header)
		}
		for _, forbidden := range []string{"X-Sub2API-On-Behalf-Of", "X-API-Key", "Cookie"} {
			if r.Header.Get(forbidden) != "" {
				t.Errorf("credential %s leaked to registration verification", forbidden)
			}
		}
		if r.Header.Get("Accept-Language") != "zh-CN" {
			t.Errorf("locale was not forwarded safely: %q", r.Header.Get("Accept-Language"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["email"] != "user@example.com" {
			t.Errorf("unexpected verification payload: %#v", body)
		}
		if strings.HasSuffix(r.URL.Path, "/verify-registration-email") && body["verify_code"] != "123456" {
			t.Errorf("verification code was not forwarded: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"message": "ok", "countdown": 60}))
	}))
	defer upstream.Close()

	client := NewMainClient(Config{
		MainAPIBaseURL: upstream.URL + "/api/v1", MainRequestTimeout: time.Second,
		AppCredential: "app-secret", SatelliteSlug: "agentapi",
	})
	if _, err := client.SendRegistrationVerifyCode(t.Context(), "user@example.com", "zh-CN"); err != nil {
		t.Fatal(err)
	}
	if err := client.VerifyRegistrationEmail(t.Context(), "user@example.com", "123456", "zh-CN"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/api/v1/auth/satellite/agentapi/send-verify-code",
		"/api/v1/auth/satellite/agentapi/verify-registration-email",
	}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("unexpected verification paths: got %v want %v", paths, want)
	}
}

func TestMainClientProfileAndPasswordUseOnlyAuthenticatedUserEndpoints(t *testing.T) {
	const accessToken = "main-user-access-token"
	var profileCalls, updateCalls, passwordCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+accessToken {
			t.Errorf("user credential was not scoped to the authenticated account endpoint: %v", r.Header)
		}
		if r.Header.Get("X-AgentAPI-Runtime-Control") != "" || r.Header.Get("Cookie") != "" {
			t.Errorf("unexpected satellite or browser credential forwarded to user endpoint: %v", r.Header)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/user/profile":
			profileCalls++
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "user@example.com", "username": "User"}))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/user":
			updateCalls++
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode profile update: %v", err)
			}
			if len(payload) != 1 || payload["username"] != "Updated User" {
				t.Errorf("profile proxy accepted more than the username field: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 42, "email": "user@example.com", "username": "Updated User"}))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/user/password":
			passwordCalls++
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode password change: %v", err)
			}
			if len(payload) != 2 || payload["old_password"] != "old-password" || payload["new_password"] != "new-password" {
				t.Errorf("password proxy sent unexpected fields: %#v", payload)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"message": "Password changed successfully"}))
		default:
			t.Errorf("unexpected Sub2API user endpoint: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	client := NewMainClient(Config{MainAPIBaseURL: upstream.URL + "/api/v1", MainRequestTimeout: time.Second})
	profile, err := client.UserProfile(t.Context(), accessToken)
	if err != nil || !strings.Contains(string(profile), `"username":"User"`) {
		t.Fatalf("profile request failed: profile=%s err=%v", profile, err)
	}
	profile, err = client.UpdateUserProfile(t.Context(), accessToken, "Updated User")
	if err != nil || !strings.Contains(string(profile), `"username":"Updated User"`) {
		t.Fatalf("profile update failed: profile=%s err=%v", profile, err)
	}
	if err := client.ChangeUserPassword(t.Context(), accessToken, "old-password", "new-password"); err != nil {
		t.Fatalf("password change failed: %v", err)
	}
	if profileCalls != 1 || updateCalls != 1 || passwordCalls != 1 {
		t.Fatalf("unexpected user endpoint calls: profile=%d update=%d password=%d", profileCalls, updateCalls, passwordCalls)
	}
}

func TestMainClientTOTPUsesOnlyAuthenticatedUserEndpoints(t *testing.T) {
	const accessToken = "main-user-access-token"
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer "+accessToken {
			t.Errorf("user token missing from TOTP request: %v", r.Header)
		}
		for _, forbidden := range []string{"X-API-Key", "X-Sub2API-Satellite", "X-AgentAPI-Runtime-Control", "Cookie"} {
			if r.Header.Get(forbidden) != "" {
				t.Errorf("credential %s leaked to user TOTP endpoint", forbidden)
			}
		}
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{"success": true, "method": "password", "enabled": false, "feature_enabled": true, "secret": "S", "qr_code_url": "otpauth://totp/A?secret=S", "setup_token": "T", "countdown": 1}))
	}))
	defer upstream.Close()
	client := NewMainClient(Config{MainAPIBaseURL: upstream.URL + "/api/v1", MainRequestTimeout: time.Second})
	if _, err := client.UserTOTPStatus(t.Context(), accessToken); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UserTOTPVerificationMethod(t.Context(), accessToken); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UserTOTPSendCode(t.Context(), accessToken); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UserTOTPSetup(t.Context(), accessToken, "", "password"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UserTOTPEnable(t.Context(), accessToken, "123456", "token"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UserTOTPDisable(t.Context(), accessToken, "", "password"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /api/v1/user/totp/status",
		"GET /api/v1/user/totp/verification-method",
		"POST /api/v1/user/totp/send-code",
		"POST /api/v1/user/totp/setup",
		"POST /api/v1/user/totp/enable",
		"POST /api/v1/user/totp/disable",
	}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("unexpected TOTP endpoint paths: got %v want %v", paths, want)
	}
}

func TestModelRelayDoesNotForwardCredentialsAcrossRedirects(t *testing.T) {
	const appCredential = "agentapi-app-credential"
	const mainUserID = "43"
	var redirectedRequests int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedRequests++
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "" {
			t.Errorf("model credentials reached redirect target: %v", r.Header)
		}
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	cfg := Config{
		MainModelBaseURL:   redirector.URL + "/v1",
		AppCredential:      appCredential,
		SatelliteSlug:      "agentapi",
		ModelStreamTimeout: time.Second,
	}
	status, _, _, err := NewMainClient(cfg).RelayModelMethod(t.Context(), http.MethodPost, "/v1/models", nil, nil, mainUserID, "")
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusTemporaryRedirect {
		t.Fatalf("unexpected redirect response status: %d", status)
	}
	if redirectedRequests != 0 {
		t.Fatalf("model redirect target received %d request(s)", redirectedRequests)
	}
}

func TestAdminFindUsageUsesZeroSchemaOwnerBridgeWithoutRuntimeCredential(t *testing.T) {
	var gotPath, gotAuthorization, gotOwner, gotSatellite string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		gotAuthorization = r.Header.Get("Authorization")
		gotOwner = r.Header.Get("X-Sub2API-On-Behalf-Of")
		gotSatellite = r.Header.Get("X-Sub2API-Satellite")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []map[string]any{{
			"id": 88, "request_id": "req-owner-usage", "model": "gpt-5.5", "total_cost": 0.30, "actual_cost": 0.25,
		}}}})
	}))
	defer upstream.Close()
	cfg := Config{
		MainModelBaseURL: upstream.URL, AppCredential: "satellite-app-secret",
		OwnerMainUserID: "42", SatelliteSlug: "agentapi", MainRequestTimeout: time.Second,
	}
	items, err := NewMainClient(cfg).AdminFindUsageForUser(t.Context(), "42", "req-owner-usage")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/sub2api/usage?request_id=req-owner-usage" || gotAuthorization != "Bearer satellite-app-secret" || gotOwner != "42" || gotSatellite != "agentapi" {
		t.Fatalf("unexpected zero-schema usage bridge request: path=%q auth=%q owner=%q satellite=%q", gotPath, gotAuthorization, gotOwner, gotSatellite)
	}
	if len(items) != 1 || items[0].ActualCents != 25 || items[0].Snapshot.Source != "sub2api_user_usage" {
		t.Fatalf("unexpected zero-schema usage result: %+v", items)
	}
}

func TestOwnerBridgeUsesOnlyPublicSatelliteEndpoints(t *testing.T) {
	var balanceCalls, usageCalls, runtimeCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sub2api/balance":
			balanceCalls++
		case "/v1/sub2api/usage":
			usageCalls++
		default:
			runtimeCalls++
			http.Error(w, "ordinary mode reached legacy runtime endpoint", http.StatusInternalServerError)
			return
		}
		if r.Header.Get("Authorization") != "Bearer satellite-app-secret" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "42" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
			t.Errorf("ordinary bridge headers are incorrect: %v", r.Header)
		}
		if r.URL.Path == "/v1/sub2api/balance" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"balance": 12.34, "frozen_balance": 2.5}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []map[string]any{{
			"id": 90, "request_id": "req-stale-control", "actual_cost": 0.12,
		}}}})
	}))
	defer upstream.Close()
	cfg := Config{
		MainAPIBaseURL: upstream.URL + "/api/v1", MainModelBaseURL: upstream.URL,
		AppCredential:   "satellite-app-secret",
		OwnerMainUserID: "42", SatelliteSlug: "agentapi", MainRequestTimeout: time.Second,
	}
	client := NewMainClient(cfg)
	owner, err := client.AdminGetUser(t.Context(), "42")
	if err != nil || owner.Balance != 1234 || owner.FrozenBalance != 250 {
		t.Fatalf("ordinary owner balance did not use satellite bridge: owner=%+v err=%v", owner, err)
	}
	items, err := client.AdminFindUsageForUser(t.Context(), "42", "req-stale-control")
	if err != nil || len(items) != 1 || items[0].ActualCents != 12 {
		t.Fatalf("ordinary usage did not use satellite bridge: items=%+v err=%v", items, err)
	}
	if balanceCalls != 1 || usageCalls != 1 || runtimeCalls != 0 {
		t.Fatalf("public satellite routing changed unexpectedly: balance=%d usage=%d runtime=%d", balanceCalls, usageCalls, runtimeCalls)
	}
}

func TestAdminFindUsagePreservesZeroActualCostInsteadOfUsingStandardCost(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"items": []map[string]any{{
				"id": 78, "request_id": "req-zero-actual", "model": "free-model", "total_cost": 1.25, "actual_cost": 0,
			}},
		}})
	}))
	defer upstream.Close()

	cfg := Config{MainModelBaseURL: upstream.URL, AppCredential: "satellite-app-secret", OwnerMainUserID: "42", SatelliteSlug: "agentapi", MainRequestTimeout: time.Second}
	items, err := NewMainClient(cfg).AdminFindUsageForUser(t.Context(), "42", "req-zero-actual")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].HasActualCost || items[0].ActualCents != 0 || items[0].TotalCents != 125 {
		t.Fatalf("explicit zero actual cost was replaced by standard cost: %+v", items)
	}
}

func TestAdminFindUsageRejectsStandardCostAsActualCharge(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"items": []map[string]any{{"id": 79, "request_id": "req-legacy-dto", "total_cost": 0.25}},
		}})
	}))
	defer upstream.Close()

	cfg := Config{MainModelBaseURL: upstream.URL, AppCredential: "satellite-app-secret", OwnerMainUserID: "42", SatelliteSlug: "agentapi", MainRequestTimeout: time.Second}
	items, err := NewMainClient(cfg).AdminFindUsageForUser(t.Context(), "42", "req-legacy-dto")
	if err == nil || len(items) != 0 {
		t.Fatalf("missing actual_cost must not become a confirmed standard charge: items=%+v err=%v", items, err)
	}
}

func TestRelayModelPreservesMultipartContentTypeAndSessionIdentity(t *testing.T) {
	var gotContentType, gotAuthorization, gotOwner, gotSatellite, gotRequestID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotAuthorization = r.Header.Get("Authorization")
		gotOwner = r.Header.Get("X-Sub2API-On-Behalf-Of")
		gotSatellite = r.Header.Get("X-Sub2API-Satellite")
		gotRequestID = r.Header.Get("X-Request-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	cfg := Config{
		MainModelBaseURL:   upstream.URL + "/v1",
		AppCredential:      "app-secret",
		OwnerMainUserID:    "owner-1",
		SatelliteSlug:      "agentapi",
		MainRequestTimeout: time.Second,
	}
	_, _, body, err := NewMainClient(cfg).RelayModelMethodWithContentType(
		t.Context(), http.MethodPost, "/v1/images/edits", nil,
		[]byte("multipart-body"), "multipart/form-data; boundary=test-boundary",
		"43", "req-multipart",
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected upstream body: %s", body)
	}
	if gotContentType != "multipart/form-data; boundary=test-boundary" {
		t.Fatalf("content type was not preserved: %q", gotContentType)
	}
	if gotAuthorization != "Bearer app-secret" || gotOwner != "43" || gotSatellite != "agentapi" || gotRequestID != "req-multipart" {
		t.Fatalf("unexpected relay headers: authorization=%q on_behalf_of=%q satellite=%q request_id=%q", gotAuthorization, gotOwner, gotSatellite, gotRequestID)
	}
}

func TestRelayModelDoesNotFallBackToOwnerWhenSessionIdentityIsMissing(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cfg := Config{
		MainModelBaseURL:   upstream.URL + "/v1",
		AppCredential:      "app-secret",
		OwnerMainUserID:    "42",
		SatelliteSlug:      "agentapi",
		MainRequestTimeout: time.Second,
	}
	_, _, _, err := NewMainClient(cfg).RelayModelMethod(
		t.Context(), http.MethodPost, "/v1/chat/completions", nil, []byte(`{"model":"gpt-5.5"}`), "", "req-no-session-user",
	)
	var apiErr *MainAPIError
	if !errors.As(err, &apiErr) || apiErr.Code != "MAIN_USER_MISSING" || upstreamCalls != 0 {
		t.Fatalf("missing Session identity was not rejected before relay: err=%v upstream_calls=%d", err, upstreamCalls)
	}
}
