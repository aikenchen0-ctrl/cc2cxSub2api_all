package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func TestAuthenticateSatellitePasswordRecoveryUsesRegisteredOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUB2API_APP_CREDENTIAL", "satellite-secret")
	t.Setenv("AGENTAPI_LINK", "https://tenant.example.test/ignored/path")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/satellite/agentapi/forgot-password", nil)
	ctx.Params = gin.Params{{Key: "slug", Value: "agentapi"}}
	ctx.Request.Header.Set("Authorization", "Bearer satellite-secret")
	ctx.Request.Header.Set(servermiddleware.HeaderSatelliteApp, "agentapi")

	app, origin, ok := authenticateSatellitePasswordRecovery(ctx)
	if !ok || app.Slug != "agentapi" || origin != "https://tenant.example.test" {
		t.Fatalf("unexpected satellite recovery identity: ok=%v app=%+v origin=%q body=%s", ok, app, origin, recorder.Body.String())
	}
}

func TestAuthenticateSatellitePasswordRecoveryRejectsMismatchedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUB2API_APP_CREDENTIAL", "satellite-secret")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/satellite/agentapi/forgot-password", nil)
	ctx.Params = gin.Params{{Key: "slug", Value: "agentapi"}}
	ctx.Request.Header.Set("Authorization", "Bearer satellite-secret")
	ctx.Request.Header.Set(servermiddleware.HeaderSatelliteApp, "canvas")

	if _, _, ok := authenticateSatellitePasswordRecovery(ctx); ok || recorder.Code != http.StatusUnauthorized {
		t.Fatalf("mismatched satellite identity was accepted: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAuthenticateSatelliteRegistrationVerificationUsesSameApplicationIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUB2API_APP_CREDENTIAL", "satellite-secret")

	for _, path := range []string{
		"/api/v1/auth/satellite/agentapi/send-verify-code",
		"/api/v1/auth/satellite/agentapi/verify-registration-email",
	} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, path, nil)
		ctx.Params = gin.Params{{Key: "slug", Value: "agentapi"}}
		ctx.Request.Header.Set("Authorization", "Bearer satellite-secret")
		ctx.Request.Header.Set(servermiddleware.HeaderSatelliteApp, "agentapi")

		app, ok := authenticateSatelliteApplication(ctx)
		if !ok || app.Slug != "agentapi" {
			t.Fatalf("satellite registration verification identity rejected for %s: status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestAuthenticateSatelliteRegistrationVerificationRejectsBrowserOrAdminCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUB2API_APP_CREDENTIAL", "satellite-secret")

	for _, authorization := range []string{"", "Bearer main-admin-key", "Basic satellite-secret"} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/satellite/agentapi/send-verify-code", nil)
		ctx.Params = gin.Params{{Key: "slug", Value: "agentapi"}}
		ctx.Request.Header.Set("Authorization", authorization)
		ctx.Request.Header.Set(servermiddleware.HeaderSatelliteApp, "agentapi")

		if _, ok := authenticateSatelliteApplication(ctx); ok || recorder.Code != http.StatusUnauthorized {
			t.Fatalf("non-application credential %q was accepted: status=%d body=%s", authorization, recorder.Code, recorder.Body.String())
		}
	}
}
