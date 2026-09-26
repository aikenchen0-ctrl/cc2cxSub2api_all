package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSatelliteBearerAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUB2API_APP_CREDENTIAL", "satellite-secret")

	require.False(t, SatelliteBearerAccepted(""))
	require.False(t, SatelliteBearerAccepted("wrong"))
	require.True(t, SatelliteBearerAccepted("satellite-secret"))
	require.False(t, service.IsSuperAPIKeyCredential("satellite-secret"))
	require.True(t, service.IsSuperAPIKeyCredential("sk-super-abc"))
}

func TestSatelliteBearerRejectsSuperKeyAsApplicationCredential(t *testing.T) {
	t.Setenv("SUB2API_APP_CREDENTIAL", "sk-super-accidental")
	require.False(t, SatelliteBearerAccepted("sk-super-accidental"))
}

func TestSatelliteBearerRejectsPlaceholderCredentials(t *testing.T) {
	for _, placeholder := range []string{"replace-with-app-credential", "your-api-key", "example-secret", "xxx"} {
		t.Setenv("SUB2API_APP_CREDENTIAL", placeholder)
		require.False(t, SatelliteBearerAccepted(placeholder), placeholder)
	}

	t.Setenv("SUB2API_APP_CREDENTIAL", "real-satellite-secret")
	for _, placeholder := range []string{"replace-with-app-credential", "your-api-key", "example-secret", "xxx"} {
		require.False(t, SatelliteBearerAccepted(placeholder), placeholder)
	}
	require.True(t, SatelliteBearerAccepted("real-satellite-secret"))
}

func TestParseOnBehalfOfUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	_, err := parseOnBehalfOfUserID(c)
	require.Error(t, err)

	c.Request.Header.Set(HeaderOnBehalfOf, "7")
	id, err := parseOnBehalfOfUserID(c)
	require.NoError(t, err)
	require.EqualValues(t, 7, id)
}

func TestBindSatelliteSuperKeyContextMarksCrossGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	namedOnly := &service.APIKey{Name: service.SuperAPIKeyName, Key: "sat-not-a-super-prefix"}
	bindSatelliteSuperKeyContext(c, true, namedOnly)
	require.True(t, service.SuperKeyCrossGroup(c.Request.Context()))
}

func TestBindAgentRuntimeModelScopeKeepsUnconfiguredAndEmptyDistinct(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	bindAgentRuntimeModelScope(ctx, &service.AgentRuntimeCredential{ModelAllowlistWasSet: false})
	models, exists := ctx.Get(ContextKeyAgentRuntimeModelAllowlist)
	if !exists || len(models.([]string)) != len(satellitePublicModelCatalog()) {
		t.Fatalf("unconfigured Agent model scope did not default to public catalog: %v", models)
	}
	scoped, _ := ctx.Get(ContextKeyAgentRuntimeModelScoped)
	if scoped.(bool) {
		t.Fatal("default public catalog must not be mistaken for an explicit scope")
	}

	ctx, _ = gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	bindAgentRuntimeModelScope(ctx, &service.AgentRuntimeCredential{ModelAllowlist: []string{}, ModelAllowlistWasSet: true})
	models, _ = ctx.Get(ContextKeyAgentRuntimeModelAllowlist)
	scoped, _ = ctx.Get(ContextKeyAgentRuntimeModelScoped)
	if !scoped.(bool) || len(models.([]string)) != 0 {
		t.Fatalf("explicit empty model scope must remain deny-all, scope=%v models=%v", scoped, models)
	}
}

func TestRejectSuperKeyAsAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	require.True(t, rejectSuperKeyAsAPIKey(c, "sk-super-leaked"))
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "SUPER_KEY_NOT_AN_API_KEY")
}

type runtimeModelAuthenticatorStub struct {
	credential     *service.AgentRuntimeCredential
	active         bool
	mappingErr     error
	lookupCalls    int
	mappingAgentID string
	mappingUserID  int64
}

func (s *runtimeModelAuthenticatorStub) LookupRuntimeCredential(context.Context, string) (*service.AgentRuntimeCredential, error) {
	s.lookupCalls++
	return s.credential, nil
}

func (s *runtimeModelAuthenticatorStub) IsActiveRuntimeUser(_ context.Context, agentID string, userID int64) (bool, error) {
	s.mappingAgentID, s.mappingUserID = agentID, userID
	return s.active, s.mappingErr
}

type satelliteOwnerKeyResolverStub struct {
	key     *service.APIKey
	userIDs []int64
	err     error
}

func (s *satelliteOwnerKeyResolverStub) SatelliteUserKey(_ context.Context, userID int64) (*service.APIKey, error) {
	s.userIDs = append(s.userIDs, userID)
	return s.key, s.err
}

func newAgentRuntimeModelContext(userID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set(HeaderSatelliteApp, "agentapi")
	if userID != "" {
		c.Request.Header.Set(HeaderOnBehalfOf, userID)
	}
	return c, recorder
}

func newAgentRuntimeModelCredential() *service.AgentRuntimeCredential {
	return &service.AgentRuntimeCredential{
		AgentID: "agt_0123456789abcdef0123456789abcdef", OwnerMainUserID: 42,
		Purpose: "model", AgentStatus: "active", ModelAllowlist: []string{"gpt-5.5"}, ModelAllowlistWasSet: true,
	}
}

