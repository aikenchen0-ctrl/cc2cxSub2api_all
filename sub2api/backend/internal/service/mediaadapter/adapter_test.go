package mediaadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogContainsAllConfiguredModels(t *testing.T) {
	for _, model := range []string{"gpt-image-1", "gpt-image-1.5", "gpt-image-2", "gemini-3.1-flash-image", "grok-imagine-image-quality", "grok-imagine-image-2.0", "grok-imagine-image", "grok-imagine", "grok-imagine-image-1.5", "grok-imagine-video-1.5", "kling-v1", "kling-v1-5", "kling-v1-6", "kling-v2-5-turbo", "kling-v2-6", "kling-v3", "kling-v3-omni", "seedance-2.0", "seedance-2.0-fast", "seedance2.0-900-720p", "sd-2.5-30秒", "minimax-h3-933-图文", "grok-video-1.5", "minimax-h3", "seedance2.5-9图"} {
		capability, ok := Lookup(model)
		if !ok || capability.Model != model {
			t.Fatalf("model %q missing", model)
		}
	}
}

func TestVividModelsUseOpenAICompatibleProfile(t *testing.T) {
	for _, model := range []string{"seedance2.0-900-720p", "sd-2.5-30秒", "minimax-h3-933-图文", "grok-video-1.5", "minimax-h3", "seedance2.5-9图"} {
		capability, ok := Lookup(model)
		if !ok || capability.Modality != "video" || capability.Profile != ProfileOpenAICompatible {
			t.Fatalf("%s capability = %#v, %v", model, capability, ok)
		}
	}
}

func TestCatalogUsesExplicitProviderProfile(t *testing.T) {
	for _, model := range []string{"grok-imagine-image-1.5", "grok-imagine-video-1.5", "kling-v3", "seedance-2.0"} {
		capability, ok := Lookup(model)
		if !ok || capability.Profile != ProfileOpenAICompatible {
			t.Fatalf("%s profile = %q, want %q", model, capability.Profile, ProfileOpenAICompatible)
		}
	}
	capability, ok := Lookup("grok-imagine-image")
	if !ok || capability.Profile != ProfileGrokMedia {
		t.Fatalf("QA Grok profile = %q, want %q", capability.Profile, ProfileGrokMedia)
	}
}

func TestCompatibleAdapterGeneratesImageAndVideo(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/images/generations":
			_ = json.NewEncoder(w).Encode(map[string]any{"created": 1, "data": []any{map[string]string{"url": "https://cdn/image.png"}}})
		case "/v1/videos":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "task-1", "status": "queued"})
		case "/v1/videos/task-1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "task-1", "status": "completed", "video_url": "https://cdn/video.mp4"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter := New(Config{BaseURL: server.URL, APIKey: "test-key"})
	image, err := adapter.GenerateImage(context.Background(), ImageRequest{Model: "kling-v3", Prompt: "a test"})
	if err != nil || len(image.Data) != 1 || image.Data[0].URL == "" {
		t.Fatalf("image = %#v, err = %v", image, err)
	}
	created, err := adapter.CreateVideo(context.Background(), VideoRequest{Model: "seedance-2.0", Prompt: "a test"})
	if err != nil || created.ID != "task-1" {
		t.Fatalf("created = %#v, err = %v", created, err)
	}
	status, err := adapter.GetVideo(context.Background(), "task-1")
	if err != nil || status.Status != VideoCompleted || status.VideoURL == "" {
		t.Fatalf("status = %#v, err = %v", status, err)
	}
	if len(paths) != 3 || paths[0] != "/v1/images/generations" || paths[1] != "/v1/videos" || paths[2] != "/v1/videos/task-1" {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestCompatibleAdapterPropagatesUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid model"}}`))
	}))
	defer server.Close()
	_, err := New(Config{BaseURL: server.URL}).GenerateImage(context.Background(), ImageRequest{Model: "gpt-image-2", Prompt: "x"})
	if err == nil || err.Error() != "upstream 400: invalid model" {
		t.Fatalf("err = %v", err)
	}
}

func TestCompatibleAdapterUnwrapsDataEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/images/generations" {
			_, _ = w.Write([]byte(`{"data":{"created":2,"data":[{"b64_json":"aGVsbG8="}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"id":"wrapped-1","status":"completed"}}`))
	}))
	defer server.Close()
	adapter := New(Config{BaseURL: server.URL})
	image, err := adapter.GenerateImage(context.Background(), ImageRequest{Model: "gpt-image-2", Prompt: "x"})
	if err != nil || image.Created != 2 || image.Data[0].B64JSON == "" {
		t.Fatalf("image = %#v, err = %v", image, err)
	}
	video, err := adapter.GetVideo(context.Background(), "wrapped-1")
	if err != nil || video.ID != "wrapped-1" || video.Status != VideoCompleted {
		t.Fatalf("video = %#v, err = %v", video, err)
	}
}

func TestEditImageUsesMultipartReferenceImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/images/edits" || !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
			t.Fatalf("unexpected request: %s %s %s", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("model") != "gpt-image-2" || r.FormValue("prompt") != "edit" {
			t.Fatalf("fields = %#v", r.MultipartForm.Value)
		}
		files := r.MultipartForm.File["image"]
		if len(files) != 1 {
			t.Fatalf("files = %d", len(files))
		}
		file, err := files[0].Open()
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if !bytes.Equal(data, []byte("hello")) {
			t.Fatalf("file = %q", data)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn/edited.png"}]}`))
	}))
	defer server.Close()
	image, err := New(Config{BaseURL: server.URL}).EditImage(context.Background(), ImageRequest{Model: "gpt-image-2", Prompt: "edit", Images: []string{"data:text/plain;base64,aGVsbG8="}})
	if err != nil || len(image.Data) != 1 {
		t.Fatalf("image = %#v, err = %v", image, err)
	}
}

func TestVideoStatusNormalizesCompatibleProviderStates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"task","status":"done","video":{"url":"https://cdn/video.mp4"}}`))
	}))
	defer server.Close()
	video, err := New(Config{BaseURL: server.URL}).GetVideo(context.Background(), "task")
	if err != nil || video.Status != VideoCompleted || video.VideoURL != "https://cdn/video.mp4" {
		t.Fatalf("video = %#v, err = %v", video, err)
	}
}
