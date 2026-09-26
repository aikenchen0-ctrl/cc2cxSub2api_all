package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/satellite"
	"github.com/gin-gonic/gin"
)

// AgentRuntimeHandler implements the deliberately narrow, per-Agent control
// API used by a managed AgentAPI instance. It never returns Sub2API admin DTOs.
type AgentRuntimeHandler struct {
	provisioning *service.AgentProvisioningService
	users        *service.UserService
	usage        *service.UsageService
}

func NewAgentRuntimeHandler(provisioning *service.AgentProvisioningService, users *service.UserService, usage *service.UsageService) *AgentRuntimeHandler {
	return &AgentRuntimeHandler{provisioning: provisioning, users: users, usage: usage}
}

func (h *AgentRuntimeHandler) GetAgent(c *gin.Context) {
	agentID, _, ok := agentRuntimeIdentity(c)
	if !ok {
		return
	}
	agent, err := h.provisioning.GetAgent(c.Request.Context(), agentID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agent)
}

// StreamAgent forwards only post-commit change hints for the authenticated
// Agent. The caller must GET /agent for authoritative state after a hint.
func (h *AgentRuntimeHandler) StreamAgent(c *gin.Context) {
	agentID, _, ok := agentRuntimeIdentity(c)
	if !ok {
		return
	}
	updates, err := h.provisioning.ListenAgentUpdates(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Agent runtime change stream is unavailable")
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(c.Writer, "retry: 3000\n\nevent: resync\ndata: {}\n\n")
	c.Writer.Flush()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case changedAgentID, open := <-updates:
			if !open {
				return
			}
			if strings.TrimSpace(changedAgentID) != agentID {
				continue
			}
			c.SSEvent("agent", gin.H{"agent_id": agentID})
			c.Writer.Flush()
		case <-ticker.C:
			c.SSEvent("heartbeat", time.Now().UTC().Format(time.RFC3339))
			c.Writer.Flush()
		}
	}
}

func (h *AgentRuntimeHandler) GetOwner(c *gin.Context) {
	_, ownerID, ok := agentRuntimeIdentity(c)
	if !ok {
		return
	}
	owner, err := h.users.GetByID(c.Request.Context(), ownerID)
	if err != nil || owner == nil || owner.Status != "active" {
		response.ErrorFrom(c, service.ErrUserNotFound)
		return
	}
	response.Success(c, gin.H{
		"id": owner.ID, "email": owner.Email, "username": owner.Username,
		"balance": owner.Balance, "status": owner.Status,
	})
}

func (h *AgentRuntimeHandler) MapUser(c *gin.Context) {
	agentID, _, ok := agentRuntimeIdentity(c)
	if !ok {
		return
	}
	userID, err := runtimeUserID(c)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}
	if !runtimeMapIdentityMatches(c, userID) {
		response.Error(c, http.StatusForbidden, "a valid Sub2API user identity proof is required")
		return
	}
	user, err := h.users.GetByID(c.Request.Context(), userID)
	if err != nil || user == nil || user.Status != "active" {
		response.ErrorFrom(c, service.ErrUserNotFound)
		return
	}
	if err := h.provisioning.MapRuntimeUser(c.Request.Context(), agentID, userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"agent_id": agentID, "user_id": userID, "status": "mapped"})
}

func runtimeMapIdentityMatches(c *gin.Context, userID int64) bool {
	if subject, authenticated := middleware.GetAuthSubjectFromContext(c); authenticated {
		if subject.UserID != userID {
			return false
		}
		if ticket := strings.TrimSpace(c.GetHeader("X-AgentAPI-SSO-Ticket")); ticket != "" {
			return validAgentRuntimeSSOIdentityTicket(ticket, userID, time.Now().UTC())
		}
		return true
	}
	return validAgentRuntimeSSOIdentityTicket(c.GetHeader("X-AgentAPI-SSO-Ticket"), userID, time.Now().UTC())
}

func validAgentRuntimeSSOIdentityTicket(raw string, userID int64, now time.Time) bool {
	raw = strings.TrimSpace(raw)
	secret := strings.TrimSpace(os.Getenv("SUB2API_SSO_SECRET"))
	app, exists := satellite.Lookup("agentapi")
	if raw == "" || len(raw) > 8192 || len(secret) < 32 || !exists || userID <= 0 {
		return false
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return false
	}
	encodedPayload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(encodedPayload) == 0 || len(encodedPayload) > 4096 {
		return false
	}
	var ticket juSSOTicket
	if err := json.Unmarshal(encodedPayload, &ticket); err != nil {
		return false
	}
	issuedAt, expiresAt := time.Unix(ticket.IssuedAt, 0), time.Unix(ticket.ExpiresAt, 0)
	maxTTL := app.TicketTTL
	if maxTTL <= 0 || maxTTL > 2*time.Minute {
		maxTTL = 2 * time.Minute
	}
	return ticket.Issuer == "sub2api" && ticket.Audience == app.Audience &&
		ticket.Subject == strconv.FormatInt(userID, 10) && strings.TrimSpace(ticket.Nonce) != "" && len(ticket.Nonce) <= 256 &&
		ticket.IssuedAt > 0 && ticket.ExpiresAt > ticket.IssuedAt && expiresAt.After(now) &&
		issuedAt.Before(now.Add(30*time.Second)) && expiresAt.Sub(issuedAt) <= maxTTL
}

