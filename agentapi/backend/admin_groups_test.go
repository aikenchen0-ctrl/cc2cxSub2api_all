package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestKeyGroupsComeFromAdminKeyAndCreateBindsSelectedGroup(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/groups/all":
			if r.Header.Get("x-api-key") != "test-main-admin-key" || r.Header.Get("Authorization") != "" {
				t.Fatalf("group query credential: %#v", r.Header)
			}
			_ = json.NewEncoder(w).Encode(envelope([]any{
				map[string]any{"id": 7, "name": "默认分组", "platform": "openai", "status": "active", "description": "对话", "accounts": []any{map[string]any{"key": "upstream-secret"}}, "notes": "hidden"},
				map[string]any{"id": 8, "name": "停用分组", "status": "disabled"},
			}))
		case "/v1/sub2api/api-keys":
			if r.Header.Get("X-Sub2API-On-Behalf-Of") != "42" || r.Header.Get("x-api-key") != "" {
				t.Fatalf("key create identity: %#v", r.Header)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["group_id"] != float64(7) || body["user_id"] != nil {
				t.Fatalf("create body %#v", body)
			}
			_ = json.NewEncoder(w).Encode(envelope(map[string]any{"id": 11, "group_id": 7, "status": "active", "key": "sk-main-secret"}))
		default:
			t.Fatalf("unexpected %s", r.URL.Path)
		}
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	sessionID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	list := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/api-keys/groups", nil)
	list.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	listRec := httptest.NewRecorder()
	server.ServeHTTP(listRec, list)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "默认分组") || strings.Contains(listRec.Body.String(), "停用分组") || strings.Contains(listRec.Body.String(), "upstream-secret") || strings.Contains(listRec.Body.String(), "hidden") {
		t.Fatalf("groups status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	rejected := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/api-keys", strings.NewReader(`{"name":"desktop","group_id":99}`))
	rejected.Header.Set("Origin", "http://agent.local")
	rejected.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	rejectedRec := httptest.NewRecorder()
	server.ServeHTTP(rejectedRec, rejected)
	if rejectedRec.Code != http.StatusBadRequest {
		t.Fatalf("unknown group status=%d body=%s", rejectedRec.Code, rejectedRec.Body.String())
	}
	create := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/api-keys", strings.NewReader(`{"name":"desktop","group_id":7}`))
	create.Header.Set("Origin", "http://agent.local")
	create.AddCookie(&http.Cookie{Name: server.cfg.CookieName, Value: sessionID})
	createRec := httptest.NewRecorder()
	server.ServeHTTP(createRec, create)
	body := createRec.Body.String()
	if createRec.Code != http.StatusCreated || !strings.Contains(body, `"group_name":"默认分组"`) || strings.Contains(body, "sk-main-secret") {
		t.Fatalf("create status=%d body=%s", createRec.Code, body)
	}
}
