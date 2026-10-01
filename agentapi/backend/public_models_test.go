package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicModelCatalogIsAnonymousTenantFilteredAndLocal(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		t.Fatalf("public model catalog must not call upstream: %s", r.URL.Path)
	}))
	defer upstream.Close()

	server := testServer(t, upstream)
	req := httptest.NewRequest(http.MethodGet, "http://agent.local/api/v1/public/models", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public models status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control=%q, want no-store", got)
	}
	var payload struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Object != "list" || len(payload.Data) != len(publicModelCatalog) {
		t.Fatalf("unexpected public model payload: %s", rec.Body.String())
	}
	for _, item := range payload.Data {
		if item.OwnedBy != "agentapi" {
			t.Fatalf("unexpected owner metadata: %+v", item)
		}
		if _, ok := publicModelNames[item.ID]; !ok {
			t.Fatalf("private model leaked: %q", item.ID)
		}
	}
	if upstreamCalls != 0 {
		t.Fatalf("public catalog called upstream %d times", upstreamCalls)
	}
}

func TestPublicModelCatalogRejectsWritesWithoutCreatingAnAuthFallback(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testServer(t, upstream)

	req := httptest.NewRequest(http.MethodPost, "http://agent.local/api/v1/public/models", strings.NewReader(`{"model":"gpt-5.5"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("public model write status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "login") || strings.Contains(rec.Body.String(), "session") {
		t.Fatalf("public catalog write unexpectedly entered an auth fallback: %s", rec.Body.String())
	}
}
