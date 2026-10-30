//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type agentTenantHandlerRepoStub struct {
	tenant            *service.AgentSharedTenant
	membership        *service.AgentSharedTenant
	ensureSharedCalls int
	memberCalls       int
}

func (s *agentTenantHandlerRepoStub) EnsureSharedTenant(_ context.Context, tenant service.AgentSharedTenant) (*service.AgentSharedTenant, error) {
	s.ensureSharedCalls++
	if s.tenant == nil {
		copy := tenant
		s.tenant = &copy
	}
	copy := *s.tenant
	return &copy, nil
}

func (s *agentTenantHandlerRepoStub) EnsureSharedTenantMember(_ context.Context, agentID string, mainUserID int64) (*service.AgentSharedTenant, error) {
	s.memberCalls++
	member := &service.AgentSharedTenant{
		AgentID: agentID, OwnerMainUserID: 42, DisplayName: "Station",
		BrandName: "Station", Status: "active", Role: "member",
	}
	if s.membership != nil {
		copy := *s.membership
		copy.Role = "member"
		member = &copy
	}
	return member, nil
}

func (s *agentTenantHandlerRepoStub) FindActiveSharedTenantMembership(_ context.Context, mainUserID int64) (*service.AgentSharedTenant, error) {
	if s.membership != nil {
		copy := *s.membership
		return &copy, nil
	}
	return nil, service.ErrAgentTenantNotFound
}

func TestAgentAPISSOEnsuresSharedTenantAndSignsTenantClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := strings.Repeat("agentapi-sso-secret-", 2)
	t.Setenv("SUB2API_SSO_SECRET", secret)
	t.Setenv("AGENTAPI_SSO_CALLBACK_URL", "https://agents.example/api/auth/sso/callback")

	repo := &agentTenantHandlerRepoStub{}
	handler := &AuthHandler{
		userService: service.NewUserService(&userHandlerRepoStub{user: &service.User{
			ID: 7, Status: service.StatusActive, Email: "owner@example.com", Username: "Owner Name",
		}}, nil, nil, nil),
		agentTenantSvc: service.NewAgentTenantService(repo),
	}

	issue := func() juSSOTicket {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/integrations/agentapi/start?next=%2Fdashboard", nil)
		ctx.Params = gin.Params{{Key: "slug", Value: "agentapi"}}
		ctx.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7})
		handler.IntegrationSSOStart(ctx)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

		var response struct {
			Data struct {
				RedirectURL string `json:"redirect_url"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		parsed, err := url.Parse(response.Data.RedirectURL)
		require.NoError(t, err)
		require.Equal(t, "agents.example", parsed.Host)
		require.NotContains(t, response.Data.RedirectURL, secret)
		require.NotContains(t, strings.ToLower(response.Data.RedirectURL), "superkey")
		return decodeCanvasSSOTicket(t, parsed.Query().Get("ticket"), secret)
	}

	first := issue()
	require.Regexp(t, `^agt_[0-9a-f]{32}$`, first.AgentID)
	require.Equal(t, "owner", first.AgentRole)
	require.Equal(t, "Owner Name", first.AgentName)
	require.Equal(t, "7", first.Subject)
	require.Equal(t, "/dashboard", first.Next)

	second := issue()
	require.Equal(t, first.AgentID, second.AgentID)
	require.Equal(t, 2, repo.ensureSharedCalls)
}

func TestAgentAPISSOFailsClosedWithoutTenantRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUB2API_SSO_SECRET", strings.Repeat("s", 32))
	t.Setenv("AGENTAPI_SSO_CALLBACK_URL", "https://agents.example/api/auth/sso/callback")
	handler := &AuthHandler{userService: service.NewUserService(&userHandlerRepoStub{user: &service.User{
		ID: 7, Status: service.StatusActive, Email: "owner@example.com", Username: "Owner Name",
	}}, nil, nil, nil)}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/integrations/agentapi/start", nil)
	ctx.Params = gin.Params{{Key: "slug", Value: "agentapi"}}
	ctx.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7})
	handler.IntegrationSSOStart(ctx)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "ticket")
}

func TestAgentAPISSOUsesExistingMemberTenantWithoutCreatingAnother(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := strings.Repeat("agentapi-sso-secret-", 2)
	t.Setenv("SUB2API_SSO_SECRET", secret)
	t.Setenv("AGENTAPI_SSO_CALLBACK_URL", "https://agents.example/api/auth/sso/callback")

	repo := &agentTenantHandlerRepoStub{membership: &service.AgentSharedTenant{
		AgentID: "agt_11111111111111111111111111111111", OwnerMainUserID: 42,
		DisplayName: "Station", BrandName: "Station", Status: "active", Role: "member",
	}}
	handler := &AuthHandler{
		userService: service.NewUserService(&userHandlerRepoStub{user: &service.User{
			ID: 43, Status: service.StatusActive, Email: "member@example.com", Username: "Member",
		}}, nil, nil, nil),
		agentTenantSvc: service.NewAgentTenantService(repo),
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/integrations/agentapi/start?next=%2Fdashboard", nil)
	ctx.Params = gin.Params{{Key: "slug", Value: "agentapi"}}
	ctx.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 43})
	handler.IntegrationSSOStart(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		Data struct {
			RedirectURL string `json:"redirect_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	parsed, err := url.Parse(response.Data.RedirectURL)
	require.NoError(t, err)
	ticket := decodeCanvasSSOTicket(t, parsed.Query().Get("ticket"), secret)
	require.Equal(t, repo.membership.AgentID, ticket.AgentID)
	require.Equal(t, "member", ticket.AgentRole)
	require.Equal(t, "43", ticket.Subject)
	require.Zero(t, repo.ensureSharedCalls)
}

func TestSatelliteAgentMembershipRequiresApplicationCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUB2API_APP_CREDENTIAL", "satellite-secret")
	repo := &agentTenantHandlerRepoStub{}
	handler := &AuthHandler{agentTenantSvc: service.NewAgentTenantService(repo)}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/satellite/agentapi/agent-membership", strings.NewReader(`{"agent_id":"agt_11111111111111111111111111111111","main_user_id":43}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "slug", Value: "agentapi"}}
	ctx.Request.Header.Set(middleware2.HeaderSatelliteApp, "agentapi")
	ctx.Request.Header.Set("Authorization", "Bearer satellite-secret")
	handler.SatelliteAgentMembership(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"role":"member"`)
	require.Equal(t, 1, repo.memberCalls)

	rejected := httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(rejected)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/satellite/agentapi/agent-membership", strings.NewReader(`{"agent_id":"agt_11111111111111111111111111111111","main_user_id":43}`))
	ctx.Params = gin.Params{{Key: "slug", Value: "agentapi"}}
	ctx.Request.Header.Set(middleware2.HeaderSatelliteApp, "agentapi")
	ctx.Request.Header.Set("Authorization", "Bearer main-admin-key")
	handler.SatelliteAgentMembership(ctx)
	require.Equal(t, http.StatusUnauthorized, rejected.Code)
	require.Equal(t, 1, repo.memberCalls)
}
