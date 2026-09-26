package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AgentProvisioningHandler struct {
	service *service.AgentProvisioningService
}

func NewAgentProvisioningHandler(agentProvisioningService *service.AgentProvisioningService) *AgentProvisioningHandler {
	return &AgentProvisioningHandler{service: agentProvisioningService}
}

func (h *AgentProvisioningHandler) RuntimeAuthMiddleware() gin.HandlerFunc {
	if h == nil {
		return middleware.NewAgentRuntimeAuthMiddleware(nil)
	}
	return middleware.NewAgentRuntimeAuthMiddleware(h.service)
}

type createAgentProvisioningRequest struct {
	RequestedSlug   string          `json:"requested_slug"`
	DisplayName     string          `json:"display_name"`
	OwnerMainUserID json.RawMessage `json:"owner_main_user_id"`
	PlanID          string          `json:"plan_id"`
	Brand           struct {
		Name    string  `json:"name"`
		LogoURL *string `json:"logo_url"`
	} `json:"brand"`
	DomainMode string `json:"domain_mode"`
}

type agentProvisioningConfirmationRequest struct {
	ConfirmAgentID     string `json:"confirm_agent_id"`
	ReadinessConfirmed bool   `json:"readiness_confirmed"`
}

type agentProvisioningProgressRequest struct {
	Step        string `json:"step"`
	FailureCode string `json:"failure_code,omitempty"`
	FailureStep string `json:"failure_step,omitempty"`
	Retryable   bool   `json:"retryable,omitempty"`
}

type agentProvisioningBrandResponse struct {
	Name    string  `json:"name"`
	LogoURL *string `json:"logo_url,omitempty"`
}

type agentProvisioningResponse struct {
	AgentID         string                          `json:"agent_id"`
	Status          string                          `json:"status"`
	Domain          string                          `json:"domain"`
	RequestID       string                          `json:"request_id"`
	StatusURL       string                          `json:"status_url"`
	Slug            string                          `json:"slug,omitempty"`
	DisplayName     string                          `json:"display_name,omitempty"`
	OwnerMainUserID int64                           `json:"owner_main_user_id,omitempty"`
	PlanID          string                          `json:"plan_id,omitempty"`
	Brand           *agentProvisioningBrandResponse `json:"brand,omitempty"`
	CurrentStep     string                          `json:"current_step,omitempty"`
	RecentError     string                          `json:"recent_error,omitempty"`
	CanRetry        bool                            `json:"can_retry"`
	DomainStatus    string                          `json:"domain_status,omitempty"`
	CreatedAt       time.Time                       `json:"created_at"`
	UpdatedAt       time.Time                       `json:"updated_at"`
}

type agentProvisioningListResponse struct {
	Items    []service.AgentProvisioningAgent `json:"items"`
	Total    int                              `json:"total"`
	Page     int                              `json:"page"`
	PageSize int                              `json:"page_size"`
}

type agentProvisioningLeaseResponse struct {
	Agent      *service.AgentProvisioningAgent `json:"agent"`
	LeaseToken string                          `json:"lease_token,omitempty"`
	ExpiresAt  time.Time                       `json:"lease_expires_at"`
}

type agentRuntimeCredentialRegistrationRequest struct {
	ControlTokenHash string `json:"control_token_hash"`
	ModelTokenHash   string `json:"model_token_hash"`
}

