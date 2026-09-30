package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOwnerCannotRevokeOtherAgentKeyEvenForSameMainUser(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("key management must not call main") }))
	defer upstream.Close()
	s := testServer(t, upstream)
	if _, err := s.store.UpsertUser("other-agent", "42", "owner@example.com", "Owner"); err != nil {
		t.Fatal(err)
	}
	foreign, raw, err := s.store.CreateAPIKey("other-agent", "42", "foreign")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.store.CreateSession("42", []byte(`{"id":42}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/api-keys/%d?main_user_id=42", foreign.ID), nil)
	req.AddCookie(&http.Cookie{Name: s.cfg.CookieName, Value: session})
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("foreign key revoke status %d %s", rec.Code, rec.Body.String())
	}
	if id, err := s.store.ResolveAPIKey("other-agent", raw); err != nil || id != "42" {
		t.Fatalf("foreign key changed %q %v", id, err)
	}
	if _, err := s.store.ResolveAPIKey(s.cfg.AgentID, raw); err != errNotFound {
		t.Fatalf("foreign key usable locally: %v", err)
	}
}
