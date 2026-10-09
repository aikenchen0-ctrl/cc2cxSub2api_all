package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelRelayCarriesKeyGroupAndIgnoresClientGroup(t *testing.T) {
	var gotGroup, gotBehalf, gotQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sub2api/balance" {
			_ = json.NewEncoder(w).Encode(map[string]any{"balance": 10, "frozen_balance": 0})
			return
		}
		if r.URL.Path == "/v1/chat/completions" {
			gotGroup = r.Header.Get("X-Sub2API-Group-ID")
			gotBehalf = r.Header.Get("X-Sub2API-On-Behalf-Of")
			gotQuery = r.URL.RawQuery
			if r.Header.Get("Authorization") != "Bearer app-secret" || r.Header.Get("X-Sub2API-Satellite") != "agentapi" {
				t.Fatalf("satellite identity missing: %#v", r.Header)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ok"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	view, key, err := server.store.CreateAPIKeyInGroup(server.cfg.AgentID, "42", "grouped", "默认分组", 7)
	if err != nil || view.GroupID != 7 {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?group_id=99", strings.NewReader(`{"model":"gpt-5.5","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("X-Sub2API-Group-ID", "99")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || gotGroup != "7" || gotBehalf != "42" || strings.Contains(gotQuery, "group_id") {
		t.Fatalf("status=%d group=%q behalf=%q query=%q body=%s", rec.Code, gotGroup, gotBehalf, gotQuery, rec.Body.String())
	}
}
