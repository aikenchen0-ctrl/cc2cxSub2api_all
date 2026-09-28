package handler

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
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

// SatelliteUserUsage returns usage rows for one request id, scoped to the
// authenticated API key owner. It reads the existing usage_logs data and does
// not require any Agent runtime/provisioning tables.
func (h *GatewayHandler) SatelliteUserUsage(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.User == nil {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	requestID := strings.TrimSpace(c.Query("request_id"))
	if requestID == "" || len(requestID) > 128 || strings.ContainsAny(requestID, "\r\n") {
		response.BadRequest(c, "request_id is required")
		return
	}
	params := pagination.PaginationParams{Page: 1, PageSize: 20, SortBy: "created_at", SortOrder: pagination.SortOrderDesc}
	filters := usagestats.UsageLogFilters{UserID: apiKey.User.ID, RequestID: requestID}
	logs, result, err := h.usageService.ListWithFilters(c.Request.Context(), params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := ownerUsageItems(logs, apiKey.User.ID, requestID)
	response.Paginated(c, items, result.Total, 1, params.PageSize)
}
