package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterAgentRuntimeRoutes exposes only the authenticated capabilities
// needed by the AgentAPI instance identified by its per-Agent control secret.
func RegisterAgentRuntimeRoutes(v1 *gin.RouterGroup, h *handler.AgentRuntimeHandler, runtimeAuth, optionalJWTAuth gin.HandlerFunc) {
	group := v1.Group("/agent-runtime")
	group.Use(runtimeAuth)
	{
		group.GET("/agent", h.GetAgent)
		group.GET("/agent/stream", h.StreamAgent)
		group.GET("/owner", h.GetOwner)
		group.GET("/usage", h.ListOwnerUsage)
		group.PUT("/model-policy", h.UpdateModelPolicy)
		group.POST("/users/:user_id/map", optionalJWTAuth, h.MapUser)
		group.PATCH("/users/:user_id/status", h.UpdateUserStatus)
	}
}