// Create creates an idempotent provisioning record and returns immediately.
// It does not imply that DNS, TLS, Docker, or the AgentAPI instance is ready.
func (h *AgentProvisioningHandler) Create(c *gin.Context) {
	middleware.SetAuditAction(c, "agent_provisioning.agent.create")
	var request createAgentProvisioningRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "invalid agent provisioning request")
		return
	}
	ownerID, err := parseAgentProvisioningOwnerID(request.OwnerMainUserID)
	if err != nil {
		response.BadRequest(c, "invalid owner_main_user_id")
		return
	}
	brandName := strings.TrimSpace(request.Brand.Name)
	if brandName == "" {
		brandName = request.DisplayName
	}
	input := service.AgentProvisioningCreateInput{
		RequestedSlug:   request.RequestedSlug,
		DisplayName:     request.DisplayName,
		OwnerMainUserID: ownerID,
		PlanID:          request.PlanID,
		BrandName:       brandName,
		LogoURL:         request.Brand.LogoURL,
		DomainMode:      request.DomainMode,
	}
	agent, replayed, err := h.service.CreateAgent(c.Request.Context(), authenticatedAdminID(c), c.GetHeader("Idempotency-Key"), agentProvisioningRequestID(c), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result := agentProvisioningResponseFrom(agent, false)
	if replayed {
		response.Success(c, result)
		return
	}
	response.Accepted(c, result)
}

// Get returns the current control-plane record without exposing deployment credentials.
func (h *AgentProvisioningHandler) Get(c *gin.Context) {
	middleware.SetAuditAction(c, "agent_provisioning.agent.read")
	agent, err := h.service.GetAgent(c.Request.Context(), c.Param("agent_id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentProvisioningResponseFrom(agent, true))
}

func (h *AgentProvisioningHandler) List(c *gin.Context) {
	middleware.SetAuditAction(c, "agent_provisioning.agent.list")
	page, err := parseAgentProvisioningQueryInt(c.Query("page"), 1, 1, 1_000_000)
	if err != nil {
		response.BadRequest(c, "invalid page")
		return
	}
	pageSize, err := parseAgentProvisioningQueryInt(c.Query("page_size"), 20, 1, 100)
	if err != nil {
		response.BadRequest(c, "invalid page_size")
		return
	}
	filter := service.AgentProvisioningListFilter{
		Page: page, PageSize: pageSize, Status: c.Query("status"), Search: c.Query("q"),
	}
	items, total, err := h.service.ListAgents(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentProvisioningListResponse{Items: items, Total: total, Page: page, PageSize: pageSize})
}

// Stream emits post-commit database notifications as Server-Sent Events. The
// event contains only the Agent ID; consumers must GET authoritative state.
func (h *AgentProvisioningHandler) Stream(c *gin.Context) {
	middleware.SetAuditAction(c, "agent_provisioning.agent.stream")
	updates, err := h.service.ListenAgentUpdates(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Agent provisioning change stream is unavailable")
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
		case agentID, ok := <-updates:
			if !ok {
				return
			}
			// SSE is only a wake-up hint. Consumers must GET the authoritative
			// record themselves instead of trusting state copied into an event.
			c.SSEvent("agent", gin.H{"agent_id": agentID})
			c.Writer.Flush()
		case <-ticker.C:
			c.SSEvent("heartbeat", time.Now().UTC().Format(time.RFC3339))
			c.Writer.Flush()
		}
	}
}

// Claim atomically leases one pending/provisioning Agent to a single worker.
// The raw token is returned once; the database stores only its hash.
func (h *AgentProvisioningHandler) Claim(c *gin.Context) {
	middleware.SetAuditAction(c, "agent_provisioning.agent.claim")
	// The lease token is returned once to the worker. Prevent proxies and
	// browsers from retaining a credential-bearing claim response.
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	if !requireAgentProvisioningWorker(c) {
		return
	}
	lease, err := h.service.ClaimAgent(c.Request.Context(), c.Param("agent_id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentProvisioningLeaseResponse{Agent: lease.Agent, LeaseToken: lease.Token, ExpiresAt: lease.ExpiresAt})
}

// RenewLease extends an unexpired claim. Heartbeats live in a separate table
// from Agent state and therefore do not flood the provisioning change stream.
func (h *AgentProvisioningHandler) RenewLease(c *gin.Context) {
	middleware.SetAuditAction(c, "agent_provisioning.agent.lease.renew")
	if !requireAgentProvisioningWorker(c) {
		return
	}
	lease, err := h.service.RenewAgentLease(c.Request.Context(), c.Param("agent_id"), c.GetHeader("X-AgentAPI-Provisioning-Lease"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentProvisioningLeaseResponse{Agent: lease.Agent, ExpiresAt: lease.ExpiresAt})
}

// RegisterRuntimeCredentials accepts only hashes from the leased provisioning
// worker. Raw credentials remain in the generated instance Secret files.
func (h *AgentProvisioningHandler) RegisterRuntimeCredentials(c *gin.Context) {
	middleware.SetAuditAction(c, "agent_provisioning.runtime_credentials.register")
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	if !requireAgentProvisioningWorker(c) {
		return
	}
	var request agentRuntimeCredentialRegistrationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "invalid runtime credential registration request")
		return
	}
	if err := h.service.RegisterRuntimeCredentialHashes(
		c.Request.Context(), c.Param("agent_id"), request.ControlTokenHash, request.ModelTokenHash,
		c.GetHeader("X-AgentAPI-Provisioning-Lease"),
	); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"registered": true})
}

