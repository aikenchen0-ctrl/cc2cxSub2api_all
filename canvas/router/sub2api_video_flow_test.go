package router

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
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
	"github.com/tigerowo/infinite-canvas/service"
)

func TestSub2APIVideoLifecycleHTTP(t *testing.T) {
	// Repository initialization and the background poller are process-global.
	const marker = "CANVAS_SUB2API_VIDEO_CHILD"
	if os.Getenv(marker) != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSub2APIVideoLifecycleHTTP$", "-test.v")
		cmd.Env = append(os.Environ(), marker+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("video lifecycle: %v\n%s", err, output)
		}
		return
	}
	config.Cfg = config.Config{StorageDriver: "sqlite", DatabaseDSN: t.TempDir() + "/test.db", JWTSecret: strings.Repeat("j", 32), AILogDir: t.TempDir()}
	var polls, downloads atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-video-fixture" {
			t.Error("missing upstream credential")
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/videos":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				http.Error(w, "invalid multipart", 400)
				return
			}
			defer r.MultipartForm.RemoveAll()
			for key, expected := range map[string]string{"model": "sora-2", "prompt": "Test reference video", "seconds": "8", "size": "1280x720", "resolution_name": "720p"} {
				if got := r.FormValue(key); got != expected {
					t.Errorf("%s = %q, want %q", key, got, expected)
				}
			}
			file, _, err := r.FormFile("input_reference[]")
			if err != nil {
				t.Error(err)
				http.Error(w, "missing reference", 400)
				return
			}
			data, err := io.ReadAll(file)
			file.Close()
			if err != nil || string(data) != "reference-image-fixture" {
				t.Error("reference image changed")
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"provider-video","status":"queued","model":"sora-2","seconds":"8","size":"1280x720"}`)
		case r.Method == "GET" && r.URL.Path == "/v1/videos/provider-video":
			polls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"provider-video","status":"completed","progress":100,"seconds":"8","size":"1280x720"}`)
		case r.Method == "GET" && r.URL.Path == "/v1/videos/provider-video/content":
			downloads.Add(1)
			w.Header().Set("Content-Type", "video/mp4")
			io.WriteString(w, "video-content-fixture")
		default:
			t.Errorf("unexpected upstream route: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_RELAY_BASE_URL", upstream.URL)
	t.Setenv("SUB2API_RELAY_API_KEY", "sk-video-fixture")
	t.Setenv("SUB2API_RELAY_MODELS", "")
	t.Setenv("SUB2API_RELAY_IMAGE_MODELS", "")
	t.Setenv("SUB2API_RELAY_VIDEO_MODELS", "sora-2")
	if err := service.EnsureSub2APIRelayChannel(); err != nil {
		t.Fatal(err)
	}
	session, err := service.CompleteSub2APISSO(service.Sub2APISSOPayload{Subject: "video-owner", Nonce: "video-owner-nonce"})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.ReleaseMode)
	router := New()
	handler.StartVideoTaskPoller()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for key, value := range map[string]string{"model": "sora-2", "prompt": "Test reference video", "seconds": "8", "size": "1280x720", "resolution_name": "720p"} {
		if err := form.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	file, err := form.CreateFormFile("input_reference[]", "reference.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(file, "reference-image-fixture"); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/api/v1/videos", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+session.Token)
	request.Header.Set("X-Model-Channel-ID", "sub2api-relay")
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
			t.Fatalf("task lookup: found=%v error=%v", found, err)
		}
		if task.Status == "completed" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if task.Status != "completed" || polls.Load() == 0 {
		t.Fatalf("polling failed: status=%s polls=%d", task.Status, polls.Load())
	}
	for _, suffix := range []string{"", "/content"} {
		request = httptest.NewRequest("GET", "/api/v1/videos/"+created.Data.ID+suffix, nil)
		request.Header.Set("Authorization", "Bearer "+session.Token)
		response = httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if suffix == "" {
			var polled struct {
				Code int
				Data struct{ Status string }
			}
			if err := json.Unmarshal(response.Body.Bytes(), &polled); err != nil {
				t.Fatal(err)
			}
			if polled.Code != 0 || polled.Data.Status != "completed" {
				t.Fatalf("poll route: %s", response.Body.String())
			}
		} else if response.Code != 200 || response.Body.String() != "video-content-fixture" || response.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("download failed: %d %s", response.Code, response.Body.String())
		}
	}
	if downloads.Load() != 1 {
		t.Fatalf("download calls = %d", downloads.Load())
	}
}