func (h *AgentRuntimeHandler) UpdateUserStatus(c *gin.Context) {
	agentID, ownerID, ok := agentRuntimeIdentity(c)
	if !ok {
		return
	}
	userID, err := runtimeUserID(c)
	if err != nil {
		response.BadRequest(c, "invalid user id")
		return
	}
	var request struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || (request.Status != "active" && request.Status != "disabled") {
		response.BadRequest(c, "status must be active or disabled")
		return
	}
	if userID == ownerID && request.Status != "active" {
		response.Error(c, http.StatusConflict, "agent owner cannot be disabled by its runtime")
		return
	}
	mapped, err := h.provisioning.IsRuntimeUser(c.Request.Context(), agentID, userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !mapped {
		response.NotFound(c, "agent user mapping not found")
		return
	}
	if err := h.users.UpdateStatus(c.Request.Context(), userID, request.Status); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"agent_id": agentID, "user_id": userID, "status": request.Status})
}

func (h *AgentRuntimeHandler) UpdateModelPolicy(c *gin.Context) {
	agentID, _, ok := agentRuntimeIdentity(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var request struct {
		Enabled []string `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "invalid Agent model policy")
		return
	}
	if request.Enabled == nil {
		response.BadRequest(c, "enabled must be an array")
		return
	}
	models, err := h.provisioning.UpdateRuntimeModelAllowlist(c.Request.Context(), agentID, request.Enabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"agent_id": agentID, "enabled": models})
}

func (h *AgentRuntimeHandler) ListOwnerUsage(c *gin.Context) {
	_, ownerID, ok := agentRuntimeIdentity(c)
	if !ok {
		return
	}
	requestID := strings.TrimSpace(c.Query("request_id"))
	if requestID == "" || len(requestID) > 128 || strings.ContainsAny(requestID, "\r\n") {
		response.BadRequest(c, "request_id is required")
		return
	}
	params := pagination.PaginationParams{Page: 1, PageSize: 20, SortBy: "created_at", SortOrder: pagination.SortOrderDesc}
	filters := usagestats.UsageLogFilters{UserID: ownerID, RequestID: requestID}
	logs, result, err := h.usage.ListWithFilters(c.Request.Context(), params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
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
			"request_id": item.RequestID, "model": model,
			"total_cost": item.TotalCost, "actual_cost": item.ActualCost,
			"input_tokens": item.InputTokens, "output_tokens": item.OutputTokens,
			"cache_creation_tokens": item.CacheCreationTokens, "cache_read_tokens": item.CacheReadTokens,
			"cache_creation_5m_tokens": item.CacheCreation5mTokens, "cache_creation_1h_tokens": item.CacheCreation1hTokens,
			"input_cost": item.InputCost, "output_cost": item.OutputCost,
			"cache_creation_cost": item.CacheCreationCost, "cache_read_cost": item.CacheReadCost,
			"rate_multiplier": item.RateMultiplier, "long_context_billing_applied": item.LongContextBillingApplied,
			"image_count": item.ImageCount, "image_input_tokens": item.ImageInputTokens,
			"image_input_cost": item.ImageInputCost, "image_output_tokens": item.ImageOutputTokens,
			"image_output_cost": item.ImageOutputCost,
			"upstream_model":    item.UpstreamModel, "upstream_response_model": item.UpstreamResponseModel,
			"upstream_model_mismatch": item.UpstreamModelMismatch,
			"service_tier":            item.ServiceTier, "reasoning_effort": item.ReasoningEffort,
			"inbound_endpoint": item.InboundEndpoint, "request_type": item.EffectiveRequestType().String(),
			"billing_mode": item.BillingMode, "billing_type": item.BillingType,
			"openai_ws_mode": item.OpenAIWSMode, "native_compaction_v2": item.NativeCompactionV2,
			"duration_ms": item.DurationMs, "first_token_ms": item.FirstTokenMs,
			"stream": item.Stream, "image_size": item.ImageSize, "image_input_size": item.ImageInputSize,
			"image_output_size": item.ImageOutputSize, "image_size_source": item.ImageSizeSource,
			"image_size_breakdown": item.ImageSizeBreakdown, "media_type": item.MediaType,
			"cache_ttl_overridden": item.CacheTTLOverridden, "created_at": item.CreatedAt,
		})
	}
	response.Paginated(c, items, result.Total, 1, params.PageSize)
}

func agentRuntimeIdentity(c *gin.Context) (string, int64, bool) {
	agentValue, agentOK := c.Get(middleware.ContextKeyAgentRuntimeID)
	ownerValue, ownerOK := c.Get(middleware.ContextKeyAgentRuntimeOwner)
	agentID, agentTypeOK := agentValue.(string)
	ownerID, ownerTypeOK := ownerValue.(int64)
	if !agentOK || !ownerOK || !agentTypeOK || !ownerTypeOK || strings.TrimSpace(agentID) == "" || ownerID <= 0 {
		response.Error(c, http.StatusUnauthorized, "agent runtime identity is unavailable")
		return "", 0, false
	}
	return agentID, ownerID, true
}

func runtimeUserID(c *gin.Context) (int64, error) {
	value := c.Param("user_id")
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value {
		return 0, service.ErrAgentProvisioningInvalid
	}
	return id, nil
}
