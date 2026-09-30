package service

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func TestManagedAutoDLVideoCatalog(t *testing.T) {
	t.Setenv("LINK", "https://relay.example")
	t.Setenv("SUB2API_RELAY_BASE_URL", "")
	t.Setenv("SUB2API_APP_CREDENTIAL", "application-credential")
	t.Setenv("SUB2API_RELAY_VIDEO_MODELS", "grok-imagine-video-1.5")
	channel, err := configuredSub2APIChannel()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../web/src/lib/autodl-video-workflows.json")
	if err != nil {
		t.Fatal(err)
	}
	var workflows []AutoDLWorkflow
	if err := json.Unmarshal(data, &workflows); err != nil {
		t.Fatal(err)
	}
	if len(workflows) != 16 || len(AutoDLVideoModels) != len(workflows) {
		t.Fatal("video catalog is incomplete")
	}
	for _, workflow := range workflows {
		if !slices.Contains(channel.Models, workflow.UUID) || AutoDLModelKind(workflow.UUID) != "video" {
			t.Fatalf("missing managed video %s", workflow.UUID)
		}
		if !isVideoModelName(workflow.UUID) || isImageModelName(workflow.UUID) || isTextModelName(workflow.UUID) {
			t.Fatalf("incorrect capability %s", workflow.UUID)
		}
	}
	if slices.Contains(channel.Models, "indextts2-v1") {
		t.Fatal("audio workflow included in video presets")
	}
}
