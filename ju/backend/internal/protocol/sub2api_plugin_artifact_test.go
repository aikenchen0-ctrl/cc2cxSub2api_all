package protocol

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSub2APIVideoPlugin(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "plugin-packages", "sub2api-video-v1.yingce-plugin"))
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := ParsePluginPackage(data)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "plugin-packages", "sub2api-video-v1", "manifest.json"))
	if err != nil || !bytes.Equal(source, pkg.ManifestRaw) {
		t.Fatal("packaged manifest must match its source")
	}
	adapters, err := LoadInstalledProviders(pkg.ManifestRaw, nil)
	if err != nil || len(adapters) != 1 {
		t.Fatalf("providers = %d, err = %v", len(adapters), err)
	}
	a := adapters[0]
	if a.Metadata().Execution != "declarative" || !a.Metadata().RequiresPublicMediaURLs {
		t.Fatal("expected declarative public-media adapter")
	}
	spec, err := a.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{
		Model: "minimax_h3_b99_002", Prompt: "scene", Duration: 5, Resolution: "736p", AspectRatio: "16:9",
		Images: []MediaReference{{URL: "https://media.example/last.png", Role: "last_frame", Order: 0}, {URL: "https://media.example/first.png", Role: "first_frame", Order: 1}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	body := spec.Body.(map[string]any)
	if spec.Path != "/v1/videos" || spec.ContentType != "application/json" || spec.Auth.Type != "bearer" || body["duration"] != 5 {
		t.Fatalf("spec = %#v", spec)
	}
	if !reflect.DeepEqual(body["images"], []any{"https://media.example/first.png", "https://media.example/last.png"}) {
		t.Fatalf("images = %#v", body["images"])
	}
	if _, err := a.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{Model: "indextts2-v1"}}); err == nil {
		t.Fatal("audio workflow accepted as video")
	}
	for _, tc := range []struct {
		body   string
		status Status
		id     string
	}{
		{`{"code":"Success","data":{"task_id":"job","status":"queued"}}`, StatusPending, "job"},
		{`{"code":200,"data":{"id":"job","status":"running"}}`, StatusProcessing, "job"},
		{`{"id":"job","status":"completed"}`, StatusSucceeded, "job"},
		{`{"code":"RequestParameterIsWrong","data":null,"msg":"invalid"}`, StatusFailed, ""},
		{`{"error":{"message":"denied"}}`, StatusFailed, ""},
	} {
		got, err := a.ParseCreate(context.Background(), []byte(tc.body))
		if err != nil || got.Status != tc.status || got.TaskID != tc.id {
			t.Fatalf("parse %s = %#v, %v", tc.body, got, err)
		}
	}
	got, err := a.ParsePoll(context.Background(), PollContext{TaskID: "job"}, []byte(`{"code":"Success","data":{"task_id":"job","status":"completed","results":[{"url":"https://untrusted.example/video.mp4"}]}}`))
	if err != nil || got.Status != StatusSucceeded || got.Result != nil {
		t.Fatalf("must ignore external URLs: %#v, %v", got, err)
	}
	resultSpec, err := a.(ResultAdapter).BuildResult(context.Background(), PollContext{TaskID: "job"})
	if err != nil || resultSpec.Path != "/v1/videos/job/content" || resultSpec.Auth.Type != "bearer" {
		t.Fatalf("result spec = %#v, %v", resultSpec, err)
	}
}
