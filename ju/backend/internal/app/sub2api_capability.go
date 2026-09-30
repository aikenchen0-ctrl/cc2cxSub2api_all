package app

import (
	"fmt"
	"strings"

	"infinite-canvas/backend/internal/model"
)

// Only publish workflows whose input contract is supported by both this app
// and the Sub2API gateway. Workflow-specific body mapping stays in Sub2API.
func sub2APIVideoCapabilityConfig(name string) (*ModelCapabilityConfig, error) {
	if workflow, ok := sub2APIAutoDLWorkflowByID(strings.TrimSpace(name)); ok {
		durationSupported := workflow.DurationSupported
		video := &VideoCapabilityConfig{
			References: VideoReferenceConfig{
				PromptMaxChars: workflow.PromptMaxChars,
				MinImages:      workflow.MinImages, MaxImages: workflow.MaxImages, MaxImageBytes: 30 * 1024 * 1024,
				MaxVideos: workflow.MaxVideos, MaxVideoBytes: 200 * 1024 * 1024, MaxVideoDuration: 30,
				MaxAudios: workflow.MaxAudios, MaxAudioBytes: 15 * 1024 * 1024, MaxAudioDuration: 30,
			},
			Duration:          VideoDurationConfig{Selection: "range", Min: workflow.DurationMin, Max: workflow.DurationMax, Step: 1, Default: workflow.DurationDefault},
			DurationSupported: &durationSupported,
			Ratios:            workflow.Ratios, DefaultRatio: workflow.DefaultRatio,
			Resolutions: workflow.Resolutions, DefaultResolution: workflow.DefaultResolution,
			Operations: workflow.Operations, DefaultOperation: workflow.DefaultOperation,
		}
		return &ModelCapabilityConfig{Version: 1, Video: video}, nil
	}
	video := &VideoCapabilityConfig{
		References: VideoReferenceConfig{PromptMaxChars: 1000},
		Duration:   VideoDurationConfig{Selection: "range", Min: 1, Max: 15, Step: 1, Default: 5},
		Ratios:     []string{"16:9", "9:16"}, DefaultRatio: "9:16",
		Resolutions: []string{"736p"}, DefaultResolution: "736p",
		Operations: []string{"text_to_video"}, DefaultOperation: "text_to_video",
	}
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "grok-imagine-video-1.5", "grok-imagine-video", "seedance-2.0", "seedance-2.0-fast",
		"kling-v1", "kling-v1-5", "kling-v1-6", "kling-v2-5-turbo", "kling-v2-6", "kling-v3", "kling-v3-omni":
		return DefaultModelCapabilityConfigForModel(sub2APIVideoProtocol, normalized), nil
	case "minimax", "minimax-h3", "minimax_h3":
	default:
		return nil, fmt.Errorf("SUB2API_RELAY_VIDEO_MODELS contains unsupported model %q", name)
	}
	return &ModelCapabilityConfig{Version: 1, Video: video}, nil
}

func sub2APIImageProtocol(name string) model.ChannelInterfaceType {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "grok-imagine" || n == "grok-imagine-edit" || strings.HasPrefix(n, "grok-imagine-image") {
		return model.ChannelInterfaceGrokImage
	}
	return model.ChannelInterfaceOpenAIImage
}
