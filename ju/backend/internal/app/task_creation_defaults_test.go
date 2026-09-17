package app

import (
	"encoding/json"
	"testing"
)

func TestChannelCapabilityDefaultsRespectProviderWireTypes(t *testing.T) {
	video, err := sub2APIVideoCapabilityConfig("minimax_h3_b99_001")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		capability string
		profile    *ModelCapabilityConfig
	}{
		{"video", video},
		{"image", DefaultModelCapabilityConfigForModel("openai-image", "gpt-image-1")},
	} {
		t.Run(test.capability, func(t *testing.T) {
			config := map[string]any{}
			applyChannelCapabilityDefaults(config, test.capability, test.profile)
			encoded, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			var decoded providerConfig
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("default options cannot reach the worker: %s: %v", encoded, err)
			}
			if test.capability == "video" && (decoded.VideoSeconds != "5" || decoded.VideoGenerateAudio != "false" || decoded.VideoWatermark != "false") {
				t.Fatalf("incorrect video defaults: %s", encoded)
			}
			if test.capability == "image" && (decoded.Count != "1" || decoded.TransparentBackground != "false") {
				t.Fatalf("incorrect image defaults: %s", encoded)
			}
		})
	}
}

func TestChannelCapabilityDefaultsPreserveExplicitOptions(t *testing.T) {
	profile, err := sub2APIVideoCapabilityConfig("minimax_h3_b99_001")
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"videoSeconds": "8", "size": "16:9", "videoGenerateAudio": "false"}
	applyChannelCapabilityDefaults(config, "video", profile)
	if config["videoSeconds"] != "8" || config["size"] != "16:9" || config["videoGenerateAudio"] != "false" {
		t.Fatalf("explicit options changed: %#v", config)
	}
}
