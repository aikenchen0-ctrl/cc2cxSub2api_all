package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type agentProvisioningHandlerRepoStub struct {
	created             service.AgentProvisioningAgent
	createCalls         int
	transitionCalls     int
	transitionAction    string
	transitionAgentID   string
	transitionRequestID string
	progressCalls       int
	progress            service.AgentProvisioningProgress
	progressRequestID   string
	claimAgentID        string
	renewAgentID        string
	leaseToken          string
}

func (r *agentProvisioningHandlerRepoStub) CreateAgent(_ context.Context, actorUserID int64, _, _, requestID string, agent service.AgentProvisioningAgent) (*service.AgentProvisioningAgent, bool, error) {
	r.createCalls++
	agent.CreatedByUserID = actorUserID
	agent.RequestID = requestID
	r.created = agent
	return &agent, false, nil
}

func (r *agentProvisioningHandlerRepoStub) GetAgent(_ context.Context, agentID string) (*service.AgentProvisioningAgent, error) {
	agent := r.created
	agent.AgentID = agentID
	return &agent, nil
}

func (r *agentProvisioningHandlerRepoStub) ListAgents(_ context.Context, _ service.AgentProvisioningListFilter) ([]service.AgentProvisioningAgent, int, error) {
	return []service.AgentProvisioningAgent{r.created}, 1, nil
}

func (r *agentProvisioningHandlerRepoStub) ListenAgentUpdates(_ context.Context) (<-chan string, error) {
	updates := make(chan string, 1)
	updates <- r.created.AgentID
	close(updates)
	return updates, nil
}

func (r *agentProvisioningHandlerRepoStub) ClaimAgent(_ context.Context, agentID, tokenHash string, _ time.Duration) (*service.AgentProvisioningAgent, time.Time, error) {
	r.claimAgentID, r.leaseToken = agentID, tokenHash
	agent := r.created
	agent.AgentID = agentID
	if agent.Status == "" {
		agent.Status, agent.CurrentStep = "pending", "queued"
	}
	return &agent, time.Now().Add(time.Minute), nil
}

func (r *agentProvisioningHandlerRepoStub) RenewAgentLease(_ context.Context, agentID, tokenHash string, _ time.Duration) (*service.AgentProvisioningAgent, time.Time, error) {
	r.renewAgentID, r.leaseToken = agentID, tokenHash
	agent := r.created
	agent.AgentID, agent.Status = agentID, "provisioning"
	return &agent, time.Now().Add(time.Minute), nil
}

func (r *agentProvisioningHandlerRepoStub) TransitionAgent(_ context.Context, _ int64, _, _, operation, agentID, requestID, leaseTokenHash string) (*service.AgentProvisioningAgent, bool, error) {
	r.transitionCalls++
	r.transitionAction, r.transitionAgentID, r.transitionRequestID = operation, agentID, requestID
	r.leaseToken = leaseTokenHash
	status := map[string]string{"activate": "active", "suspend": "suspended", "resume": "active", "retry": "pending", "revoke": "revoked"}[operation]
	return &service.AgentProvisioningAgent{AgentID: agentID, Status: status, Domain: "agent-01.cc2.cx", RequestID: "req-transition"}, false, nil
}

func (r *agentProvisioningHandlerRepoStub) ReportProgress(_ context.Context, _ int64, _, _, requestID string, progress service.AgentProvisioningProgress, leaseTokenHash string) (*service.AgentProvisioningAgent, bool, error) {
	r.progressCalls++
	r.progress, r.progressRequestID = progress, requestID
	r.leaseToken = leaseTokenHash
	return &service.AgentProvisioningAgent{AgentID: progress.AgentID, Status: "provisioning", CurrentStep: progress.Step}, false, nil
}

func (r *agentProvisioningHandlerRepoStub) StoreRuntimeCredentialHashes(_ context.Context, _, _, _, leaseTokenHash string) error {
	r.leaseToken = leaseTokenHash
	return nil
}

func (r *agentProvisioningHandlerRepoStub) ResolveRuntimeCredential(_ context.Context, _ string) (*service.AgentRuntimeCredential, error) {
	return nil, nil
}

func (r *agentProvisioningHandlerRepoStub) SetRuntimeModelAllowlist(_ context.Context, _ string, _ []string) error {
	return nil
}

func (r *agentProvisioningHandlerRepoStub) RevokeRuntimeCredentials(_ context.Context, _ string) error {
	return nil
}

func (r *agentProvisioningHandlerRepoStub) MapRuntimeUser(_ context.Context, _ string, _ int64) error {
	return nil
}

func (r *agentProvisioningHandlerRepoStub) IsRuntimeUser(_ context.Context, _ string, _ int64) (bool, error) {
	return false, nil
}