func TestAgentRuntimeModelUsesMappedSessionIdentityAndOwnerBillingKey(t *testing.T) {
	c, recorder := newAgentRuntimeModelContext("43")
	keyResolver := &satelliteOwnerKeyResolverStub{key: &service.APIKey{UserID: 42, Name: service.SuperAPIKeyName}}
	runtime := &runtimeModelAuthenticatorStub{credential: newAgentRuntimeModelCredential(), active: true}

	key, handled := loadSatelliteUserKey(c, keyResolver, "agt_model_runtime-secret", runtime)
	if !handled || key == nil {
		t.Fatalf("Agent runtime request was not authenticated: handled=%v key=%+v response=%s", handled, key, recorder.Body.String())
	}
	if key.UserID != 42 || len(keyResolver.userIDs) != 1 || keyResolver.userIDs[0] != 42 {
		t.Fatalf("gateway billing key was not resolved for the Agent owner: key=%+v lookups=%v", key, keyResolver.userIDs)
	}
	if runtime.mappingAgentID != runtime.credential.AgentID || runtime.mappingUserID != 43 {
		t.Fatalf("session identity mapping was not checked: agent=%q user=%d", runtime.mappingAgentID, runtime.mappingUserID)
	}
	if got, ok := c.Get(ContextKeyAgentRuntimeSessionUser); !ok || got != int64(43) {
		t.Fatalf("session identity was not retained in gateway context: got=%v exists=%v", got, ok)
	}
	if got, ok := c.Get(ContextKeyAgentRuntimeOwner); !ok || got != int64(42) {
		t.Fatalf("billing owner was not retained in gateway context: got=%v exists=%v", got, ok)
	}
	bindSatelliteSuperKeyContext(c, handled, key)
	if !service.SuperKeyCrossGroup(c.Request.Context()) {
		t.Fatal("Agent owner billing key lost satellite cross-group routing")
	}
}

func TestAgentRuntimeModelRejectsUnmappedSessionIdentityBeforeLoadingOwnerKey(t *testing.T) {
	c, recorder := newAgentRuntimeModelContext("44")
	keyResolver := &satelliteOwnerKeyResolverStub{key: &service.APIKey{UserID: 42, Name: service.SuperAPIKeyName}}
	runtime := &runtimeModelAuthenticatorStub{credential: newAgentRuntimeModelCredential(), active: false}

	key, handled := loadSatelliteUserKey(c, keyResolver, "agt_model_runtime-secret", runtime)
	if !handled || key != nil || recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "AGENT_RUNTIME_USER_SCOPE_MISMATCH") {
		t.Fatalf("unmapped session user was not rejected: handled=%v key=%+v status=%d body=%s", handled, key, recorder.Code, recorder.Body.String())
	}
	if len(keyResolver.userIDs) != 0 {
		t.Fatalf("owner Super Key was loaded before session mapping validation: %v", keyResolver.userIDs)
	}
}

func TestAgentRuntimeModelFailsClosedWhenSessionMappingCannotBeChecked(t *testing.T) {
	c, recorder := newAgentRuntimeModelContext("43")
	keyResolver := &satelliteOwnerKeyResolverStub{key: &service.APIKey{UserID: 42, Name: service.SuperAPIKeyName}}
	runtime := &runtimeModelAuthenticatorStub{credential: newAgentRuntimeModelCredential(), mappingErr: context.DeadlineExceeded}

	key, handled := loadSatelliteUserKey(c, keyResolver, "agt_model_runtime-secret", runtime)
	if !handled || key != nil || recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "AGENT_RUNTIME_USER_LOOKUP_UNAVAILABLE") {
		t.Fatalf("mapping-store error did not fail closed: handled=%v key=%+v status=%d body=%s", handled, key, recorder.Code, recorder.Body.String())
	}
	if len(keyResolver.userIDs) != 0 {
		t.Fatalf("owner Super Key was loaded after a mapping-store error: %v", keyResolver.userIDs)
	}
}

func TestAgentRuntimeModelRequiresSessionOnBehalfIdentity(t *testing.T) {
	c, recorder := newAgentRuntimeModelContext("")
	keyResolver := &satelliteOwnerKeyResolverStub{key: &service.APIKey{UserID: 42, Name: service.SuperAPIKeyName}}
	runtime := &runtimeModelAuthenticatorStub{credential: newAgentRuntimeModelCredential(), active: true}

	key, handled := loadSatelliteUserKey(c, keyResolver, "agt_model_runtime-secret", runtime)
	if !handled || key != nil || recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "SATELLITE_USER_REQUIRED") {
		t.Fatalf("missing session identity was not rejected: handled=%v key=%+v status=%d body=%s", handled, key, recorder.Code, recorder.Body.String())
	}
	if runtime.mappingAgentID != "" || len(keyResolver.userIDs) != 0 {
		t.Fatalf("mapping or billing-key lookup ran without an identity: mapping=%q/%d keys=%v", runtime.mappingAgentID, runtime.mappingUserID, keyResolver.userIDs)
	}
}
