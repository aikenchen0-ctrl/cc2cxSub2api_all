package handler

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type satelliteUserBalanceResponse struct {
	Object        string  `json:"object"`
	Balance       float64 `json:"balance"`
	FrozenBalance float64 `json:"frozen_balance"`
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
		Object:        "sub2api.user_balance",
		Balance:       apiKey.User.Balance,
		FrozenBalance: apiKey.User.FrozenBalance,
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

func ownerUsageItems(logs []service.UsageLog, ownerID int64, requestID string) []gin.H {
	items := make([]gin.H, 0, len(logs))
	for _, item := range logs {
		if item.UserID != ownerID || item.RequestID != requestID {
			continue
		}
		model := item.RequestedModel
		if model == "" {
			model = item.Model
		}
		items = append(items, gin.H{
			"id":                           item.ID,
			"request_id":                   item.RequestID,
			"model":                        model,
			"total_cost":                   item.TotalCost,
			"actual_cost":                  item.ActualCost,
			"input_tokens":                 item.InputTokens,
			"output_tokens":                item.OutputTokens,
			"cache_creation_tokens":        item.CacheCreationTokens,
			"cache_read_tokens":            item.CacheReadTokens,
			"cache_creation_5m_tokens":     item.CacheCreation5mTokens,
			"cache_creation_1h_tokens":     item.CacheCreation1hTokens,
			"input_cost":                   item.InputCost,
			"output_cost":                  item.OutputCost,
			"cache_creation_cost":          item.CacheCreationCost,
			"cache_read_cost":              item.CacheReadCost,
			"rate_multiplier":              item.RateMultiplier,
			"long_context_billing_applied": item.LongContextBillingApplied,
			"image_count":                  item.ImageCount,
			"image_input_tokens":           item.ImageInputTokens,
			"image_input_cost":             item.ImageInputCost,
			"image_output_tokens":          item.ImageOutputTokens,
			"image_output_cost":            item.ImageOutputCost,
			"upstream_model":               item.UpstreamModel,
			"upstream_response_model":      item.UpstreamResponseModel,
			"upstream_model_mismatch":      item.UpstreamModelMismatch,
			"service_tier":                 item.ServiceTier,
			"reasoning_effort":             item.ReasoningEffort,
			"inbound_endpoint":             item.InboundEndpoint,
			"request_type":                 item.EffectiveRequestType().String(),
			"billing_mode":                 item.BillingMode,
			"billing_type":                 item.BillingType,
			"openai_ws_mode":               item.OpenAIWSMode,
			"native_compaction_v2":         item.NativeCompactionV2,
			"duration_ms":                  item.DurationMs,
			"first_token_ms":               item.FirstTokenMs,
			"stream":                       item.Stream,
			"image_size":                   item.ImageSize,
			"image_input_size":             item.ImageInputSize,
			"image_output_size":            item.ImageOutputSize,
			"image_size_source":            item.ImageSizeSource,
			"image_size_breakdown":         item.ImageSizeBreakdown,
			"media_type":                   item.MediaType,
			"cache_ttl_overridden":         item.CacheTTLOverridden,
			"created_at":                   item.CreatedAt,
		})
	}
	return items
}