func (r *agentProvisioningHandlerRepoStub) IsActiveRuntimeUser(_ context.Context, _ string, _ int64) (bool, error) {
	return false, nil
}

func TestAgentProvisioningHandlerCreateUsesAdminIdentityAndReturnsPendingTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &agentProvisioningHandlerRepoStub{}
	handler := NewAgentProvisioningHandler(service.NewAgentProvisioningService(repo))
	router := gin.New()
	router.POST("/api/v1/agent-provisioning/agents", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 77})
		handler.Create(c)
	})
	body := `{"requested_slug":"Agent-01","display_name":"Agent 01","owner_main_user_id":"u_42","plan_id":"standard","brand":{"name":"Brand 01","logo_url":null},"domain_mode":"platform_subdomain"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-provisioning/agents", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "create-agent-01")
	req.Header.Set("X-Request-ID", "req-create-01")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	if response.Code != http.StatusAccepted {
		t.Fatalf("Create status = %d, body = %s", response.Code, response.Body.String())
	}
	if repo.createCalls != 1 || repo.created.CreatedByUserID != 77 || repo.created.OwnerMainUserID != 42 {
		t.Fatalf("create repository input = %+v (calls=%d)", repo.created, repo.createCalls)
	}
	for _, expected := range []string{`"status":"pending"`, `"domain":"agent-01.cc2.cx"`, `"request_id":"req-create-01"`, `"status_url":"/api/v1/agent-provisioning/agents/agt_`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("response missing %s: %s", expected, response.Body.String())
		}
	}
}

func TestAgentProvisioningHandlerRevokeRequiresMatchingConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &agentProvisioningHandlerRepoStub{}
	handler := NewAgentProvisioningHandler(service.NewAgentProvisioningService(repo))
	router := gin.New()
	router.POST("/api/v1/agent-provisioning/agents/:agent_id/revoke", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 77})
		handler.Revoke(c)
	})
	agentID := "agt_01234567890123456789012345678901"
	wrong := httptest.NewRequest(http.MethodPost, "/api/v1/agent-provisioning/agents/"+agentID+"/revoke", strings.NewReader(`{"confirm_agent_id":"other"}`))
	wrong.Header.Set("Content-Type", "application/json")
	wrong.Header.Set("Idempotency-Key", "revoke-agent-01")
	wrongResponse := httptest.NewRecorder()
	router.ServeHTTP(wrongResponse, wrong)
	if wrongResponse.Code != http.StatusBadRequest || repo.transitionCalls != 0 {
		t.Fatalf("mismatched revoke confirmation status=%d calls=%d body=%s", wrongResponse.Code, repo.transitionCalls, wrongResponse.Body.String())
	}

	correct := httptest.NewRequest(http.MethodPost, "/api/v1/agent-provisioning/agents/"+agentID+"/revoke", strings.NewReader(`{"confirm_agent_id":"`+agentID+`"}`))
	correct.Header.Set("Content-Type", "application/json")
	correct.Header.Set("Idempotency-Key", "revoke-agent-02")
	correctResponse := httptest.NewRecorder()
	router.ServeHTTP(correctResponse, correct)
	if correctResponse.Code != http.StatusOK || repo.transitionCalls != 1 || !strings.Contains(correctResponse.Body.String(), `"status":"revoked"`) {
		t.Fatalf("confirmed revoke status=%d calls=%d body=%s", correctResponse.Code, repo.transitionCalls, correctResponse.Body.String())
	}
}

func TestAgentProvisioningHandlerActivationRequiresReadinessAndMatchingAgentID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &agentProvisioningHandlerRepoStub{}
	handler := NewAgentProvisioningHandler(service.NewAgentProvisioningService(repo))
	router := gin.New()
	router.POST("/api/v1/agent-provisioning/agents/:agent_id/activate", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 77})
		handler.Activate(c)
	})
	agentID := "agt_01234567890123456789012345678901"
	for _, body := range []string{
		`{"confirm_agent_id":"` + agentID + `"}`,
		`{"confirm_agent_id":"other","readiness_confirmed":true}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-provisioning/agents/"+agentID+"/activate", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "activate-invalid")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || repo.transitionCalls != 0 {
			t.Fatalf("invalid activation status=%d calls=%d body=%s", response.Code, repo.transitionCalls, response.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-provisioning/agents/"+agentID+"/activate", strings.NewReader(`{"confirm_agent_id":"`+agentID+`","readiness_confirmed":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "activate-agent-01")
	request.Header.Set("X-Request-ID", "req-activate-agent-01")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || repo.transitionCalls != 1 || repo.transitionAction != "activate" || repo.transitionAgentID != agentID || repo.transitionRequestID != "req-activate-agent-01" || !strings.Contains(response.Body.String(), `"status":"active"`) {
		t.Fatalf("confirmed activation status=%d calls=%d action=%s agent_id=%s request_id=%s body=%s", response.Code, repo.transitionCalls, repo.transitionAction, repo.transitionAgentID, repo.transitionRequestID, response.Body.String())
	}
}

