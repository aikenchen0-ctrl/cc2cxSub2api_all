package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTenantContentPagesStayLocalAndRespectPublicationBoundary(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	server := testServer(t, upstream)
	if _, err := server.store.UpsertUser(server.cfg.AgentID, "43", "member@example.com", "Member"); err != nil {
		t.Fatal(err)
	}
	ownerID, err := server.store.CreateSession("42", json.RawMessage(`{"id":"42"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	memberID, err := server.store.CreateSession("43", json.RawMessage(`{"id":"43"}`), "", "", time.Now().Add(sessionTTL))
	if err != nil {
		t.Fatal(err)
	}
	owner := &http.Cookie{Name: server.cfg.CookieName, Value: ownerID}
	member := &http.Cookie{Name: server.cfg.CookieName, Value: memberID}

	request := func(method, target, body string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://agent.local"+target, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		server.ServeHTTP(response, req)
		return response
	}

	legalPayload := `{"slug":"terms","kind":"legal","title":"本站服务条款","content":"# 条款\n\n仅属于本站。","status":"active","sort_order":1}`
	createdLegal := request(http.MethodPost, "/api/v1/agent/admin/content-pages", legalPayload, owner, "http://agent.local")
	if createdLegal.Code != http.StatusCreated {
		t.Fatalf("create legal page status=%d body=%s", createdLegal.Code, createdLegal.Body.String())
	}

	customPayload := `{"slug":"guide","kind":"custom","title":"本站指南","content":"# 指南","status":"active","sort_order":2}`
	createdCustom := request(http.MethodPost, "/api/v1/agent/admin/content-pages", customPayload, owner, "http://agent.local")
	if createdCustom.Code != http.StatusCreated {
		t.Fatalf("create custom page status=%d body=%s", createdCustom.Code, createdCustom.Body.String())
	}

	publicLegal := request(http.MethodGet, "/api/v1/agent/content/legal/terms", "", nil, "")
	if publicLegal.Code != http.StatusOK || !strings.Contains(publicLegal.Body.String(), "本站服务条款") {
		t.Fatalf("public legal page status=%d body=%s", publicLegal.Code, publicLegal.Body.String())
	}
	customAnonymous := request(http.MethodGet, "/api/v1/agent/content/custom/guide", "", nil, "")
	if customAnonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous custom page status=%d body=%s", customAnonymous.Code, customAnonymous.Body.String())
	}
	customMember := request(http.MethodGet, "/api/v1/agent/content/custom/guide", "", member, "")
	if customMember.Code != http.StatusOK || !strings.Contains(customMember.Body.String(), "本站指南") {
		t.Fatalf("member custom page status=%d body=%s", customMember.Code, customMember.Body.String())
	}
	menu := request(http.MethodGet, "/api/v1/agent/content-pages", "", member, "")
	if menu.Code != http.StatusOK || strings.Contains(menu.Body.String(), "本站服务条款") || !strings.Contains(menu.Body.String(), "本站指南") {
		t.Fatalf("custom page menu leaked legal data: status=%d body=%s", menu.Code, menu.Body.String())
	}

	memberCreate := request(http.MethodPost, "/api/v1/agent/admin/content-pages", customPayload, member, "http://agent.local")
	if memberCreate.Code != http.StatusForbidden {
		t.Fatalf("member managed content pages: status=%d body=%s", memberCreate.Code, memberCreate.Body.String())
	}
	crossOrigin := request(http.MethodPost, "/api/v1/agent/admin/content-pages", customPayload, owner, "https://attacker.example")
	if crossOrigin.Code != http.StatusForbidden {
		t.Fatalf("cross-origin content create status=%d body=%s", crossOrigin.Code, crossOrigin.Body.String())
	}

	draft, err := server.store.CreateContentPage(server.cfg.AgentID, "draft", "legal", "草稿协议", "不可见", "draft", 3)
	if err != nil {
		t.Fatal(err)
	}
	if hidden := request(http.MethodGet, "/api/v1/agent/content/legal/"+draft.Slug, "", nil, ""); hidden.Code != http.StatusNotFound {
		t.Fatalf("draft legal page became public: status=%d body=%s", hidden.Code, hidden.Body.String())
	}
	if _, err := server.store.ContentPageByID("another-agent", draft.ID); err != errNotFound {
		t.Fatalf("cross-tenant page lookup should be hidden, got %v", err)
	}
	if upstreamCalls.Load() != 0 {
		t.Fatalf("tenant content pages called Sub2API %d times", upstreamCalls.Load())
	}
}
