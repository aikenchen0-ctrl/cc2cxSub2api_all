package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminFindUsageDecodesPaginatedUsage(t *testing.T) {
	var gotPath string
	var gotKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		gotKey = r.Header.Get("x-api-key")
		_ = json.NewEncoder(w).Encode(envelope(map[string]any{
			"items": []map[string]any{{
				"id": 77, "request_id": "req-usage", "model": "gpt-5.5", "total_cost": 1.25, "actual_cost": 1.10,
			}},
			"total": 1, "page": 1, "page_size": 20,
		}))
	}))
	defer upstream.Close()
	cfg := Config{MainAPIBaseURL: upstream.URL + "/api/v1", AdminKey: "admin-secret", MainRequestTimeout: time.Second}
	items, err := NewMainClient(cfg).AdminFindUsage(t.Context(), "req-usage")
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "admin-secret" || gotPath != "/api/v1/admin/usage?request_id=req-usage&page=1&page_size=20" {
		t.Fatalf("unexpected admin usage request: path=%q key=%q", gotPath, gotKey)
	}
	if len(items) != 1 || items[0].ID != "77" || items[0].ActualCents != 110 || items[0].TotalCents != 125 {
		t.Fatalf("unexpected decoded usage: %+v", items)
	}
}

func TestRelayModelPreservesMultipartContentTypeAndServerHeaders(t *testing.T) {
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
		"ignored-client-user", "req-multipart",
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
	if gotAuthorization != "Bearer app-secret" || gotOwner != "owner-1" || gotSatellite != "agentapi" || gotRequestID != "req-multipart" {
		t.Fatalf("unexpected relay headers: authorization=%q owner=%q satellite=%q request_id=%q", gotAuthorization, gotOwner, gotSatellite, gotRequestID)
	}
}
