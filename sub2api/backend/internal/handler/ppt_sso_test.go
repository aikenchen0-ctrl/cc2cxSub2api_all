//go:build unit

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPPTSSOStart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := strings.Repeat("p", 32)
	handler := &AuthHandler{userService: service.NewUserService(&userHandlerRepoStub{
		user: &service.User{ID: 7, Username: "ppt-user"},
	}, nil, nil, nil)}
	for _, tc := range []struct {
		name, secret, callback, next string
		authenticated                bool
		status                       int
	}{
		{"valid", secret, "http://localhost:8341/api/v1/auth/sso/callback", "/upload", true, 200},
		{"default callback", secret, "", "", true, 200},
		{"external next", secret, "https://ppt.example/api/v1/auth/sso/callback", "//evil.example", true, 200},
		{"control next", secret, "", "/a\nb", true, 200},
		{"long next", secret, "", "/" + strings.Repeat("a", 2049), true, 200},
		{"anonymous", secret, "", "", false, 401},
		{"missing secret", "", "", "", true, 503},
		{"short secret", "short", "", "", true, 503},
		{"bad scheme", secret, "javascript://ppt.example", "", true, 503},
		{"query", secret, "https://ppt.example/callback?key=secret", "", true, 503},
		{"fragment", secret, "https://ppt.example/callback#secret", "", true, 503},
		{"credentials", secret, "https://user:secret@ppt.example/callback", "", true, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SUB2API_SSO_SECRET", tc.secret)
			t.Setenv("PPT_SSO_CALLBACK_URL", tc.callback)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/integrations/ppt/start?next="+url.QueryEscape(tc.next), nil)
			if tc.authenticated {
				c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7})
			}
			handler.PPTSSOStart(c)
			require.Equal(t, tc.status, recorder.Code)
			require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			if tc.status != 200 {
				require.NotContains(t, recorder.Body.String(), "ticket=")
				return
			}
			var result struct {
				Data struct {
					RedirectURL string `json:"redirect_url"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
			parsed, err := url.Parse(result.Data.RedirectURL)
			require.NoError(t, err)
			require.Len(t, parsed.Query(), 1)
			payload := decodeCanvasSSOTicket(t, parsed.Query().Get("ticket"), secret)
			require.Equal(t, "presenton", payload.Audience)
			require.Equal(t, "7", payload.Subject)
			require.Equal(t, "/upload", payload.Next)
			require.EqualValues(t, 120, payload.ExpiresAt-payload.IssuedAt)
			require.Greater(t, payload.ExpiresAt, time.Now().Unix())
			require.NotEmpty(t, payload.Nonce)
			require.NotContains(t, recorder.Body.String(), secret)
			require.Empty(t, payload.Email)
		})
	}
}
