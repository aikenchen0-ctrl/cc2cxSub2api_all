package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type agentRuntimeUsageRepo struct {
	service.UsageLogRepository
	rows    []service.UsageLog
	filters usagestats.UsageLogFilters
}

func (r *agentRuntimeUsageRepo) ListWithFilters(_ context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	r.filters = filters
	return r.rows, &pagination.PaginationResult{Total: int64(len(r.rows)), Page: params.Page, PageSize: params.PageSize}, nil
}

func TestAgentRuntimeModelPolicyUpdatesOnlyItsPerAgentScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &agentProvisioningHandlerRepoStub{}
	provisioning := service.NewAgentProvisioningService(repo)
	handler := NewAgentRuntimeHandler(provisioning, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyAgentRuntimeID, "agt_0123456789abcdef0123456789abcdef")
		c.Set(middleware.ContextKeyAgentRuntimeOwner, int64(42))
		c.Next()
	})
	router.PUT("/api/v1/agent-runtime/model-policy", handler.UpdateModelPolicy)

	request := httptest.NewRequest(http.MethodPut, "/api/v1/agent-runtime/model-policy", strings.NewReader(`{"enabled":["gpt-image-2","gpt-5.5"]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"enabled":["gpt-5.5","gpt-image-2"]`) {
		t.Fatalf("model policy update failed or was not normalized: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/api/v1/agent-runtime/model-policy", strings.NewReader(`{"enabled":["private-model"]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("non-public model scope status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentRuntimeUsageIsOwnerScopedAndReturnsOnlySafeUsageFacts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamModel := "provider-gpt-5.5"
	responseModel := "provider-gpt-5.5-2026-01"
	mismatch := true
	serviceTier := "priority"
	reasoningEffort := "high"
	inboundEndpoint := "/v1/responses"
	durationMs, firstTokenMs := 900, 120
	imageSize := "1024x1024"
	privateEmail := "owner-private@example.com"
	privateIP := "192.0.2.10"
	repo := &agentRuntimeUsageRepo{rows: []service.UsageLog{
		{
			ID: 77, UserID: 42, RequestID: "req-runtime-usage", Model: "gpt-5.5", RequestedModel: "gpt-5.5",
			UpstreamModel: &upstreamModel, UpstreamResponseModel: &responseModel, UpstreamModelMismatch: &mismatch,
			ServiceTier: &serviceTier, ReasoningEffort: &reasoningEffort, InboundEndpoint: &inboundEndpoint,
			InputTokens: 1000, OutputTokens: 250, CacheCreationTokens: 12, CacheReadTokens: 34,
			CacheCreation5mTokens: 8, CacheCreation1hTokens: 4, InputCost: .000000123, OutputCost: .000000456,
			CacheCreationCost: .000000012, CacheReadCost: .000000034, TotalCost: .31, ActualCost: .25,
			RateMultiplier: 1.2, LongContextBillingApplied: true, ImageCount: 1, ImageInputTokens: 5,
			ImageInputCost: .000000005, ImageOutputTokens: 6, ImageOutputCost: .000000006,
			DurationMs: &durationMs, FirstTokenMs: &firstTokenMs, Stream: true,
			ImageSize: &imageSize, ImageSizeBreakdown: map[string]int{"1024x1024": 1},
			IPAddress: &privateIP, User: &service.User{Email: privateEmail}, CreatedAt: time.Now().UTC(),
		},
		{ID: 88, UserID: 99, RequestID: "req-runtime-usage", Model: "must-not-leak"},
	}}
	usage := service.NewUsageService(repo, nil, nil, nil)
	handler := NewAgentRuntimeHandler(nil, nil, usage)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyAgentRuntimeID, "agt_0123456789abcdef0123456789abcdef")
		c.Set(middleware.ContextKeyAgentRuntimeOwner, int64(42))
		c.Next()
	})
	router.GET("/api/v1/agent-runtime/usage", handler.ListOwnerUsage)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/agent-runtime/usage?request_id=req-runtime-usage", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("runtime usage status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.filters.UserID != 42 || repo.filters.RequestID != "req-runtime-usage" {
		t.Fatalf("runtime usage query was not restricted to Owner and request: %+v", repo.filters)
	}
	body := recorder.Body.String()
	for _, expected := range []string{
		`"model":"gpt-5.5"`, `"input_tokens":1000`, `"cache_creation_5m_tokens":8`,
		`"input_cost":1.23e-7`, `"actual_cost":0.25`, `"upstream_model":"provider-gpt-5.5"`,
		`"service_tier":"priority"`, `"reasoning_effort":"high"`, `"duration_ms":900`,
		`"image_size":"1024x1024"`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("runtime usage response missing safe field %s: %s", expected, body)
		}
	}
	for _, forbidden := range []string{privateEmail, privateIP, "must-not-leak", "user_agent", "account_id", "api_key_id"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("runtime usage response leaked private/admin field %q: %s", forbidden, body)
		}
	}
}

func TestAgentRuntimeModelPolicyUpdatesValidatedPerAgentScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &agentProvisioningHandlerRepoStub{}
	handler := NewAgentRuntimeHandler(service.NewAgentProvisioningService(repo), nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyAgentRuntimeID, "agt_0123456789abcdef0123456789abcdef")
		c.Set(middleware.ContextKeyAgentRuntimeOwner, int64(42))
		c.Next()
	})
	router.PUT("/api/v1/agent-runtime/model-policy", handler.UpdateModelPolicy)

	request := httptest.NewRequest(http.MethodPut, "/api/v1/agent-runtime/model-policy", strings.NewReader(`{"enabled":["gpt-image-2","gpt-5.5"]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"enabled":["gpt-5.5","gpt-image-2"]`) {
		t.Fatalf("Agent runtime model policy update status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/api/v1/agent-runtime/model-policy", strings.NewReader(`{"enabled":["provider-private"]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("non-public model scope status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentRuntimeMapUserRequiresProofForTheRequestedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAgentRuntimeHandler(nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/agent-runtime/users/:user_id/map", func(c *gin.Context) {
		c.Set(middleware.ContextKeyAgentRuntimeID, "agt_0123456789abcdef0123456789abcdef")
		c.Set(middleware.ContextKeyAgentRuntimeOwner, int64(7))
		if c.Query("subject") != "" {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 99})
		}
		c.Next()
	}, handler.MapUser)

	for _, test := range []struct {
		name  string
		query string
	}{
		{name: "no identity proof"},
		{name: "different authenticated user", query: "?subject=mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-runtime/users/42/map"+test.query, strings.NewReader(`{}`))
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("unproven or mismatched user was mapped: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAgentRuntimeMapUserAcceptsOnlyValidAgentAPISSOIdentityTicket(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef0123456789abcdef"
	t.Setenv("SUB2API_SSO_SECRET", secret)
	now := time.Now().UTC().Truncate(time.Second)
	valid := juSSOTicket{
		Issuer: "sub2api", Audience: "agentapi", Subject: "42",
		IssuedAt: now.Unix(), ExpiresAt: now.Add(90 * time.Second).Unix(), Nonce: "unique-sso-jti",
	}
	raw, err := signJuTicket(valid, secret)
	if err != nil {
		t.Fatal(err)
	}
	if !validAgentRuntimeSSOIdentityTicket(raw, 42, now) {
		t.Fatal("valid AgentAPI SSO identity ticket was rejected")
	}
	if validAgentRuntimeSSOIdentityTicket(raw, 43, now) {
		t.Fatal("SSO identity ticket was accepted for a different Sub2API user")
	}

	wrongAudience := valid
	wrongAudience.Audience = "another-satellite"
	wrongAudienceRaw, err := signJuTicket(wrongAudience, secret)
	if err != nil {
		t.Fatal(err)
	}
	if validAgentRuntimeSSOIdentityTicket(wrongAudienceRaw, 42, now) {
		t.Fatal("SSO identity ticket for a different satellite was accepted")
	}

	expired := valid
	expired.IssuedAt = now.Add(-3 * time.Minute).Unix()
	expired.ExpiresAt = now.Add(-time.Minute).Unix()
	expiredRaw, err := signJuTicket(expired, secret)
	if err != nil {
		t.Fatal(err)
	}
	if validAgentRuntimeSSOIdentityTicket(expiredRaw, 42, now) {
		t.Fatal("expired SSO identity ticket was accepted")
	}
}
