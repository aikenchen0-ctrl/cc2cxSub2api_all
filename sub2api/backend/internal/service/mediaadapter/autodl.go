package mediaadapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const ProfileAutoDL Profile = "autodl"

var autodlWorkflows = map[string]string{
	"minimax_h3_z0901":                    "minimax_h3_z0901",
	"minimax_h3_z0902":                    "minimax_h3_z0902",
	"minimax_h3_z0903":                    "minimax_h3_z0903",
	"minimax_h3_zm_u24":                   "minimax_h3_zm_u24",
	"minimax_h3_zm_u08":                   "minimax_h3_zm_u08",
	"minimax_h3_b99_001":                     "minimax_h3_b99_001",
	"minimax_h3_b99_002":                     "minimax_h3_b99_002",
	"minimax_h3_b99_003_12s":                 "minimax_h3_b99_003_12s",
	"minimax_h3_image_audio_to_video_v2_15s": "minimax_h3_image_audio_to_video_v2_15s",
	"minimax_h3_lightx2v_v5_15s":             "minimax_h3_lightx2v_v5_15s",
	"minimax_h3_image_audio_to_video_v2":     "minimax_h3_image_audio_to_video_v2",
	"minimax_h3_image_audio_to_video":        "minimax_h3_image_audio_to_video",
	"minimax_h3_lightx2v_v5":                 "minimax_h3_lightx2v_v5",
	"minimax_h3_lightx2v_no_pic":             "minimax_h3_lightx2v_no_pic",
	"minimax_h3_lightx2v":                    "minimax_h3_lightx2v",
	"wan2.2animate-v4-motion_retargeting":   "wan2.2animate-v4-motion_retargeting",
}

var autodlModelAliases = map[string]string{
	"minimax":    "minimax_h3_b99_001",
	"minimax-h3": "minimax_h3_b99_001",
	"minimax_h3": "minimax_h3_b99_001",
}

// AutoDLWorkflowModels returns the canonical workflow model IDs in stable
// order for model discovery endpoints and UI catalogs.
func AutoDLWorkflowModels() []string {
	models := make([]string, 0, len(autodlWorkflows))
	for model := range autodlWorkflows {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}

func AutoDLWorkflowForModel(model string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(model))
	if canonical, ok := autodlModelAliases[name]; ok {
		name = canonical
	}
	w, ok := autodlWorkflows[name]
	return w, ok
}

type AutoDLResponse struct {
	ID       string
	Status   VideoStatus
	VideoURL string
	Code     string
	Message  string
	Success  bool
	Raw      []byte
}

func NormalizeAutoDLResponse(payload []byte) (AutoDLResponse, error) {
	var envelope struct {
		Code    json.RawMessage `json:"code"`
		Msg     string          `json:"msg"`
		Message string          `json:"message"`
		Data json.RawMessage `json:"data"`
	}
	data := bytes.TrimSpace(payload)
	if err := json.Unmarshal(data, &envelope); err != nil {
		return AutoDLResponse{}, fmt.Errorf("invalid AutoDL response: %w", err)
	}
	code := strings.TrimSpace(string(envelope.Code))
	if strings.HasPrefix(code, "\"") {
		var decoded string
		if err := json.Unmarshal(envelope.Code, &decoded); err == nil {
			code = strings.TrimSpace(decoded)
		}
	}
	message := strings.TrimSpace(envelope.Msg)
	if message == "" {
		message = strings.TrimSpace(envelope.Message)
	}
	success := code == "" || code == "0" || code == "200" || code == "201" || strings.EqualFold(code, "success")
	if len(envelope.Data) > 0 && !bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
		data = envelope.Data
	}
	var body struct {
		TaskID  string `json:"task_id"`
		ID      string `json:"id"`
		Status  string `json:"status"`
		State   string `json:"state"`
		Results []struct {
			URL string `json:"url"`
		} `json:"results"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return AutoDLResponse{}, fmt.Errorf("invalid AutoDL data: %w", err)
	}
	id := body.TaskID
	if id == "" {
		id = body.ID
	}
	videoURL := body.URL
	if videoURL == "" && len(body.Results) > 0 {
		videoURL = body.Results[0].URL
	}
	status := strings.ToLower(strings.TrimSpace(body.Status))
	if status == "" {
		status = strings.ToLower(strings.TrimSpace(body.State))
	}
	var normalized VideoStatus
	switch status {
	case "completed", "complete", "done", "succeeded", "success":
		normalized = VideoCompleted
	case "processing", "running", "started", "in_progress":
		normalized = VideoInProgress
	case "pending", "queued", "created":
		normalized = VideoQueued
	case "failed", "failure", "error":
		normalized = VideoFailed
	case "canceled", "cancelled":
		normalized = VideoCancelled
	default:
		normalized = VideoStatus(status)
	}
	return AutoDLResponse{ID: id, Status: normalized, VideoURL: videoURL, Code: code, Message: message, Success: success, Raw: append([]byte(nil), payload...)}, nil
}
