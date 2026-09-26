package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const AgentRuntimeControlHeader = "X-AgentAPI-Runtime-Control"

const (
	ContextKeyAgentRuntimeID          = "agent_runtime_agent_id"
	ContextKeyAgentRuntimeOwner       = "agent_runtime_owner_user_id"
	ContextKeyAgentRuntimeSessionUser = "agent_runtime_session_user_id"
)

// NewAgentRuntimeAuthMiddleware authenticates the per-instance control secret
// and only admits the small set of Owner-scoped management capabilities used
// by its matching AgentAPI process.
func NewAgentRuntimeAuthMiddleware(agentProvisioning *service.AgentProvisioningService) gin.HandlerFunc {
	return func(c *gin.Context) {
		credential := strings.TrimSpace(c.GetHeader(AgentRuntimeControlHeader))
		if credential == "" || len(credential) > 256 {
			AbortWithError(c, http.StatusUnauthorized, "AGENT_RUNTIME_CREDENTIAL_REQUIRED", "Agent runtime control credential is required")
			return
		}
		if agentProvisioning == nil {
			AbortWithError(c, http.StatusServiceUnavailable, "AGENT_RUNTIME_AUTH_UNAVAILABLE", "Agent runtime authentication is unavailable")
			return
		}
		identity, err := agentProvisioning.LookupRuntimeCredential(c.Request.Context(), credential)
		if err != nil || identity == nil || identity.Purpose != "control" {
			AbortWithError(c, http.StatusUnauthorized, "AGENT_RUNTIME_CREDENTIAL_INVALID", "Agent runtime control credential is invalid or revoked")
			return
		}
		if !agentRuntimeRouteAllowed(c.Request.Method, c.Request.URL.Path) {
			AbortWithError(c, http.StatusForbidden, "AGENT_RUNTIME_ROUTE_FORBIDDEN", "Agent runtime credential is not allowed for this route")
			return
		}
		c.Set(ContextKeyAgentRuntimeID, identity.AgentID)
		c.Set(ContextKeyAgentRuntimeOwner, identity.OwnerMainUserID)
		c.Next()
		return
	}
}

func agentRuntimeRouteAllowed(method, path string) bool {
	switch {
	case method == http.MethodGet && (path == "/api/v1/agent-runtime/agent" || path == "/api/v1/agent-runtime/agent/stream" || path == "/api/v1/agent-runtime/owner"):
		return true
	case method == http.MethodGet && path == "/api/v1/agent-runtime/usage":
		return true
	case method == http.MethodPut && path == "/api/v1/agent-runtime/model-policy":
		return true
	case method == http.MethodPost && runtimeUserPath(path, "map"):
		return true
	case method == http.MethodPatch && runtimeUserPath(path, "status"):
		return true
	default:
		return false
	}
}

func runtimeUserPath(path, operation string) bool {
	const prefix = "/api/v1/agent-runtime/users/"
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) != 2 || parts[1] != operation {
		return false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == parts[0]
}
