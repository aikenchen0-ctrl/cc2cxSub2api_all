package router

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tigerowo/infinite-canvas/config"
	"github.com/tigerowo/infinite-canvas/handler"
	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/repository"
	"github.com/tigerowo/infinite-canvas/service"
)

func TestSub2APIAutoDLVideoLifecycleHTTP(t *testing.T) {
	const marker = "CANVAS_SUB2API_AUTODL_CHILD"
	if os.Getenv(marker) != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSub2APIAutoDLVideoLifecycleHTTP$", "-test.v")
		cmd.Env = append(os.Environ(), marker+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("AutoDL lifecycle: %v\n%s", err, output)
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
	var creates, polls, downloads atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer satellite-autodl-credential" || r.Header.Get("X-Sub2API-On-Behalf-Of") != "autodl-owner" || r.Header.Get("X-Sub2API-Satellite") != "canvas" {
			t.Error("missing satellite identity")
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/videos":
			creates.Add(1)
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				http.Error(w, "invalid JSON", 400)
				return
			}
			for key, expected := range map[string]string{"model": "minimax_h3_z0903", "prompt": "测试视频", "seconds": "7", "resolution_name": "480p横(864*480)"} {
				if body[key] != expected {
					t.Errorf("%s = %v", key, body[key])
				}
			}
			for key, expected := range map[string]string{"input_reference[]": "https://cdn.example/image.png", "audio_reference[]": "https://cdn.example/audio.mp3"} {
				refs, ok := body[key].([]any)
				if !ok || len(refs) != 1 || refs[0] != expected {
					t.Errorf("invalid %s: %v", key, body[key])
				}
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"autodl_task-1","request_id":"autodl_task-1","status":"pending"}`)
		case r.Method == "GET" && r.URL.Path == "/v1/videos/autodl_task-1":
			polls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"autodl_task-1","status":"done","video":{"url":"/v1/videos/autodl_task-1/content"}}`)
		case r.Method == "GET" && r.URL.Path == "/v1/videos/autodl_task-1/content":
			downloads.Add(1)
			w.Header().Set("Content-Type", "video/mp4")
			io.WriteString(w, "autodl-video-fixture")
		default:
			t.Errorf("unexpected gateway path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_RELAY_BASE_URL", upstream.URL)
	t.Setenv("SUB2API_APP_CREDENTIAL", "satellite-autodl-credential")
	t.Setenv("SUB2API_RELAY_VIDEO_MODELS", "grok-imagine-video-1.5")
	if err := service.EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	session, err := service.CompleteSub2APISSO(service.Sub2APISSOPayload{Subject: "autodl-owner", Nonce: "autodl-owner-nonce"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CompleteSub2APISSO(service.Sub2APISSOPayload{Subject: "other-owner", Nonce: "other-owner-nonce"})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.ReleaseMode)
	router := New()
	handler.StartVideoTaskPoller()
	request := httptest.NewRequest("POST", "/api/v1/videos", strings.NewReader(`{"model":"minimax_h3_z0903","prompt":"测试视频","seconds":"7","resolution_name":"480p横(864*480)","input_reference[]":["https://cdn.example/image.png"],"audio_reference[]":["https://cdn.example/audio.mp3"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Model-Channel-ID", "sub2api-relay")
	request.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: session.Token})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var created struct {
		Code int
		Data struct {
			ID        string
			ChannelID string `json:"channelId"`
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || created.Code != 0 || created.Data.ID == "" || created.Data.ChannelID != "sub2api-relay" {
		t.Fatalf("create failed: %s", response.Body.String())
	}
	var task model.VideoTask
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var found bool
		task, found, err = service.GetUserVideoTask(session.User.ID, created.Data.ID)
		if err != nil || !found {
			t.Fatalf("task missing: %v", err)
		}
		if task.Status == "completed" || task.Status == "failed" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if task.Status != "completed" || polls.Load() < 1 || creates.Load() != 1 {
		t.Fatalf("polling failed: status=%s polls=%d", task.Status, polls.Load())
	}
	for _, token := range []string{other.Token, session.Token} {
		request = httptest.NewRequest("GET", "/api/v1/videos/"+created.Data.ID+"/content", nil)
		request.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: token})
		response = httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if token == other.Token {
			if strings.Contains(response.Body.String(), "autodl-video-fixture") || downloads.Load() != 0 {
				t.Fatal("cross-user download was permitted")
			}
		} else if response.Code != 200 || response.Body.String() != "autodl-video-fixture" || response.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("download failed: %d %s", response.Code, response.Body.String())
		}
	}
	if downloads.Load() != 1 {
		t.Fatalf("download calls=%d", downloads.Load())
	}
}
