package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestAgentProvisioningRoutesRequireAdminAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminAuth := middleware.AdminAuthMiddleware(func(c *gin.Context) {
		response.Unauthorized(c, "administrator authentication required")
		c.Abort()
	})
	auditLog := middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	h := handler.NewAgentProvisioningHandler(service.NewAgentProvisioningService(nil))
	RegisterAgentProvisioningRoutes(router.Group("/api/v1"), h, adminAuth, auditLog, nil, nil)

	requests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/v1/agent-provisioning/agents"},
		{method: http.MethodPatch, path: "/api/v1/agent-provisioning/agents/agt_01234567890123456789012345678901/progress"},
	}
	for _, test := range requests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			responseRecorder := httptest.NewRecorder()
			router.ServeHTTP(responseRecorder, request)
			if responseRecorder.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated provisioning request status = %d, body = %s", responseRecorder.Code, responseRecorder.Body.String())
			}
		})
	}
}
