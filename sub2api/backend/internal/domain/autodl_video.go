package domain

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// Public workflow metadata only; never embed tokens or upstream account data.
// Keep the Canvas snapshot in sync when refreshing the AutoDL catalog.
//
//go:embed autodl_video_workflows.json
var autoDLVideoCatalog []byte

type AutoDLInputRule struct {
	Type      string   `json:"type"`
	Required  bool     `json:"required"`
	Default   any      `json:"default,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	MinLength int      `json:"min_length,omitempty"`
	MaxLength int      `json:"max_length,omitempty"`
	Options   []struct {
		Label string `json:"label"`
	} `json:"options,omitempty"`
}

type AutoDLVideoWorkflow struct {
	UUID       string                     `json:"uuid"`
	Name       string                     `json:"name"`
	Kind       string                     `json:"kind"`
	InputRules map[string]AutoDLInputRule `json:"input_rules"`
}

var AutoDLVideoWorkflows = func() []AutoDLVideoWorkflow {
	var workflows []AutoDLVideoWorkflow
	if err := json.Unmarshal(autoDLVideoCatalog, &workflows); err != nil {
		panic(err)
	}
	return workflows
}()

func FindAutoDLVideoWorkflow(model string) (AutoDLVideoWorkflow, bool) {
	for _, workflow := range AutoDLVideoWorkflows {
		if workflow.UUID == strings.TrimSpace(model) {
			return workflow, true
		}
	}
	return AutoDLVideoWorkflow{}, false
}

func AutoDLVideoModels() []string {
	models := make([]string, 0, len(AutoDLVideoWorkflows))
	for _, workflow := range AutoDLVideoWorkflows {
		models = append(models, workflow.UUID)
	}
	return models
}
