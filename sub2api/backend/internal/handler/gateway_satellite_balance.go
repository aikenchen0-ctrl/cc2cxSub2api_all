package handler

import (
	"net/http"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

type satelliteUserBalanceResponse struct {
	Object  string  `json:"object"`
	Balance float64 `json:"balance"`
}

// SatelliteUserBalance returns only the authenticated API key owner's account
// balance. Satellite servers call this with their app credential and the
// current Sub2API user id; the browser never receives either credential.
func (h *GatewayHandler) SatelliteUserBalance(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.User == nil {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	c.JSON(http.StatusOK, satelliteUserBalanceResponse{
		Object:  "sub2api.user_balance",
		Balance: apiKey.User.Balance,
	})
}
