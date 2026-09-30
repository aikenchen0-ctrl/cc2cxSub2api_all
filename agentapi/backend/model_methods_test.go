package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelRouteMethodsMatchPublicContract(t *testing.T) {
	for path, allowed := range map[string]string{
		"/v1/models":                   http.MethodGet,
		"/v1/chat/completions":         http.MethodPost,
		"/v1/responses":                http.MethodPost,
		"/v1/images/generations":       http.MethodPost,
		"/v1/images/edits":             http.MethodPost,
		"/v1/images/generations/async": http.MethodPost,
		"/v1/images/edits/async":       http.MethodPost,
		"/v1/videos":                   http.MethodPost,
		"/v1/videos/task-123":          http.MethodGet,
		"/v1/images/tasks/task-123":    http.MethodGet,
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead} {
			if got := isAllowedModelPath(path, method); got != (method == allowed) {
				t.Errorf("%s %s allowed=%v", method, path, got)
			}
		}
	}
}

func TestUnsupportedModelMethodsNeverReachMain(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unsupported method reached main: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	_, key, err := s.store.CreateAPIKey(s.cfg.AgentID, "42", "method-test")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/v1/models"},
		{http.MethodGet, "/v1/chat/completions"},
		{http.MethodGet, "/v1/responses"},
		{http.MethodGet, "/v1/images/generations"},
		{http.MethodGet, "/v1/images/edits"},
		{http.MethodGet, "/v1/videos"},
		{http.MethodPost, "/v1/videos/task-123"},
		{http.MethodPost, "/v1/images/tasks/task-123"},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"model":"gpt-5.5"}`))
		r.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "MODEL_ROUTE_FORBIDDEN") {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	usage, err := s.store.Usage(s.cfg.AgentID, "42", 10)
	if err != nil || len(usage) != 0 {
		t.Fatalf("unsupported requests created billing records: %+v %v", usage, err)
	}
}
