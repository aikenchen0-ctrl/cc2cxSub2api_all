package router

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tigerowo/infinite-canvas/config"
	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/repository"
	"github.com/tigerowo/infinite-canvas/service"
)

func TestSub2APISSOAndRelayHTTP(t *testing.T) {
	const marker = "CANVAS_SUB2API_HTTP_CHILD"
	if os.Getenv(marker) != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSub2APISSOAndRelayHTTP$", "-test.v")
		cmd.Env = append(os.Environ(), marker+"=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("HTTP integration: %v\n%s", err, output)
		}
		return
	}
	config.Cfg = config.Config{StorageDriver: "sqlite", DatabaseDSN: t.TempDir() + "/test.db", JWTSecret: strings.Repeat("j", 32), AILogDir: t.TempDir()}
	t.Cleanup(func() {
		if db, err := repository.DB(); err == nil {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	})
	secret := strings.Repeat("s", 32)
	t.Setenv("SUB2API_SSO_SECRET", secret)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		if r.URL.Path == "/v1/images/generations" {
			if r.Header.Get("Authorization") != "Bearer satellite-test-credential" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "http-user" || r.Header.Get("X-Sub2API-Satellite") != "canvas" {
				t.Error("missing image satellite identity")
			}
			var body struct{ Model string }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "gpt-image-2" {
				t.Error("image model was not forwarded unchanged")
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"b64_json":"aW1hZ2UtZml4dHVyZQ=="}]}`)
			return
		}
		if r.URL.Path == "/v1/videos/provider-video/content" {
			if r.Header.Get("Authorization") != "Bearer satellite-test-credential" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "http-user" {
				t.Error("missing video credential")
			}
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video-fixture"))
			return
		}
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer satellite-test-credential" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "http-user" || r.Header.Get("X-Sub2API-Satellite") != "canvas" {
			t.Errorf("unexpected upstream request")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_RELAY_BASE_URL", upstream.URL)
	t.Setenv("SUB2API_APP_CREDENTIAL", "satellite-test-credential")
	t.Setenv("SUB2API_RELAY_MODELS", "gpt-5.5")
	t.Setenv("SUB2API_RELAY_IMAGE_MODELS", "")
	t.Setenv("SUB2API_RELAY_VIDEO_MODELS", "")
	if err := service.EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	gin.DefaultWriter = &logs
	gin.SetMode(gin.ReleaseMode)
	router := New()
	at := time.Now()
	body, _ := json.Marshal(service.Sub2APISSOPayload{Subject: "http-user", Nonce: "http-nonce", IssuedAt: at.Unix(), ExpiresAt: at.Unix() + 60, Next: "/canvas"})
	encoded := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encoded))
	ticket := encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	callback := httptest.NewRecorder()
	router.ServeHTTP(callback, httptest.NewRequest("GET", "/api/auth/sso/callback?ticket="+url.QueryEscape(ticket), nil))
	if callback.Code != 303 || !strings.Contains(callback.Header().Get("Location"), "sso=1") {
		t.Fatalf("callback: %d %s", callback.Code, callback.Body.String())
	}
	if strings.Contains(logs.String(), ticket) || strings.Contains(callback.Header().Get("Location"), "token=") {
		t.Fatal("credential leaked to URL/log")
	}
	cookies := callback.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].MaxAge != 60 {
		t.Fatal("invalid handoff cookie")
	}
	exchange := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/auth/sso/exchange", nil)
	request.Header.Set("X-Canvas-SSO", "1")
	request.AddCookie(cookies[0])
	router.ServeHTTP(exchange, request)
	var result struct {
		Code int
		Data model.AuthSession
	}
	if err := json.Unmarshal(exchange.Body.Bytes(), &result); err != nil || result.Code != 0 || result.Data.Token == "" {
		t.Fatal("session exchange failed")
	}
	if exchange.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("handoff cookie not cleared")
	}
	var sessionCookie *http.Cookie
	for _, cookie := range exchange.Result().Cookies() {
		if cookie.Name == service.SessionCookieName {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || sessionCookie.MaxAge != 259200 || sessionCookie.SameSite != http.SameSiteLaxMode || !strings.HasPrefix(result.Data.Token, "cookie:") || strings.Contains(exchange.Body.String(), sessionCookie.Value) {
		t.Fatal("session must remain in a three-day HttpOnly cookie")
	}
	denied := httptest.NewRecorder()
	crossSite := httptest.NewRequest("POST", "/api/auth/sso/exchange", nil)
	crossSite.AddCookie(cookies[0])
	crossSite.Header.Set("Sec-Fetch-Site", "cross-site")
	router.ServeHTTP(denied, crossSite)
	if denied.Code != 403 {
		t.Fatal("cross-site exchange admitted")
	}
	completion := httptest.NewRecorder()
	request = httptest.NewRequest("POST", "/api/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","stream":true,"messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(sessionCookie)
	request.Header.Set("X-Model-Channel-ID", "sub2api-relay")
	router.ServeHTTP(completion, request)
	if completion.Code != 200 || !strings.Contains(completion.Body.String(), "[DONE]") || upstreamCalls.Load() != 1 {
		t.Fatalf("Relay stream failed: %s", completion.Body.String())
	}
	image := httptest.NewRecorder()
	request = httptest.NewRequest("POST", "/api/v1/images/generations", strings.NewReader(`{"model":"gpt-image-2","prompt":"a box of candy"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(sessionCookie)
	router.ServeHTTP(image, request)
	if image.Code != 200 || !strings.Contains(image.Body.String(), "aW1hZ2UtZml4dHVyZQ==") || upstreamCalls.Load() != 2 {
		t.Fatalf("image relay failed: %d %s", image.Code, image.Body.String())
	}
	imageTask := httptest.NewRecorder()
	request = httptest.NewRequest("POST", "/api/v1/canvas/image-tasks", strings.NewReader(`{"channelId":"sub2api-relay","request":{"model":"gpt-image-2","prompt":"a box of candy"}}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(sessionCookie)
	router.ServeHTTP(imageTask, request)
	var created struct {
		Code int
		Data struct{ ID string }
	}
	if err := json.Unmarshal(imageTask.Body.Bytes(), &created); err != nil || created.Code != 0 || created.Data.ID == "" {
		t.Fatalf("image task create failed: %s", imageTask.Body.String())
	}
	var generated model.CanvasImageTask
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		var found bool
		var err error
		generated, found, err = service.GetUserCanvasImageTask(result.Data.User.ID, created.Data.ID)
		if err != nil || !found {
			t.Fatal("image task missing")
		}
		if generated.Status == "completed" || generated.Status == "failed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if generated.Status != "completed" || generated.ImageURL == "" || generated.Error != "" || upstreamCalls.Load() != 3 {
		t.Fatalf("async image task failed: status=%s error=%s", generated.Status, generated.Error)
	}
	task, err := service.CreateVideoTask(service.VideoTaskCreateInput{UserID: result.Data.User.ID, Model: "gpt-5.5", ChannelID: "sub2api-relay", ClientTaskID: "local-video", UpstreamTaskID: "provider-video", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	download := httptest.NewRecorder()
	request = httptest.NewRequest("GET", "/api/v1/videos/"+task.ID+"/content?model=gpt-5.5", nil)
	request.AddCookie(sessionCookie)
	router.ServeHTTP(download, request)
	if download.Code != 200 || download.Body.String() != "video-fixture" {
		t.Fatalf("video download failed: %d %s", download.Code, download.Body.String())
	}
	callsBefore := upstreamCalls.Load()
	_, err = service.CreateVideoTask(service.VideoTaskCreateInput{UserID: "another-user", Model: "gpt-5.5", ChannelID: "sub2api-relay", ClientTaskID: "foreign-local", UpstreamTaskID: "foreign-provider", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	foreign := httptest.NewRecorder()
	request = httptest.NewRequest("GET", "/api/v1/videos/foreign-provider/content?model=gpt-5.5", nil)
	request.AddCookie(sessionCookie)
	router.ServeHTTP(foreign, request)
	if foreign.Code != 404 || upstreamCalls.Load() != callsBefore {
		t.Fatal("unowned upstream video ID accepted")
	}
	replay := httptest.NewRecorder()
	router.ServeHTTP(replay, httptest.NewRequest("GET", "/api/auth/sso/callback?ticket="+url.QueryEscape(ticket), nil))
	if !strings.Contains(replay.Header().Get("Location"), "SSO_failed") {
		t.Fatal("replay admitted")
	}
}
