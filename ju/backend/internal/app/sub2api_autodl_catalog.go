package app

// sub2APIAutoDLWorkflow is public catalog metadata only. The English ID is
// sent to Sub2API; displayName is used only by the JU frontend.
type sub2APIAutoDLWorkflow struct {
	ID                string
	DisplayName       string
	PromptMaxChars    int
	MinImages         int
	MaxImages         int
	MaxVideos         int
	MaxAudios         int
	DurationMin       int
	DurationMax       int
	DurationDefault   int
	DurationSupported bool
	Resolutions       []string
	DefaultResolution string
	Ratios            []string
	DefaultRatio      string
	Operations        []string
	DefaultOperation  string
}

var sub2APIAutoDLWorkflows = []sub2APIAutoDLWorkflow{
	// Put the prompt-only high-quality workflow first: it is the default video
	// model for a new SSO user and works without reference media.
	{ID: "minimax_h3_z0901", DisplayName: "H3文生视频（高质量创意直出）", PromptMaxChars: 10000, DurationMin: 1, DurationMax: 15, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"480p竖(480*864)", "480p横(864*480)", "768p竖(768*1344)", "768p横(1344*768)", "1088p竖(1088*1920)", "1088p横(1920*1088)", "1440p竖(1440*2560)", "1440p横(2560*1440)"}, DefaultResolution: "768p竖(768*1344)", Ratios: []string{"9:16", "16:9"}, DefaultRatio: "9:16", Operations: []string{"text_to_video"}, DefaultOperation: "text_to_video"},
	{ID: "minimax_h3_z0903", DisplayName: "H3六图三音频生视频（高质量音画融合）", PromptMaxChars: 10000, MinImages: 1, MaxImages: 6, MaxAudios: 3, DurationMin: 1, DurationMax: 15, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"480p竖(480*864)", "480p横(864*480)", "768p竖(768*1376)", "768p横(1376*768)", "1088p竖(1088*1920)", "1088p横(1920*1088)", "1440p竖(1440*2560)", "1440p横(2560*1440)"}, DefaultResolution: "768p竖(768*1376)", Ratios: []string{"9:16", "16:9"}, DefaultRatio: "9:16", Operations: []string{"image_to_video", "reference_to_video"}, DefaultOperation: "image_to_video"},
	{ID: "minimax_h3_z0902", DisplayName: "H3六图生视频（多图一致性创作）", PromptMaxChars: 10000, MinImages: 1, MaxImages: 6, DurationMin: 1, DurationMax: 15, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"480p竖(480*864)", "480p横(864*480)", "768p竖(768*1376)", "768p横(1376*768)", "1088p竖(1088*1920)", "1088p横(1920*1088)", "1440p竖(1440*2560)", "1440p横(2560*1440)"}, DefaultResolution: "768p竖(768*1376)", Ratios: []string{"9:16", "16:9"}, DefaultRatio: "9:16", Operations: []string{"image_to_video", "reference_to_video"}, DefaultOperation: "image_to_video"},
	{ID: "minimax_h3_zm_u24", DisplayName: "H3多图多音频生视频（升级画质）", PromptMaxChars: 10000, MinImages: 1, MaxImages: 9, MaxAudios: 3, DurationMin: 1, DurationMax: 15, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"480p横", "480p竖", "768p横", "768p竖", "480p(1:1)", "768p(1:1)"}, DefaultResolution: "768p竖", Ratios: []string{"16:9", "9:16", "1:1"}, DefaultRatio: "9:16", Operations: []string{"image_to_video", "reference_to_video"}, DefaultOperation: "image_to_video"},
	{ID: "minimax_h3_zm_u08", DisplayName: "H3多图多音频生视频（高速版）", PromptMaxChars: 10000, MinImages: 1, MaxImages: 9, MaxAudios: 3, DurationMin: 1, DurationMax: 15, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"480p横", "480p竖", "768p横", "768p竖", "480p(1:1)", "768p(1:1)"}, DefaultResolution: "768p竖", Ratios: []string{"16:9", "9:16", "1:1"}, DefaultRatio: "9:16", Operations: []string{"image_to_video", "reference_to_video"}, DefaultOperation: "image_to_video"},
	{ID: "minimax_h3_b99_002", DisplayName: "H3首尾帧生成视频", PromptMaxChars: 1000, MinImages: 2, MaxImages: 2, DurationMin: 1, DurationMax: 15, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"736p竖", "736p横", "736p(1:1)"}, DefaultResolution: "736p竖", Ratios: []string{"9:16", "16:9", "1:1"}, DefaultRatio: "9:16", Operations: []string{"image_to_video"}, DefaultOperation: "image_to_video"},
	{ID: "minimax_h3_b99_001", DisplayName: "H3文生视频", PromptMaxChars: 1000, DurationMin: 1, DurationMax: 15, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"736p竖", "736p横", "736p(1:1)"}, DefaultResolution: "736p竖", Ratios: []string{"9:16", "16:9", "1:1"}, DefaultRatio: "9:16", Operations: []string{"text_to_video"}, DefaultOperation: "text_to_video"},
	{ID: "minimax_h3_b99_003_12s", DisplayName: "H3多图生视频12秒", PromptMaxChars: 10000, MinImages: 1, MaxImages: 9, DurationMin: 1, DurationMax: 12, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"736p竖", "736p横", "736p(1:1)"}, DefaultResolution: "736p竖", Ratios: []string{"9:16", "16:9", "1:1"}, DefaultRatio: "9:16", Operations: []string{"image_to_video", "reference_to_video"}, DefaultOperation: "image_to_video"},
	{ID: "wan2.2animate-v4-motion_retargeting", DisplayName: "动作迁移", PromptMaxChars: 0, MinImages: 1, MaxImages: 1, MaxVideos: 1, DurationDefault: 0, DurationSupported: false, Resolutions: []string{"464*832px(竖版)", "832*464px(横版)"}, DefaultResolution: "464*832px(竖版)", Ratios: []string{"9:16", "16:9"}, DefaultRatio: "9:16", Operations: []string{"reference_to_video"}, DefaultOperation: "reference_to_video"},
	{ID: "minimax_h3_image_audio_to_video_v2_15s", DisplayName: "H3多图多音频生视频15秒", PromptMaxChars: 10000, MaxImages: 9, MaxAudios: 3, DurationMin: 1, DurationMax: 15, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"480p竖", "768p竖", "480p横", "768p横"}, DefaultResolution: "768p竖", Ratios: []string{"9:16", "16:9"}, DefaultRatio: "9:16", Operations: []string{"text_to_video", "image_to_video", "reference_to_video", "audio_to_video"}, DefaultOperation: "text_to_video"},
	{ID: "minimax_h3_lightx2v_v5_15s", DisplayName: "H3多图生视频15秒", PromptMaxChars: 500000, MinImages: 1, MaxImages: 9, DurationMin: 1, DurationMax: 15, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"480p竖", "768p竖", "480p横", "768p横", "480p(1:1)", "768p(1:1)"}, DefaultResolution: "768p竖", Ratios: []string{"9:16", "16:9", "1:1"}, DefaultRatio: "9:16", Operations: []string{"image_to_video", "reference_to_video"}, DefaultOperation: "image_to_video"},
	{ID: "minimax_h3_image_audio_to_video_v2", DisplayName: "H3多图多音频生视频", PromptMaxChars: 10000, MaxImages: 9, MaxAudios: 3, DurationMin: 1, DurationMax: 10, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"480p竖", "768p竖", "1080p竖", "480p横", "768p横", "1080p横"}, DefaultResolution: "768p竖", Ratios: []string{"9:16", "16:9"}, DefaultRatio: "9:16", Operations: []string{"text_to_video", "image_to_video", "reference_to_video", "audio_to_video"}, DefaultOperation: "text_to_video"},
	{ID: "minimax_h3_image_audio_to_video", DisplayName: "H3图生视频-音频同步（自动对口型）", PromptMaxChars: 0, MinImages: 1, MaxImages: 1, MaxAudios: 1, DurationMin: 1, DurationMax: 15, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"480p竖", "768p竖", "1080p竖", "480p横", "768p横", "1080p横"}, DefaultResolution: "768p竖", Ratios: []string{"9:16", "16:9"}, DefaultRatio: "9:16", Operations: []string{"image_to_video"}, DefaultOperation: "image_to_video"},
	{ID: "minimax_h3_lightx2v_v5", DisplayName: "H3多图参考生视频", PromptMaxChars: 500000, MinImages: 1, MaxImages: 9, DurationMin: 1, DurationMax: 10, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"480p竖", "768p竖", "1080p竖", "480p横", "768p横", "1080p横", "480p(1:1)", "768p(1:1)", "1080p(1:1)"}, DefaultResolution: "768p竖", Ratios: []string{"9:16", "16:9", "1:1"}, DefaultRatio: "9:16", Operations: []string{"image_to_video", "reference_to_video"}, DefaultOperation: "image_to_video"},
	{ID: "minimax_h3_lightx2v_no_pic", DisplayName: "H3文生视频", PromptMaxChars: 200000, DurationMin: 1, DurationMax: 15, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"480p竖", "768p竖", "480p横", "768p横", "480p(1:1)", "768p(1:1)"}, DefaultResolution: "768p竖", Ratios: []string{"9:16", "16:9", "1:1"}, DefaultRatio: "9:16", Operations: []string{"text_to_video"}, DefaultOperation: "text_to_video"},
	{ID: "minimax_h3_lightx2v", DisplayName: "H3首尾帧生成视频", PromptMaxChars: 2000000, MinImages: 2, MaxImages: 2, DurationMin: 1, DurationMax: 15, DurationDefault: 5, DurationSupported: true, Resolutions: []string{"480p竖", "768p竖", "480p横", "768p横", "480p(1:1)", "768p(1:1)"}, DefaultResolution: "768p竖", Ratios: []string{"9:16", "16:9", "1:1"}, DefaultRatio: "9:16", Operations: []string{"image_to_video"}, DefaultOperation: "image_to_video"},
}

func sub2APIAutoDLWorkflowByID(id string) (sub2APIAutoDLWorkflow, bool) {
	for _, workflow := range sub2APIAutoDLWorkflows {
		if workflow.ID == id {
			return workflow, true
		}
	}
	return sub2APIAutoDLWorkflow{}, false
}

func sub2APIAutoDLModelNames() []string {
	models := make([]string, 0, len(sub2APIAutoDLWorkflows))
	for _, workflow := range sub2APIAutoDLWorkflows {
		models = append(models, workflow.ID)
	}
	return models
}
