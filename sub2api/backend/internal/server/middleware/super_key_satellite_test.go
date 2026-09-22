package middleware

import (
	"net/http"
	"net/http/httptest"
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

func TestRejectSuperKeyAsAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	require.True(t, rejectSuperKeyAsAPIKey(c, "sk-super-leaked"))
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "SUPER_KEY_NOT_AN_API_KEY")
}
