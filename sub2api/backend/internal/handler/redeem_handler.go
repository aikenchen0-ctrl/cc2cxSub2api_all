package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RedeemHandler handles redeem code-related requests
type RedeemHandler struct {
	redeemService *service.RedeemService
}

// NewRedeemHandler creates a new RedeemHandler
func NewRedeemHandler(redeemService *service.RedeemService) *RedeemHandler {
	return &RedeemHandler{
		redeemService: redeemService,
	}
}

// RedeemRequest represents the redeem code request payload
type RedeemRequest struct {
	Code string `json:"code" binding:"required"`
}

// RedeemResponse represents the redeem response
type RedeemResponse struct {
	Message        string   `json:"message"`
	Type           string   `json:"type"`
	Value          float64  `json:"value"`
	NewBalance     *float64 `json:"new_balance,omitempty"`
	NewConcurrency *int     `json:"new_concurrency,omitempty"`
}

// Redeem handles redeeming a code
// POST /api/v1/redeem
func (h *RedeemHandler) Redeem(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req RedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	h.redeemForUser(c, subject.UserID, req.Code)
}

// SatelliteRedeem exposes the same redemption operation through the gateway
// authentication chain. The effective user comes exclusively from the
// satellite on-behalf-of API key resolved by middleware; no administrator
// identity or browser-supplied user ID is accepted.
// POST /v1/sub2api/redeem
func (h *RedeemHandler) SatelliteRedeem(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.User == nil {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req RedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	h.redeemForUser(c, apiKey.User.ID, req.Code)
}

func (h *RedeemHandler) redeemForUser(c *gin.Context, userID int64, code string) {
	result, err := h.redeemService.Redeem(c.Request.Context(), userID, code)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.RedeemCodeFromService(result))
}

// GetHistory returns the user's redemption history
// GET /api/v1/redeem/history
func (h *RedeemHandler) GetHistory(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	h.historyForUser(c, subject.UserID)
}

// SatelliteGetHistory returns only the current satellite user's redemption
// history. It deliberately reuses the same field-whitelisted DTO as the panel
// endpoint and never accepts a user selector.
// GET /v1/sub2api/redeem/history
func (h *RedeemHandler) SatelliteGetHistory(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.User == nil {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	h.historyForUser(c, apiKey.User.ID)
}

func (h *RedeemHandler) historyForUser(c *gin.Context, userID int64) {
	// Keep the same bounded history window as the main-site user page.
	limit := 25

	codes, err := h.redeemService.GetUserHistory(c.Request.Context(), userID, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.RedeemCode, 0, len(codes))
	for i := range codes {
		out = append(out, *dto.RedeemCodeFromService(&codes[i]))
	}
	response.Success(c, out)
}
