package middleware

import (
	"net/http"
	"testing"
)

func TestAgentRuntimeRouteAllowlistScopesAgentStatusStream(t *testing.T) {
	for _, test := range []struct {
		method string
		path   string
		want   bool
	}{
		{method: http.MethodGet, path: "/api/v1/agent-runtime/agent/stream", want: true},
		{method: http.MethodPost, path: "/api/v1/agent-runtime/agent/stream", want: false},
		{method: http.MethodGet, path: "/api/v1/agent-runtime/agent/stream/other", want: false},
		{method: http.MethodPost, path: "/api/v1/agent-runtime/users/42/map", want: true},
		{method: http.MethodPut, path: "/api/v1/agent-runtime/model-policy", want: true},
		{method: http.MethodPatch, path: "/api/v1/agent-runtime/model-policy", want: false},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			if got := agentRuntimeRouteAllowed(test.method, test.path); got != test.want {
				t.Fatalf("agent runtime route allowed=%v, want %v", got, test.want)
			}
		})
	}
}