func TestAgentProvisioningHandlerRetryUsesIdempotentTransition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &agentProvisioningHandlerRepoStub{}
	handler := NewAgentProvisioningHandler(service.NewAgentProvisioningService(repo))
	router := gin.New()
	router.POST("/api/v1/agent-provisioning/agents/:agent_id/retry", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 77})
		handler.Retry(c)
	})
	const agentID = "agt_01234567890123456789012345678901"
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-provisioning/agents/"+agentID+"/retry", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "retry-agent-01")
	request.Header.Set("X-Request-ID", "req-retry-agent-01")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || repo.transitionCalls != 1 || repo.transitionAction != "retry" || repo.transitionAgentID != agentID || repo.transitionRequestID != "req-retry-agent-01" || !strings.Contains(response.Body.String(), `"status":"pending"`) {
		t.Fatalf("retry status=%d calls=%d action=%s agent_id=%s request_id=%s body=%s", response.Code, repo.transitionCalls, repo.transitionAction, repo.transitionAgentID, repo.transitionRequestID, response.Body.String())
	}
}

func TestAgentProvisioningHandlerListsAndStreamsCommittedAgents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &agentProvisioningHandlerRepoStub{created: service.AgentProvisioningAgent{
		AgentID: "agt_01234567890123456789012345678901", Slug: "agent01", Domain: "agent01.cc2.cx", Status: "pending",
	}}
	handler := NewAgentProvisioningHandler(service.NewAgentProvisioningService(repo))

	listRouter := gin.New()
	listRouter.GET("/api/v1/agent-provisioning/agents", handler.List)
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/agent-provisioning/agents?page=2&page_size=10&q=agent&status=pending", nil)
	listResponse := httptest.NewRecorder()
	listRouter.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"page":2`) || !strings.Contains(listResponse.Body.String(), `"total":1`) {
		t.Fatalf("list status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}

	streamRouter := gin.New()
	streamRouter.GET("/api/v1/agent-provisioning/agents/stream", handler.Stream)
	streamRequest := httptest.NewRequest(http.MethodGet, "/api/v1/agent-provisioning/agents/stream", nil)
	streamResponse := httptest.NewRecorder()
	streamRouter.ServeHTTP(streamResponse, streamRequest)
	if streamResponse.Code != http.StatusOK || !strings.Contains(streamResponse.Body.String(), "event: resync") || !strings.Contains(streamResponse.Body.String(), "event:agent") || !strings.Contains(streamResponse.Body.String(), `"agent_id":"`+repo.created.AgentID+`"`) {
		t.Fatalf("stream status=%d body=%s", streamResponse.Code, streamResponse.Body.String())
	}
	if strings.Contains(streamResponse.Body.String(), `"status"`) {
		t.Fatalf("stream exposed an authoritative record instead of an ID-only hint: %s", streamResponse.Body.String())
	}

}

func TestAgentProvisioningHandlerAcceptsSafeProgressAndRejectsFreeformErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const agentID = "agt_01234567890123456789012345678901"
	repo := &agentProvisioningHandlerRepoStub{}
	handler := NewAgentProvisioningHandler(service.NewAgentProvisioningService(repo))
	router := gin.New()
	router.PATCH("/api/v1/agent-provisioning/agents/:agent_id/progress", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 77})
		c.Set("auth_method", "agent_provisioning_worker")
		handler.Progress(c)
	})

	bad := httptest.NewRequest(http.MethodPatch, "/api/v1/agent-provisioning/agents/"+agentID+"/progress", strings.NewReader(`{"step":"failed","failure_code":"Bearer secret-token","retryable":true}`))
	bad.Header.Set("Content-Type", "application/json")
	bad.Header.Set("Idempotency-Key", "worker-progress-bad")
	badResponse := httptest.NewRecorder()
	router.ServeHTTP(badResponse, bad)
	if badResponse.Code != http.StatusBadRequest || repo.progressCalls != 0 {
		t.Fatalf("unsafe progress status=%d calls=%d body=%s", badResponse.Code, repo.progressCalls, badResponse.Body.String())
	}

	request := httptest.NewRequest(http.MethodPatch, "/api/v1/agent-provisioning/agents/"+agentID+"/progress", strings.NewReader(`{"step":"validating"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "worker-progress-good")
	request.Header.Set("X-Request-ID", "req-progress")
	request.Header.Set("X-AgentAPI-Provisioning-Lease", "provisioning-worker-lease-token-0123456789")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || repo.progressCalls != 1 || repo.progress.AgentID != agentID || repo.progress.Step != "validating" || repo.progressRequestID != "req-progress" {
		t.Fatalf("safe progress status=%d calls=%d progress=%+v request_id=%q body=%s", response.Code, repo.progressCalls, repo.progress, repo.progressRequestID, response.Body.String())
	}

	failureRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/agent-provisioning/agents/"+agentID+"/progress", strings.NewReader(`{"step":"failed","failure_code":"deployment_failed","failure_step":"deploying","retryable":true}`))
	failureRequest.Header.Set("Content-Type", "application/json")
	failureRequest.Header.Set("Idempotency-Key", "worker-progress-failed")
	failureRequest.Header.Set("X-Request-ID", "req-progress-failed")
	failureRequest.Header.Set("X-AgentAPI-Provisioning-Lease", "provisioning-worker-lease-token-0123456789")
	failureResponse := httptest.NewRecorder()
	router.ServeHTTP(failureResponse, failureRequest)
	if failureResponse.Code != http.StatusOK || repo.progressCalls != 2 || repo.progress.FailureStep != "deploying" || repo.progress.FailureCode != "deployment_failed" || !repo.progress.Retryable {
		t.Fatalf("failure checkpoint status=%d calls=%d progress=%+v body=%s", failureResponse.Code, repo.progressCalls, repo.progress, failureResponse.Body.String())
	}
}

