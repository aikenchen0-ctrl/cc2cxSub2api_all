package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service/mediaadapter"
)

func TestNormalizeAutoDLBaseURL(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"https://www.autodl.art", "https://www.autodl.art"},
		{"https://www.autodl.art/", "https://www.autodl.art"},
		{"https://www.autodl.art/v1/", "https://www.autodl.art"},
		{"https://www.autodl.art/api/v1/", "https://www.autodl.art"},
		{"https://www.autodl.art/api/", "https://www.autodl.art"},
	} {
		if got := normalizeAutoDLBaseURL(tc.input); got != tc.want {
			t.Errorf("normalizeAutoDLBaseURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestAutoDLWorkflowForModelAcceptsCanvasAliases(t *testing.T) {
	for _, model := range []string{"MiniMax-H3", "minimax_h3", "minimax"} {
		if got, ok := mediaadapter.AutoDLWorkflowForModel(model); !ok || got != "minimax_h3_b99_001" {
			t.Fatalf("AutoDLWorkflowForModel(%q) = %q, %v; want minimax_h3_b99_001, true", model, got, ok)
		}
	}
}

func TestAutoDLVideoModelSupportedRejectsUnknownModel(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://www.autodl.art"}}
	if AutoDLVideoModelSupported(account, "video-v3") {
		t.Fatal("unknown video model must not be sent to AutoDL")
	}
	if !AutoDLVideoModelSupported(account, "MiniMax-H3") {
		t.Fatal("Canvas MiniMax-H3 alias should be supported by AutoDL")
	}
}

func TestIsAutoDLBaseURLUsesExactHost(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want bool
	}{
		{"https://autodl.art", true},
		{"https://www.autodl.art/api/v1", true},
		{"https://cdn.autodl.art", true},
		{"https://autodl.art.example.com", false},
		{"not-a-url-autodl.art", false},
	} {
		if got := IsAutoDLBaseURL(tc.url); got != tc.want {
			t.Errorf("IsAutoDLBaseURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestValidateAutoDLVideoResultURL(t *testing.T) {
	base := "https://www.autodl.art/api/v1"
	for _, raw := range []string{
		"https://autodl.art/video.mp4",
		"https://cdn.autodl.art/video.mp4",
		"https://www.autodl.art/video.mp4",
	} {
		if err := validateAutoDLVideoResultURL(raw, base); err != nil {
			t.Errorf("validateAutoDLVideoResultURL(%q): %v", raw, err)
		}
	}
	for _, raw := range []string{
		"http://autodl.art/video.mp4",
		"https://169.254.169.254/latest/meta-data",
		"https://autodl.art.example.com/video.mp4",
		"https://user:pass@autodl.art/video.mp4",
	} {
		if err := validateAutoDLVideoResultURL(raw, base); err == nil {
			t.Errorf("validateAutoDLVideoResultURL(%q) unexpectedly accepted", raw)
		}
	}
}

func TestShouldPreferAutoDLVideoModel(t *testing.T) {
	for _, model := range []string{"MiniMax-H3", "minimax", "minimax_h3_b99_001"} {
		if !ShouldPreferAutoDLVideoModel(model) {
			t.Fatalf("%q should prefer AutoDL", model)
		}
	}
	for _, model := range []string{"veo-3-1", "seedance-2.0", "video-v3"} {
		if ShouldPreferAutoDLVideoModel(model) {
			t.Fatalf("%q should not prefer AutoDL", model)
		}
	}
}

func TestOpenAICompatibleVideoModelSupportedAllowsCatalogVideoModels(t *testing.T) {
	noMapping := &Account{Platform: PlatformOpenAI, Credentials: map[string]any{"base_url": "https://provider.example"}}
	if !OpenAICompatibleVideoModelSupported(noMapping, "video-v3") {
		t.Fatal("catalog video model should be accepted without an explicit mapping")
	}
	unknown := &Account{Platform: PlatformOpenAI, Credentials: map[string]any{"base_url": "https://provider.example"}}
	if OpenAICompatibleVideoModelSupported(unknown, "unknown-video") {
		t.Fatal("unknown video model without an explicit mapping must be rejected")
	}
	mapped := &Account{Platform: PlatformOpenAI, Credentials: map[string]any{
		"base_url":      "https://provider.example",
		"model_mapping": map[string]any{"video-v3": "video-v3"},
	}}
	if !OpenAICompatibleVideoModelSupported(mapped, "video-v3") {
		t.Fatal("mapped video model should be accepted")
	}
}

func TestPrepareAutoDLVideoBody(t *testing.T) {
	body, err := prepareAutoDLVideoBody([]byte(`{"model":"minimax_h3_z0901","prompt":"x","seconds":"8","duration":8,"resolution":"720p","size":"1280x720","aspect_ratio":"16:9","images":["https://img/1.png",{"url":"https://img/2.png"}],"audios":["https://audio/1.mp3"]}`), "minimax_h3_z0901")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["model"]; ok {
		t.Fatal("model must be removed")
	}
	if _, ok := got["images"]; ok {
		t.Fatal("images must be removed")
	}
	if got["ref_image_0"] != "https://img/1.png" || got["ref_image_1"] != "https://img/2.png" {
		t.Fatalf("mapped images = %#v", got)
	}
	if got["ref_audio_0"] != "https://audio/1.mp3" {
		t.Fatalf("mapped audio = %#v", got)
	}
	if got["duration"] != float64(8) || got["resolution"] != "768p横(1344*768)" {
		t.Fatalf("normalized AutoDL fields = duration=%#v resolution=%#v", got["duration"], got["resolution"])
	}
	for _, field := range []string{"seconds", "size", "ratio", "generateAudio", "watermark"} {
		if _, ok := got[field]; ok {
			t.Fatalf("compatibility field %s must be removed", field)
		}
	}
	if _, ok := got["aspect_ratio"]; ok {
		t.Fatal("aspect_ratio must not be sent to AutoDL")
	}
}

func TestPrepareAutoDLVideoBodyPreservesWorkflowResolutionEnums(t *testing.T) {
	body, err := prepareAutoDLVideoBody([]byte(`{"prompt":"x","duration":"3","resolution":"480p","aspect_ratio":"9:16","size":"720x1280"}`), "minimax_h3_zm_u08")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["duration"] != float64(3) || got["resolution"] != "480p竖" {
		t.Fatalf("normalized ZM fields = %#v", got)
	}
}

func TestPrepareAutoDLVideoBodyMapsMinimaxFirstLastFrames(t *testing.T) {
	body, err := prepareAutoDLVideoBody([]byte(`{"model":"minimax_h3_b99_002","prompt":"x","duration":5,"images":["https://img/first.png","https://img/last.png"]}`), "minimax_h3_b99_002")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["first_frame"] != "https://img/first.png" || got["last_frame"] != "https://img/last.png" {
		t.Fatalf("mapped frames = %#v", got)
	}
	if _, ok := got["images"]; ok {
		t.Fatal("images must be removed for b99_002")
	}
}

func TestPrepareAutoDLVideoBodyRemovesMotionUnsupportedFields(t *testing.T) {
	body, err := prepareAutoDLVideoBody([]byte(`{"model":"wan2.2animate-v4-motion_retargeting","prompt":"x","duration":5,"seed":1,"ref_image":"https://img/a.png","ref_video":"https://vid/a.mp4"}`), "wan2.2animate-v4-motion_retargeting")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["prompt"]; ok {
		t.Fatal("prompt must be removed for motion retargeting")
	}
	if _, ok := got["duration"]; ok {
		t.Fatal("duration must be removed for motion retargeting")
	}
	if got["ref_image"] != "https://img/a.png" || got["ref_video"] != "https://vid/a.mp4" {
		t.Fatalf("motion fields = %#v", got)
	}
}

func TestRewriteAutoDLVideoResultURL(t *testing.T) {
	body := rewriteAutoDLVideoResultURL([]byte(`{"code":"Success","data":{"task_id":"task-1","status":"SUCCESS","url":"https://autodl.art/top-level.mp4","results":[{"url":"https://autodl.art/short-lived.mp4","type":"video"}]}}`), "task-1", "/v1/videos/task-1/content")
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	data := got["data"].(map[string]any)
	if data["url"] != "/v1/videos/task-1/content" {
		t.Fatalf("rewritten top-level URL = %#v", data["url"])
	}
	url := data["results"].([]any)[0].(map[string]any)["url"]
	if url != "/v1/videos/task-1/content" {
		t.Fatalf("rewritten result URL = %#v", url)
	}
}
