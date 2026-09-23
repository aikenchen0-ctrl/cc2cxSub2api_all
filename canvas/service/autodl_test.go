package service

import "testing"

func TestAutoDLModelKindVideoWorkflows(t *testing.T) {
	for _, modelName := range []string{
		"minimax_h3_z0901",
		"minimax_h3_z0902",
		"minimax_h3_z0903",
	} {
		if kind := AutoDLModelKind(modelName); kind != "video" {
			t.Errorf("AutoDLModelKind(%q) = %q, want video", modelName, kind)
		}
	}
}

func TestAutoDLDefaultVideoWorkflowCatalog(t *testing.T) {
	for _, modelName := range []string{
		"minimax_h3_z0901", "minimax_h3_z0902", "minimax_h3_z0903",
		"minimax_h3_zm_u24", "minimax_h3_zm_u08", "minimax_h3_b99_001", "minimax_h3_b99_002", "minimax_h3_b99_003_12s",
		"wan2.2animate-v4-motion_retargeting", "minimax_h3_image_audio_to_video_v2_15s", "minimax_h3_lightx2v_v5_15s",
		"minimax_h3_image_audio_to_video_v2", "minimax_h3_image_audio_to_video", "minimax_h3_lightx2v_v5", "minimax_h3_lightx2v_no_pic", "minimax_h3_lightx2v",
	} {
		if kind := AutoDLModelKind(modelName); kind != "video" {
			t.Errorf("documented AutoDL workflow %q has kind %q", modelName, kind)
		}
	}
}

func TestAmamPresetModelsAreVideoModels(t *testing.T) {
	for _, modelName := range []string{
		"xinghe-2.0", "xinghe-fast", "xinghe-mini", "A-SD2.0",
		"zhiying-seedance-2.5-line2-1080p", "zhiying-wan3.0-video",
		"zhiying-minimax-h3-2k", "zhiying-minimax-h3-768p",
		"zhiying-seedance-2.5-720p", "zhiying-minimax-h3-4k",
		"zhiying-seedance-2.5-line2-720p", "zhiying-wan3.0-prime",
		"zhiying-seedance-2.5-480p", "zhiying-seedance-2.5-line2-480p",
		"zhiying-google-omni-1080p", "zhiying-minimax-h3-1080p",
		"zhiying-seedance-2.5-line1", "sd-2.5-30秒",
	} {
		if !isVideoModelName(modelName) {
			t.Errorf("isVideoModelName(%q) = false, want true", modelName)
		}
	}
}
