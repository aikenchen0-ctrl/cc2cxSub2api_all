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

func TestCanvasSSOStartRejectsUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUB2API_SSO_SECRET", strings.Repeat("s", 32))
	t.Setenv("CANVAS_SSO_CALLBACK_URL", "http://localhost:3522/api/auth/sso/callback")

	handler := &AuthHandler{userService: service.NewUserService(&userHandlerRepoStub{
		user: &service.User{ID: 7, Status: service.StatusActive, Email: "user@example.com", Username: "canvas-user"},
	}, nil, nil, nil)}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/integrations/canvas/start", nil)

	handler.CanvasSSOStart(c)

	require.NotEqual(t, http.StatusOK, recorder.Code)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "redirect_url")
	require.NotContains(t, recorder.Body.String(), "ticket=")
}

func TestCanvasSSOStartIssuesCallbackTicketWithoutSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withTestSuperKey(t)
	secret := strings.Repeat("canvas-sso-secret-", 2)
	require.GreaterOrEqual(t, len(secret), 32)
	t.Setenv("SUB2API_SSO_SECRET", secret)
	t.Setenv("CANVAS_SSO_CALLBACK_URL", "http://localhost:3522/api/auth/sso/callback")

	handler := &AuthHandler{userService: service.NewUserService(&userHandlerRepoStub{
		user: &service.User{ID: 7, Status: service.StatusActive, Email: "user@example.com", Username: "canvas-user", AvatarURL: "https://cdn.example/a.png"},
	}, nil, nil, nil)}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/integrations/canvas/start?next=%2F", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7})

	handler.CanvasSSOStart(c)

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
	require.Equal(t, "localhost:3522", parsed.Host)
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
	require.NotContains(t, strings.ToLower(resp.Data.RedirectURL), "jwt")
	require.NotContains(t, strings.ToLower(recorder.Body.String()), "sk-")
	require.NotContains(t, recorder.Body.String(), secret)

	payload := decodeCanvasSSOTicket(t, ticket, secret)
	require.Equal(t, "7", payload.Subject)
	require.Equal(t, "user@example.com", payload.Email)
	require.Equal(t, "canvas-user", payload.Username)
	require.Equal(t, "/", payload.Next)
	require.NotEmpty(t, payload.Nonce)
}

func TestCanvasSSOStartRejectsExternalNext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withTestSuperKey(t)
	secret := strings.Repeat("s", 32)
	t.Setenv("SUB2API_SSO_SECRET", secret)
	t.Setenv("CANVAS_SSO_CALLBACK_URL", "https://canvas.example/api/auth/sso/callback")

	handler := &AuthHandler{userService: service.NewUserService(&userHandlerRepoStub{
		user: &service.User{ID: 9, Status: service.StatusActive, Email: "next@example.com", Username: "next-user"},
	}, nil, nil, nil)}

	for _, next := range []string{
		"https://evil.example",
		"//evil.example",
		`/\evil.example`,
		"javascript:alert(1)",
	} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/integrations/canvas/start?next="+url.QueryEscape(next), nil)
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 9})

		handler.CanvasSSOStart(c)

		require.Equal(t, http.StatusOK, recorder.Code, "next=%s", next)
		var resp struct {
			Data struct {
				RedirectURL string `json:"redirect_url"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
		parsed, err := url.Parse(resp.Data.RedirectURL)
		require.NoError(t, err)
		require.Equal(t, "canvas.example", parsed.Host)
		payload := decodeCanvasSSOTicket(t, parsed.Query().Get("ticket"), secret)
		require.Equal(t, "/", payload.Next, "next=%s", next)
		require.NotContains(t, payload.Next, "evil")
	}
}

func decodeCanvasSSOTicket(t *testing.T, raw, secret string) juSSOTicket {
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
