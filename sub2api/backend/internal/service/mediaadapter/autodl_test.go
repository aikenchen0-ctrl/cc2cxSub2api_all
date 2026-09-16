package mediaadapter

import (
	"encoding/json"
	"testing"
)

func TestAutoDLWorkflowForModel(t *testing.T) {
	tests := map[string]string{
		"minimax_h3_z0901":                    "minimax_h3_z0901",
		"minimax_h3_z0902":                    "minimax_h3_z0902",
		"minimax_h3_z0903":                    "minimax_h3_z0903",
		"minimax_h3_zm_u24":                   "minimax_h3_zm_u24",
		"minimax_h3_zm_u08":                   "minimax_h3_zm_u08",
		"minimax_h3_b99_001":                 "minimax_h3_b99_001",
		"minimax_h3_b99_002":                 "minimax_h3_b99_002",
		"minimax_h3_b99_003_12s":             "minimax_h3_b99_003_12s",
		"minimax_h3_lightx2v_v5_15s":         "minimax_h3_lightx2v_v5_15s",
		"minimax_h3_image_audio_to_video_v2": "minimax_h3_image_audio_to_video_v2",
		"minimax_h3_image_audio_to_video":    "minimax_h3_image_audio_to_video",
		"minimax_h3_lightx2v_v5":             "minimax_h3_lightx2v_v5",
		"minimax_h3_lightx2v_no_pic":         "minimax_h3_lightx2v_no_pic",
		"wan2.2animate-v4-motion_retargeting": "wan2.2animate-v4-motion_retargeting",
	}
	for model, want := range tests {
		if got, ok := AutoDLWorkflowForModel(model); !ok || got != want {
			t.Fatalf("AutoDLWorkflowForModel(%q) = %q, %v; want %q, true", model, got, ok, want)
		}
	}
	if _, ok := AutoDLWorkflowForModel("seedance-2.0"); ok {
		t.Fatal("unexpected AutoDL workflow for seedance")
	}
}

func TestAutoDLWorkflowModelsReturnsCompleteStableCatalog(t *testing.T) {
	models := AutoDLWorkflowModels()
	if len(models) != 16 {
		t.Fatalf("AutoDLWorkflowModels() returned %d models, want 16", len(models))
	}
	for i := 1; i < len(models); i++ {
		if models[i-1] >= models[i] {
			t.Fatalf("catalog is not strictly sorted: %q before %q", models[i-1], models[i])
		}
	}
}

func TestAutoDLResponseNormalizesEnvelope(t *testing.T) {
	input := []byte(`{"msg":"","code":"Success","data":{"status":"completed","results":[{"url":"https://cdn/video.mp4"}],"task_id":"task-1"}}`)
	out, err := NormalizeAutoDLResponse(input)
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != "task-1" || out.Status != VideoCompleted || out.VideoURL != "https://cdn/video.mp4" {
		t.Fatalf("out = %#v", out)
	}
	if !out.Success || out.Code != "Success" {
		t.Fatalf("success envelope metadata = %#v", out)
	}
	if !json.Valid(out.Raw) {
		t.Fatal("raw response is not valid JSON")
	}
}

func TestAutoDLResponseCapturesBusinessErrorEnvelope(t *testing.T) {
	out, err := NormalizeAutoDLResponse([]byte(`{"msg":"缺少必填参数：ref_audio_0","code":"RequestParameterIsWrong","data":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.Success || out.Code != "RequestParameterIsWrong" || out.Message == "" {
		t.Fatalf("business error envelope = %#v", out)
	}
}