// RevokeRuntimeCredentials is a platform-admin operation. Revocation is
// permanent for the current token hashes; issuing replacements requires a new
// provisioning secret pair and a fresh valid worker lease.
func (h *AgentProvisioningHandler) RevokeRuntimeCredentials(c *gin.Context) {
	middleware.SetAuditAction(c, "agent_provisioning.runtime_credentials.revoke")
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	if c.GetString("auth_method") == "agent_provisioning_worker" {
		response.ErrorWithDetails(c, http.StatusForbidden, "provisioning worker identity cannot revoke runtime credentials", "FORBIDDEN", nil)
		return
	}
	if err := h.service.RevokeRuntimeCredentials(c.Request.Context(), c.Param("agent_id")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"revoked": true})
}

// Progress accepts a single ordered checkpoint from the external deployment
// worker. Arbitrary error text is intentionally not accepted; the service maps
// a bounded failure code to a safe message before persisting it.
func (h *AgentProvisioningHandler) Progress(c *gin.Context) {
	middleware.SetAuditAction(c, "agent_provisioning.agent.progress")
	if !requireAgentProvisioningWorker(c) {
		return
	}
	var request agentProvisioningProgressRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "invalid agent provisioning progress request")
		return
	}
	agent, _, err := h.service.ReportProgress(
		c.Request.Context(), authenticatedAdminID(c), c.GetHeader("Idempotency-Key"),
		agentProvisioningRequestID(c), service.AgentProvisioningProgress{
			AgentID: c.Param("agent_id"), Step: request.Step,
			FailureCode: request.FailureCode, FailureStep: request.FailureStep, Retryable: request.Retryable,
		}, c.GetHeader("X-AgentAPI-Provisioning-Lease"),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, agentProvisioningResponseFrom(agent, true))
}

func parseAgentProvisioningQueryInt(raw string, fallback, minimum, maximum int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, strconv.ErrSyntax
	}
	return value, nil
}

func (h *AgentProvisioningHandler) Suspend(c *gin.Context) {
	h.transition(c, "suspend", "agent_provisioning.agent.suspend", "")
}
func (h *AgentProvisioningHandler) Retry(c *gin.Context) {
	h.transition(c, "retry", "agent_provisioning.agent.retry", "")
}
func (h *AgentProvisioningHandler) Resume(c *gin.Context) {
	h.transition(c, "resume", "agent_provisioning.agent.resume", "")
}

