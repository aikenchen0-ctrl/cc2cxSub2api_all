package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBrowserUserAllowsOnlyPublicProfileFields(t *testing.T) {
	s := &Server{cfg: Config{AgentID: "site-a", OwnerMainUserID: "42"}}
	raw := []byte(`{"id":999,"role":"admin","agent_admin":true,"agent_id":"site-b","email":"member@example.com","username":"Member","display_name":"Display","avatar_url":"/avatar.png","status":"active","notes":"private-marker","super_key":"private-key","access_token":"private-access","refresh_token":"private-refresh","credentials":{"secret":"private-nested"},"balance":999,"allowed_groups":[1]}`)
	for _, id := range []string{"42", "43"} {
		result := s.browserUser(raw, id)
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "private-") || len(result) != 9 {
			t.Fatalf("unexpected public profile fields: %s", encoded)
		}
		if result["role"] != "user" || result["agent_id"] != "site-a" || result["agent_admin"] != (id == "42") || result["email"] != "member@example.com" {
			t.Fatalf("profile or local authority incorrect: %s", encoded)
		}
		if result["id"] == int64(999) {
			t.Fatal("upstream replaced authenticated identity")
		}
	}
}

func TestBrowserUserRejectsNestedProfileValues(t *testing.T) {
	s := &Server{cfg: Config{AgentID: "site-a", OwnerMainUserID: "42"}}
	for _, raw := range []string{`{"email":{"secret":"private"},"username":["private"],"display_name":null,"status":true}`, `null`, `not-json`} {
		result := s.browserUser([]byte(raw), "43")
		if len(result) != 4 || result["agent_admin"] != false || result["id"] != int64(43) {
			t.Fatalf("invalid upstream profile accepted: %+v", result)
		}
	}
}