func TestAgentProvisioningHandlerClaimAndRenewRequireWorkerIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const agentID = "agt_01234567890123456789012345678901"
	repo := &agentProvisioningHandlerRepoStub{created: service.AgentProvisioningAgent{Status: "pending", CurrentStep: "queued"}}
	handler := NewAgentProvisioningHandler(service.NewAgentProvisioningService(repo))
	router := gin.New()
	router.POST("/api/v1/agent-provisioning/agents/:agent_id/claim", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 77})
		c.Set("auth_method", "agent_provisioning_worker")
		handler.Claim(c)
	})
	router.POST("/api/v1/agent-provisioning/agents/:agent_id/lease", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 77})
		c.Set("auth_method", "agent_provisioning_worker")
		handler.RenewLease(c)
	})

	claim := httptest.NewRecorder()
	router.ServeHTTP(claim, httptest.NewRequest(http.MethodPost, "/api/v1/agent-provisioning/agents/"+agentID+"/claim", strings.NewReader(`{}`)))
	if claim.Code != http.StatusOK || repo.claimAgentID != agentID || !strings.Contains(claim.Body.String(), `"lease_token":"`) ||
		claim.Header().Get("Cache-Control") != "no-store" || claim.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("claim status=%d agent=%s body=%s", claim.Code, repo.claimAgentID, claim.Body.String())
	}

	renew := httptest.NewRequest(http.MethodPost, "/api/v1/agent-provisioning/agents/"+agentID+"/lease", strings.NewReader(`{}`))
	renew.Header.Set("X-AgentAPI-Provisioning-Lease", "provisioning-worker-lease-token-0123456789")
	renewResponse := httptest.NewRecorder()
	router.ServeHTTP(renewResponse, renew)
	if renewResponse.Code != http.StatusOK || repo.renewAgentID != agentID || len(repo.leaseToken) != 64 {
		t.Fatalf("renew status=%d agent=%s token_hash=%q body=%s", renewResponse.Code, repo.renewAgentID, repo.leaseToken, renewResponse.Body.String())
	}
}

func TestAgentProvisioningHandlerRuntimeCredentialRegistrationRequiresWorkerLeaseAndReturnsNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &agentProvisioningHandlerRepoStub{}
	h := NewAgentProvisioningHandler(service.NewAgentProvisioningService(repo))
	router := gin.New()
	router.POST("/api/v1/agent-provisioning/agents/:agent_id/runtime-credentials", func(c *gin.Context) {
		c.Set("auth_method", "agent_provisioning_worker")
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 77})
		h.RegisterRuntimeCredentials(c)
	})
	const agentID = "agt_01234567890123456789012345678901"
	body := `{"control_token_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","model_token_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-provisioning/agents/"+agentID+"/runtime-credentials", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-AgentAPI-Provisioning-Lease", "provisioning-worker-lease-token-0123456789")
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, request)
	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("registration status = %d, body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if responseRecorder.Header().Get("Cache-Control") != "no-store" || responseRecorder.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("registration response cache headers = %v", responseRecorder.Header())
	}
	if repo.leaseToken == "" || repo.leaseToken == "provisioning-worker-lease-token-0123456789" {
		t.Fatalf("raw lease token reached repository: %q", repo.leaseToken)
	}
}
