package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// RegisterAgentProvisioningRoutes exposes the main-site provisioning control
// plane only to authenticated administrators. It records desired state; an
// external deployment worker remains responsible for DNS/TLS and runtime work.
func RegisterAgentProvisioningRoutes(
	v1 *gin.RouterGroup,
	h *handler.AgentProvisioningHandler,
	adminAuth middleware.AdminAuthMiddleware,
	auditLog middleware.AuditLogMiddleware,
	settingService *service.SettingService,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	group := v1.Group("/agent-provisioning")
	group.Use(gin.HandlerFunc(adminAuth))
	if panelRateLimiter != nil {
		group.Use(panelRateLimiter.Global())
	}
	group.Use(gin.HandlerFunc(auditLog))
	group.Use(middleware.AdminComplianceGuard(settingService))
	{
		group.POST("/agents", h.Create)
		group.GET("/agents", h.List)
		group.GET("/agents/stream", h.Stream)
		group.GET("/agents/:agent_id", h.Get)
		group.POST("/agents/:agent_id/claim", h.Claim)
		group.POST("/agents/:agent_id/lease", h.RenewLease)
		group.POST("/agents/:agent_id/runtime-credentials", h.RegisterRuntimeCredentials)
		group.POST("/agents/:agent_id/runtime-credentials/revoke", h.RevokeRuntimeCredentials)
		group.PATCH("/agents/:agent_id/progress", h.Progress)
		group.POST("/agents/:agent_id/activate", h.Activate)
		group.POST("/agents/:agent_id/suspend", h.Suspend)
		group.POST("/agents/:agent_id/retry", h.Retry)
		group.POST("/agents/:agent_id/resume", h.Resume)
		group.POST("/agents/:agent_id/revoke", h.Revoke)
	}
}
