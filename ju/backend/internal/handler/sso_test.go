package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
	"infinite-canvas/backend/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSSOExchangeOrigin(t *testing.T) {
	for _, test := range []struct {
		origin, site, header string
		allow                bool
	}{
		{"http://ju.example", "same-origin", "1", true},
		{"http://ju.example", "", "1", true},
		{"http://evil.example", "cross-site", "1", false},
		{"http://ju.example", "same-site", "1", false},
		{"", "", "1", false},
		{"http://ju.example", "same-origin", "", false},
		{"https://ju.example", "same-origin", "1", false},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "http://ju.example/api/auth/sso/exchange", nil)
		c.Request.Header.Set("Origin", test.origin)
		c.Request.Header.Set("Sec-Fetch-Site", test.site)
		c.Request.Header.Set("X-Ju-SSO", test.header)
		if validSSOExchangeOrigin(c) != test.allow {
			t.Errorf("origin check mismatch: %+v", test)
		}
	}
}

func TestSSOHTTPRoundTripAndOwnership(t *testing.T) {
	t.Setenv("REDIS_URL", "")
	secret := strings.Repeat("test-only-", 4)
	t.Setenv("SUB2API_SSO_SECRET", secret)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sso.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	svc := service.New(repository.New(db), t.TempDir())
	previousRuntime := runtimeService
	ConfigureRuntime(svc)
	t.Cleanup(func() { ConfigureRuntime(previousRuntime); _ = svc.Close() })
	router := gin.New()
	RegisterAuthRoutes(router.Group("/api"), svc)
	request := func(method, path, cookie, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://ju.example"+path, nil)
		r.Header.Set("Cookie", cookie)
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Ju-SSO", "1")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	login := func(subject string) (string, *model.User) {
		t.Helper()
		now := time.Now().Unix()
		body, _ := json.Marshal(map[string]any{"iss": "sub2api", "aud": "ju", "sub": subject, "iat": now, "exp": now + 120, "jti": subject, "next": "/projects"})
		encoded := base64.RawURLEncoding.EncodeToString(body)
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(encoded))
		ticket := encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
		path := "/api/auth/sso/callback?ticket=" + url.QueryEscape(ticket)
		callback := request("GET", path, "", "")
		if callback.Code != 302 || callback.Header().Get("Location") != "/auth/sso" {
			t.Fatalf("callback failed: %d", callback.Code)
		}
		if callback.Header().Get("Cache-Control") != "no-store" || callback.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("callback exposed credentials to cache/referrer")
		}
		handoff := callback.Result().Cookies()[0]
		cookie := handoff.Name + "=" + handoff.Value
		if request("POST", "/api/auth/sso/exchange", cookie, "http://evil.example").Code != 403 {
			t.Fatal("cross-site exchange accepted")
		}
		exchanged := request("POST", "/api/auth/sso/exchange", cookie, "http://ju.example")
		if exchanged.Code != 200 {
			t.Fatalf("exchange failed: %d %s", exchanged.Code, exchanged.Body.String())
		}
		var session string
		var cleared bool
		for _, c := range exchanged.Result().Cookies() {
			if c.Name == "ju_sso_handoff" {
				cleared = c.MaxAge < 0 && c.Value == ""
			}
			if c.Name == service.SessionCookieName {
				session = c.Value
				if !c.HttpOnly {
					t.Fatal("session must be HttpOnly")
				}
			}
		}
		if session == "" || !cleared {
			t.Fatal("session/cookie handoff incomplete")
		}
		if request("POST", "/api/auth/sso/exchange", cookie, "http://ju.example").Code != 401 {
			t.Fatal("exchange replay accepted")
		}
		if request("GET", path, "", "").Header().Get("Location") != "/login?sso_error=1" {
			t.Fatal("callback replay accepted")
		}
		user, err := svc.CurrentUser(session)
		if err != nil {
			t.Fatal(err)
		}
		return session, user
	}
	firstSession, first := login("first")
	_, second := login("second")
	if first.ID == second.ID || first.Role != model.UserRoleUser || second.Role != model.UserRoleUser {
		t.Fatal("identities or roles were merged")
	}
	if err := db.Create(&model.Task{ID: "sso-task", UserID: first.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Task(first.ID, "sso-task"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Task(second.ID, "sso-task"); err == nil {
		t.Fatal("cross-user task access allowed")
	}
	if err := db.Create(&model.Resource{ID: "sso-resource", UserID: first.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resource(first.ID, "sso-resource"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resource(second.ID, "sso-resource"); err == nil {
		t.Fatal("cross-user resource access allowed")
	}
	if err := db.Create(&model.Project{ID: "sso-project", UserID: first.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProjectDetail(first.ID, "sso-project"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProjectDetail(second.ID, "sso-project"); err == nil {
		t.Fatal("cross-user project access allowed")
	}
	if err := svc.Logout(firstSession); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CurrentUser(firstSession); err == nil {
		t.Fatal("local logout left session active")
	}
}

func TestSSOHandoffCookie(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "https://ju.example/api/auth/sso/callback", nil)
	setSSOHandoffCookie(c, "test", int((3 * 24 * time.Hour).Seconds()))
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/api/auth/sso/exchange" || cookie.MaxAge != int((3*24*time.Hour).Seconds()) {
		t.Fatal("unsafe handoff cookie attributes")
	}
}
