package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicRegistrationAvailabilityRequiresActiveSiteAndCredential(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("public settings must not call upstream")
	}))
	defer upstream.Close()
	for _, tc := range []struct {
		name, status, credential string
		want                     bool
	}{
		{"active", "active", "test-admin-key", true},
		{"no credential", "active", "", false},
		{"revoked", "revoked", "test-admin-key", false},
		{"pending", "pending", "test-admin-key", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer(t, upstream)
			s.cfg.MainAdminAPIKey = tc.credential
			if _, err := s.store.db.Exec(`UPDATE agent_config SET status=? WHERE agent_id=?`, tc.status, s.cfg.AgentID); err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/settings/public", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d", rec.Code)
			}
			var body struct {
				Data struct {
					Enabled bool `json:"registration_enabled"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Data.Enabled != tc.want {
				t.Fatalf("enabled=%v want=%v", body.Data.Enabled, tc.want)
			}
		})
	}
}
