package app

import (
	"fmt"
	"strings"

	"infinite-canvas/backend/internal/model"
)

// Only publish workflows whose input contract is supported by both this app
// and the Sub2API gateway. Workflow-specific body mapping stays in Sub2API.
func sub2APIVideoCapabilityConfig(name string) (*ModelCapabilityConfig, error) {
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
	case "minimax", "minimax-h3", "minimax_h3", "minimax_h3_b99_001":
	case "minimax_h3_b99_002":
		video.References.MinImages, video.References.MaxImages = 2, 2
		video.References.MaxImageBytes = 30 * 1024 * 1024
		video.Operations, video.DefaultOperation = []string{"image_to_video"}, "image_to_video"
	case "minimax_h3_b99_003_12s":
		video.References.PromptMaxChars = 10000
		video.References.MinImages, video.References.MaxImages = 1, 9
		video.References.MaxImageBytes = 30 * 1024 * 1024
		video.Duration.Max = 12
		video.Operations, video.DefaultOperation = []string{"image_to_video", "reference_to_video"}, "image_to_video"
	default:
		return nil, fmt.Errorf("SUB2API_RELAY_VIDEO_MODELS contains unsupported model %q; supported models: grok-imagine-video-1.5, seedance-2.0, kling-v3, minimax_h3_b99_001, minimax_h3_b99_002, minimax_h3_b99_003_12s", name)
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
