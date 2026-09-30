package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayHandlerSatelliteUserBalanceReturnsOnlyCurrentUserBalanceFacts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/sub2api/balance", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Key: "never-return-this-credential",
		User: &service.User{
			ID:            7,
			Balance:       0,
			FrozenBalance: 2.5,
		},
	})

	(&GatewayHandler{}).SatelliteUserBalance(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, "sub2api.user_balance", got["object"])
	require.Equal(t, float64(0), got["balance"])
	require.Equal(t, 2.5, got["frozen_balance"])
	require.NotContains(t, got, "email")
	require.NotContains(t, got, "user_id")
	require.NotContains(t, w.Body.String(), "never-return-this-credential")
}

func TestGatewayHandlerSatelliteUserBalanceRequiresAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/sub2api/balance", nil)

	(&GatewayHandler{}).SatelliteUserBalance(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}
