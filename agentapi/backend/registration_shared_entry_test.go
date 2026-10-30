package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSharedEntryRegistrationBindsStationMemberOnBothSites(t *testing.T) {
	const agentID = "agt_95b3bd2700964a009877ccfea6103451"
	var membershipCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/users":
			if r.Header.Get("x-api-key") != "test-main-admin-key" {
				t.Error("registration did not use the server-side admin credential")
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !strings.HasPrefix(stringValue(body["notes"]), "agentapi-registration:") {
				t.Error("registration marker is missing")
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"id": 43, "email": "new@example.com", "role": "user",
			}))
		case "/api/v1/auth/satellite/agentapi/agent-membership":
			membershipCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
				t.Error("membership request did not use the satellite application credential")
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["agent_id"] != agentID || body["main_user_id"] != float64(43) {
				t.Errorf("unexpected membership body: %+v", body)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"agent_id": agentID, "main_user_id": 43, "role": "member",
			}))
		case "/api/v1/auth/login":
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{
				"access_token": "main-access-secret", "user": map[string]any{
					"id": 43, "email": "new@example.com", "username": "New User",
				},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	server := testSharedServer(t, upstream)
	server.cfg.EntryTenants = map[string]string{"shared.example": agentID}
	cfg := server.cfg
	cfg.AgentID = agentID
	cfg.AgentName = "VIP Station"
	cfg.SiteName = "VIP Station"
	cfg.OwnerMainUserID = "42"
	cfg.BillingMode = "user_upstream"
	cfg.AgentDomain = ""
	if err := server.store.UpsertAgent(cfg); err != nil {
		t.Fatal(err)
	}

	options := httptest.NewRecorder()
	server.ServeHTTP(options, httptest.NewRequest(http.MethodGet, "http://shared.example/api/v1/auth/login-options", nil))
	if options.Code != http.StatusOK {
		t.Fatalf("login options status=%d body=%s", options.Code, options.Body.String())
	}
	var optionsBody struct {
		Data struct {
			SharedRuntime  bool `json:"shared_runtime"`
			TenantResolved bool `json:"tenant_resolved"`
			Registration   bool `json:"registration_enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(options.Body.Bytes(), &optionsBody); err != nil {
		t.Fatal(err)
	}
	if optionsBody.Data.SharedRuntime || !optionsBody.Data.TenantResolved || !optionsBody.Data.Registration {
		t.Fatalf("entry host did not resolve to an open station: %+v", optionsBody.Data)
	}

	settings := httptest.NewRecorder()
	server.ServeHTTP(settings, httptest.NewRequest(http.MethodGet, "http://shared.example/api/v1/settings/public", nil))
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), `"registration_enabled":true`) ||
		!strings.Contains(settings.Body.String(), `"site_name":"VIP Station"`) {
		t.Fatalf("entry host public settings are not registration-ready: status=%d body=%s", settings.Code, settings.Body.String())
	}

	register := httptest.NewRequest(http.MethodPost, "http://shared.example/api/v1/auth/register", strings.NewReader(`{"email":"new@example.com","password":"password123","username":"New User"}`))
	register.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, register)
	if recorder.Code != http.StatusOK {
		t.Fatalf("shared registration status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if membershipCalls.Load() != 1 {
		t.Fatalf("membership binding calls=%d", membershipCalls.Load())
	}
	user, err := server.store.User(agentID, "43")
	if err != nil || user.Email != "new@example.com" || user.Status != "active" {
		t.Fatalf("local station membership missing: user=%+v err=%v", user, err)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("unexpected registration cookies: %#v", cookies)
	}
	session, err := server.store.LoadSession(cookies[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	if session.AgentID != agentID || session.MainUserID != "43" || session.Role != tenantRoleMember {
		t.Fatalf("registered user did not get a member session: %+v", session)
	}
	if strings.Contains(recorder.Body.String(), "main-access-secret") {
		t.Fatal("main access token leaked to the browser")
	}
}
