//go:build unit

package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

func TestYibiaoSSOStartRejectsUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUB2API_SSO_SECRET", strings.Repeat("s", 32))
	t.Setenv("YIBIAO_SSO_CALLBACK_URL", "http://localhost:5173/api/auth/sso/callback")

	handler := &AuthHandler{userService: service.NewUserService(&userHandlerRepoStub{
		user: &service.User{ID: 7, Email: "user@example.com", Username: "yibiao-user", Status: "active"},
	}, nil, nil, nil)}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/integrations/yibiao/start", nil)

	handler.YibiaoSSOStart(c)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "redirect_url")
}

func TestYibiaoSSOStartIssuesAudienceScopedTicket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := strings.Repeat("yibiao-sso-secret-", 2)
	t.Setenv("SUB2API_SSO_SECRET", secret)
	t.Setenv("YIBIAO_SSO_CALLBACK_URL", "http://localhost:5173/api/auth/sso/callback")

	handler := &AuthHandler{userService: service.NewUserService(&userHandlerRepoStub{
		user: &service.User{ID: 7, Email: "user@example.com", Username: "yibiao-user", AvatarURL: "https://cdn.example/user.png", Status: "active"},
	}, nil, nil, nil)}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/integrations/yibiao/start?next=%2F", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7})

	handler.YibiaoSSOStart(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Data struct {
			RedirectURL string `json:"redirect_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	parsed, err := url.Parse(resp.Data.RedirectURL)
	require.NoError(t, err)
	require.Equal(t, "localhost:5173", parsed.Host)
	require.Equal(t, "/api/auth/sso/callback", parsed.Path)
	ticket := parsed.Query().Get("ticket")
	require.NotEmpty(t, ticket)
	require.NotContains(t, strings.ToLower(resp.Data.RedirectURL), "bearer")
	require.NotContains(t, resp.Data.RedirectURL, secret)

	payload := decodeYibiaoSSOTicket(t, ticket, secret)
	require.Equal(t, "sub2api", payload.Issuer)
	require.Equal(t, "yibiao-bidmonitor", payload.Audience)
	require.Equal(t, "7", payload.Subject)
	require.Equal(t, "user@example.com", payload.Email)
	require.Equal(t, "yibiao-user", payload.Username)
	require.Equal(t, "/", payload.Next)
	require.NotEmpty(t, payload.Nonce)
}

func TestYibiaoSSOStartRejectsExternalNextAndInvalidCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := strings.Repeat("s", 32)
	t.Setenv("SUB2API_SSO_SECRET", secret)
	t.Setenv("YIBIAO_SSO_CALLBACK_URL", "https://bidmonitor.example/api/auth/sso/callback")

	handler := &AuthHandler{userService: service.NewUserService(&userHandlerRepoStub{
		user: &service.User{ID: 9, Email: "next@example.com", Username: "next-user", Status: "active"},
	}, nil, nil, nil)}
	for _, next := range []string{"https://evil.example", "//evil.example", `/\\evil.example`, "javascript:alert(1)"} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/integrations/yibiao/start?next="+url.QueryEscape(next), nil)
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 9})

		handler.YibiaoSSOStart(c)

		require.Equal(t, http.StatusOK, recorder.Code, "next=%s", next)
		var resp struct {
			Data struct {
				RedirectURL string `json:"redirect_url"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
		parsed, err := url.Parse(resp.Data.RedirectURL)
		require.NoError(t, err)
		payload := decodeYibiaoSSOTicket(t, parsed.Query().Get("ticket"), secret)
		require.Equal(t, "/", payload.Next)
	}

	t.Setenv("YIBIAO_SSO_CALLBACK_URL", "https://bidmonitor.example/wrong-path")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/integrations/yibiao/start", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 9})
	handler.YibiaoSSOStart(c)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}

func decodeYibiaoSSOTicket(t *testing.T, raw, secret string) juSSOTicket {
	t.Helper()
	parts := strings.Split(raw, ".")
	require.Len(t, parts, 2)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	require.True(t, hmac.Equal(signature, mac.Sum(nil)))
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	var payload juSSOTicket
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload
}
