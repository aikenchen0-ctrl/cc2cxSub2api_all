package handler

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSatelliteOAuthBindingCallbackURLUsesRegisteredAgentOrigin(t *testing.T) {
	t.Setenv("AGENTAPI_LINK", "https://agent.example/base?ignored=1#ignored")

	callback, err := satelliteOAuthBindingCallbackURL("agentapi")
	require.NoError(t, err)
	require.Equal(t, "https://agent.example/auth/oauth/binding/callback", callback)
}

func TestSatelliteOAuthBindingClaimsSetsShortLivedHttpOnlyCookie(t *testing.T) {
	const signingKey = "0123456789abcdef0123456789abcdef"
	t.Setenv("PAYMENT_RESUME_SIGNING_KEY", signingKey)
	handler := &AuthHandler{}
	token, err := service.NewPaymentResumeService([]byte(signingKey)).CreateOAuthBindingHandoffToken(service.OAuthBindingHandoffClaims{
		SatelliteSlug: "agentapi",
		UserID:        42,
		Provider:      "linuxdo",
		RedirectTo:    "/profile",
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/bind/start?satellite_handoff="+url.QueryEscape(token), nil)
	claims, err := handler.satelliteOAuthBindingClaims(c, "linuxdo")
	require.NoError(t, err)
	require.Equal(t, int64(42), claims.UserID)

	cookie := findCookie(recorder.Result().Cookies(), satelliteOAuthBindingHandoffCookie)
	require.NotNil(t, cookie)
	require.True(t, cookie.HttpOnly)
	require.Equal(t, int(service.OAuthBindingHandoffTTL().Seconds()), cookie.MaxAge)
	require.NotEqual(t, token, cookie.Value)
}

func TestSatelliteOAuthBindingCookieRejectsProviderMismatch(t *testing.T) {
	const signingKey = "0123456789abcdef0123456789abcdef"
	t.Setenv("PAYMENT_RESUME_SIGNING_KEY", signingKey)
	handler := &AuthHandler{}
	token, err := service.NewPaymentResumeService([]byte(signingKey)).CreateOAuthBindingHandoffToken(service.OAuthBindingHandoffClaims{
		SatelliteSlug: "agentapi",
		UserID:        42,
		Provider:      "linuxdo",
		RedirectTo:    "/profile",
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/callback", nil)
	c.Request.AddCookie(encodedCookie(satelliteOAuthBindingHandoffCookie, token))

	claims, err := handler.satelliteOAuthBindingClaimsFromCookie(c, "wechat")
	require.Error(t, err)
	require.Nil(t, claims)
}
