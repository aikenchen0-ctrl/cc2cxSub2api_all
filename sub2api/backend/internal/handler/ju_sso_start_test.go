//go:build unit

package handler

import (
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

func TestJuSSOStartRejectsUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUB2API_SSO_SECRET", strings.Repeat("s", 32))
	t.Setenv("JU_SSO_CALLBACK_URL", "http://localhost:3000/api/auth/sso/callback")

	handler := &AuthHandler{userService: service.NewUserService(&userHandlerRepoStub{
		user: &service.User{ID: 7, Email: "user@example.com", Username: "ju-user"},
	}, nil, nil, nil)}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/integrations/ju/start", nil)

	handler.JuSSOStart(c)

	require.NotEqual(t, http.StatusOK, recorder.Code)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "redirect_url")
	require.NotContains(t, recorder.Body.String(), "ticket=")
}

func TestJuSSOStartIssuesCallbackTicketWithoutSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := strings.Repeat("ju-sso-secret-value-", 2)
	require.GreaterOrEqual(t, len(secret), 32)
	t.Setenv("SUB2API_SSO_SECRET", secret)
	t.Setenv("JU_SSO_CALLBACK_URL", "http://localhost:3000/api/auth/sso/callback")

	handler := &AuthHandler{userService: service.NewUserService(&userHandlerRepoStub{
		user: &service.User{ID: 7, Email: "user@example.com", Username: "ju-user", AvatarURL: "https://cdn.example/a.png"},
	}, nil, nil, nil)}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/integrations/ju/start?next=%2Fprojects", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7})

	handler.JuSSOStart(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			RedirectURL string `json:"redirect_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)

	parsed, err := url.Parse(resp.Data.RedirectURL)
	require.NoError(t, err)
	require.Equal(t, "http", parsed.Scheme)
	require.Equal(t, "localhost:3000", parsed.Host)
	require.Equal(t, "/api/auth/sso/callback", parsed.Path)
	ticket := parsed.Query().Get("ticket")
	require.NotEmpty(t, ticket)
	require.Empty(t, parsed.Query().Get("apiKey"))
	require.Empty(t, parsed.Query().Get("api_key"))
	require.Empty(t, parsed.Query().Get("token"))
	require.Empty(t, parsed.Query().Get("jwt"))
	require.Empty(t, parsed.Query().Get("superKey"))
	require.Empty(t, parsed.Query().Get("super_key"))
	require.NotContains(t, strings.ToLower(resp.Data.RedirectURL), "sk-")
	require.NotContains(t, strings.ToLower(resp.Data.RedirectURL), "bearer")
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "sk-")
	require.NotContains(t, recorder.Body.String(), secret)

	payload := decodeLivartSSOTicket(t, ticket, secret)
	require.Equal(t, "7", payload.Subject)
	require.Equal(t, "user@example.com", payload.Email)
	require.Equal(t, "ju-user", payload.Username)
	require.Equal(t, "/projects", payload.Next)
	require.NotEmpty(t, payload.Nonce)
}