// Activate is an explicit operator confirmation that the independently
// deployed instance passed its health/readiness checks. Automated deployment
// workers can call the same idempotent endpoint after verifying /readyz.
func (h *AgentProvisioningHandler) Activate(c *gin.Context) {
	middleware.SetAuditAction(c, "agent_provisioning.agent.activate")
	var request agentProvisioningConfirmationRequest
	if err := c.ShouldBindJSON(&request); err != nil || !request.ReadinessConfirmed {
		response.BadRequest(c, "activation requires readiness_confirmed=true and confirm_agent_id")
		return
	}
	if c.GetString("auth_method") == "agent_provisioning_worker" && strings.TrimSpace(c.GetHeader("X-AgentAPI-Provisioning-Lease")) == "" {
		response.ErrorWithDetails(c, http.StatusConflict, "a valid worker lease is required to activate", "AGENT_PROVISIONING_LEASE_LOST", nil)
		return
	}
	h.transition(c, "activate", "agent_provisioning.agent.activate", request.ConfirmAgentID)
}

func (h *AgentProvisioningHandler) Revoke(c *gin.Context) {
	middleware.SetAuditAction(c, "agent_provisioning.agent.revoke")
	var request agentProvisioningConfirmationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "revoke requires confirm_agent_id")
		return
	}
	h.transition(c, "revoke", "agent_provisioning.agent.revoke", request.ConfirmAgentID)
}

func (h *AgentProvisioningHandler) transition(c *gin.Context, operation, auditAction, confirmation string) {
	middleware.SetAuditAction(c, auditAction)
	agent, replayed, err := h.service.TransitionAgent(
		c.Request.Context(), authenticatedAdminID(c), c.GetHeader("Idempotency-Key"),
		agentProvisioningRequestID(c), operation, c.Param("agent_id"), confirmation, c.GetHeader("X-AgentAPI-Provisioning-Lease"),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	_ = replayed // The current authoritative state is returned for both first use and safe replay.
	response.Success(c, agentProvisioningResponseFrom(agent, true))
}

func requireAgentProvisioningWorker(c *gin.Context) bool {
	if c.GetString("auth_method") == "agent_provisioning_worker" {
		return true
	}
	response.ErrorWithDetails(c, http.StatusForbidden, "provisioning worker identity required", "FORBIDDEN", nil)
	return false
}

func authenticatedAdminID(c *gin.Context) int64 {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		return 0
	}
	return subject.UserID
}

func agentProvisioningRequestID(c *gin.Context) string {
	if requestID := strings.TrimSpace(c.Writer.Header().Get("X-Request-ID")); requestID != "" {
		return requestID
	}
	return c.GetHeader("X-Request-ID")
}

func parseAgentProvisioningOwnerID(raw json.RawMessage) (int64, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return 0, strconv.ErrSyntax
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		text = strings.TrimPrefix(text, "u_")
		if text == "" {
			return 0, strconv.ErrSyntax
		}
		return strconv.ParseInt(text, 10, 64)
	}
	return strconv.ParseInt(value, 10, 64)
}

func agentProvisioningResponseFrom(agent *service.AgentProvisioningAgent, includeDetails bool) agentProvisioningResponse {
	if agent == nil {
		return agentProvisioningResponse{}
	}
	result := agentProvisioningResponse{
		AgentID:   agent.AgentID,
		Status:    agent.Status,
		Domain:    agent.Domain,
		RequestID: agent.RequestID,
		StatusURL: "/api/v1/agent-provisioning/agents/" + agent.AgentID,
		CreatedAt: agent.CreatedAt,
		UpdatedAt: agent.UpdatedAt,
	}
	if includeDetails {
		result.Slug = agent.Slug
		result.DisplayName = agent.DisplayName
		result.OwnerMainUserID = agent.OwnerMainUserID
		result.PlanID = agent.PlanID
		result.Brand = &agentProvisioningBrandResponse{Name: agent.BrandName, LogoURL: agent.LogoURL}
		result.CurrentStep = agent.CurrentStep
		result.RecentError = agent.RecentError
		result.CanRetry = agent.CanRetry
		result.DomainStatus = agent.DomainStatus
	}
	return result
}
