//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	mp4 "github.com/abema/go-mp4"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAutoDLAllWorkflowRequests(t *testing.T) {
	for _, workflow := range domain.AutoDLVideoWorkflows {
		t.Run(workflow.UUID, func(t *testing.T) {
			input := map[string]any{"model": workflow.UUID}
			for field, rule := range workflow.InputRules {
				if !rule.Required {
					continue
				}
				value := any("a cinematic video")
				if rule.Type == "image" || rule.Type == "audio" || rule.Type == "video" {
					value = "https://cdn.example/reference"
				}
				input[field] = value
			}
			body, _ := json.Marshal(input)
			payload, duration, resolution, err := PrepareAutoDLVideoRequest(body)
			require.NoError(t, err)
			require.NotEmpty(t, resolution)
			if workflow.UUID != "wan2.2animate-v4-motion_retargeting" {
				require.Equal(t, 5, duration)
			}
			require.False(t, gjson.GetBytes(payload, "model").Exists())
			require.False(t, gjson.GetBytes(payload, "seed").Exists(), "omitted seed must remain random")
		})
	}
}

func TestAutoDLVideoValidation(t *testing.T) {
	for _, body := range []string{
		`{"model":"minimax_h3_z0903","prompt":"test"}`,
		`{"model":"minimax_h3_z0901","prompt":"test","seconds":16}`,
		`{"model":"minimax_h3_z0901","prompt":"test","resolution_name":"4k"}`,
		`{"model":"minimax_h3_z0901","prompt":"test","input_reference[]":["https://cdn.example/image"]}`,
		`{"model":"minimax_h3_lightx2v","prompt":"test","first_frame_url":"data:image/png;base64,xx","last_frame_url":"https://cdn.example/end"}`,
		`{"model":"minimax_h3_z0901","prompt":{}}`,
		`{"model":"minimax_h3_z0901","prompt":[]}`,
	} {
		_, _, _, err := PrepareAutoDLVideoRequest([]byte(body))
		require.Error(t, err, body)
	}
	body := []byte(`{"model":"minimax_h3_z0903","prompt":"test","seconds":"7","resolution_name":"480p横(864*480)","input_reference[]":["https://cdn.example/image"],"audio_reference[]":["https://cdn.example/audio"]}`)
	payload, seconds, resolution, err := PrepareAutoDLVideoRequest(body)
	require.NoError(t, err)
	require.Equal(t, 7, seconds)
	require.Equal(t, "480p", resolution)
	require.Equal(t, "https://cdn.example/image", gjson.GetBytes(payload, "ref_image_0").String())
	require.Equal(t, "https://cdn.example/audio", gjson.GetBytes(payload, "ref_audio_0").String())
}

func autoDLTestAccount() *Account {
	return &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://autodl.art/v1", "api_key": "test-autodl-token"}}
}

func TestAutoDLVideoRoutingAndURLs(t *testing.T) {
	a := autoDLTestAccount()
	require.True(t, a.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityAutoDLVideo))
	other := *a
	other.Credentials = map[string]any{"base_url": "https://other.example/v1"}
	require.False(t, other.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityAutoDLVideo))
	for _, base := range []string{"https://autodl.art", "https://autodl.art/v1", "https://autodl.art/api/v1/comfyui"} {
		a.Credentials["base_url"] = base
		u, err := autoDLVideoURL(a, &config.Config{}, "wan2.2animate-v4-motion_retargeting", false)
		require.NoError(t, err)
		require.Equal(t, "https://autodl.art/api/v1/comfyui/comfyui_workflow/wan2.2animate-v4-motion_retargeting", u)
		u, err = autoDLVideoURL(a, &config.Config{}, "autodl_test-task", true)
		require.NoError(t, err)
		require.Equal(t, "https://autodl.art/api/v1/comfyui/comfyui_workflow/result/test-task", u)
	}
	_, err := autoDLVideoURL(a, &config.Config{}, "autodl_../secret", true)
	require.Error(t, err)
}

func TestAutoDLVideoCreatePollAndContent(t *testing.T) {
	a := autoDLTestAccount()
	a.Credentials["model_mapping"] = map[string]any{"*": "gpt-5.5"}
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(`{"code":"Success","data":{"task_id":"task-1","status":"pending"}}`),
		grokMediaContentStatusResponse(`{"code":"Success","data":{"task_id":"task-1","status":"completed","results":[{"type":"video","url":"https://cdn.example/video.mp4"}]}}`),
		grokMediaContentStatusResponse(`{"code":"Success","data":{"task_id":"task-1","status":"completed","results":[{"type":"video","url":"https://cdn.example/video.mp4"}]}}`),
		{StatusCode: 200, Header: http.Header{"Content-Type": []string{"video/mp4"}}, Body: io.NopCloser(strings.NewReader("video-bytes"))},
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	c, recorder := grokMediaContentTestContext(http.MethodPost, "/v1/videos", nil)
	result, err := svc.ForwardGrokMedia(context.Background(), c, a, GrokMediaEndpointVideosGenerations, "", []byte(`{"model":"minimax_h3_z0901","prompt":"test","seconds":"7"}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, "autodl_task-1", result.ResponseID)
	require.Equal(t, 7, result.VideoDurationSeconds)
	require.Equal(t, "768p", result.VideoResolution)
	require.Equal(t, 0, result.VideoCount)
	require.Equal(t, "autodl_task-1", gjson.Get(recorder.Body.String(), "id").String())
	require.Equal(t, "test-autodl-token", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "/api/v1/comfyui/comfyui_workflow/minimax_h3_z0901", upstream.requests[0].URL.Path, "Chinese labels and chat aliases must not replace the English workflow ID")
	payload, err := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, err)
	require.Equal(t, int64(7), gjson.GetBytes(payload, "duration").Int())
	require.False(t, gjson.GetBytes(payload, "model").Exists())
	c, recorder = grokMediaContentTestContext(http.MethodGet, "/v1/videos/autodl_task-1", nil)
	result, err = svc.ForwardGrokMedia(context.Background(), c, a, GrokMediaEndpointVideoStatus, "autodl_task-1", nil, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.VideoCount)
	require.Empty(t, result.Model, "billing merges the original model from the owner-bound snapshot")
	require.Contains(t, gjson.Get(recorder.Body.String(), "video.url").String(), "/videos/autodl_task-1/content")
	c, recorder = grokMediaContentTestContext(http.MethodGet, "/v1/videos/autodl_task-1/content", nil)
	result, err = svc.ForwardGrokMedia(context.Background(), c, a, GrokMediaEndpointVideoContent, "autodl_task-1", nil, "")
	require.NoError(t, err)
	require.Equal(t, "video-bytes", recorder.Body.String())
	require.Equal(t, 1, result.VideoCount)
	require.Empty(t, result.Model)
	require.Equal(t, "test-autodl-token", upstream.requests[2].Header.Get("Authorization"))
	require.Empty(t, upstream.requests[3].Header.Get("Authorization"), "never send upstream credentials to the media CDN")
}

func TestAutoDLVideoFailureResponses(t *testing.T) {
	_, _, _, err := normalizeAutoDLVideoResponse([]byte(`{"code":"InsufficientBalance","msg":"余额不足"}`), "", true)
	require.ErrorContains(t, err, "余额不足")
	_, _, _, err = normalizeAutoDLVideoResponse([]byte(`{"code":"Success","data":{"status":"pending"}}`), "", true)
	require.Error(t, err)
	for _, status := range []string{"failed", "completed"} {
		body, _, done, err := normalizeAutoDLVideoResponse([]byte(`{"code":"Success","data":{"status":"`+status+`"}}`), "autodl_test", false)
		require.NoError(t, err)
		require.False(t, done)
		require.Equal(t, "failed", gjson.GetBytes(body, "status").String())
	}
}

func TestAutoDLVideoImmediateCompletionStillPolls(t *testing.T) {
	body, id, done, err := normalizeAutoDLVideoResponse([]byte(`{"code":"Success","data":{"task_id":"fast","status":"completed","results":[{"type":"video","url":"https://cdn.example/video.mp4"}]}}`), "", true)
	require.NoError(t, err)
	require.Equal(t, "autodl_fast", id)
	require.False(t, done)
	require.Equal(t, "pending", gjson.GetBytes(body, "status").String())
	require.False(t, gjson.GetBytes(body, "video.url").Exists())
}

func TestAutoDLVideoSuperKeyCrossGroupAndOwnerIsolation(t *testing.T) {
	groupID := int64(24)
	a := autoDLTestAccount()
	a.Status, a.Schedulable, a.Concurrency = StatusActive, true, 50
	a.GroupIDs, a.Groups = []int64{99}, []*Group{{ID: 99, Status: StatusActive}}
	a.Credentials["model_mapping"] = map[string]any{"gpt-5.5": "gpt-5.5"}
	svc := &OpenAIGatewayService{accountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{*a}}}, cache: &schedulerTestGatewayCache{}, cfg: &config.Config{}}
	ctx := context.WithValue(context.Background(), ctxkey.SuperAPIKey, true)
	selection, _, err := svc.SelectAccountWithSchedulerForCapability(ctx, &groupID, "", "create", "minimax_h3_z0901", nil, OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityAutoDLVideo, false, false, false, PlatformOpenAI)
	require.NoError(t, err)
	require.Equal(t, a.ID, selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
	require.NoError(t, svc.BindGrokMediaVideoRequestAccount(ctx, &groupID, "autodl_test", 10, 20, a.ID))
	for _, owner := range [][2]int64{{11, 20}, {10, 21}} {
		_, err := svc.ResolveGrokMediaVideoRequestAccount(ctx, &groupID, "autodl_test", owner[0], owner[1])
		require.Error(t, err)
	}
	bound, err := svc.ResolveGrokMediaVideoRequestAccount(ctx, &groupID, "autodl_test", 10, 20)
	require.NoError(t, err)
	selection, _, err = svc.SelectGrokMediaVideoRequestAccount(ctx, &groupID, GrokMediaVideoRequestSessionHash("autodl_test", 10, 20), bound, "", PlatformOpenAI)
	require.NoError(t, err)
	require.Equal(t, a.ID, selection.Account.ID)
	selection.ReleaseFunc()
	_, _, err = svc.SelectGrokMediaVideoRequestAccount(context.Background(), &groupID, "task", bound, "", PlatformOpenAI)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
}

func TestAutoDLVideoMotionDurationAndBillingTiers(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "motion-*.mp4")
	require.NoError(t, err)
	defer f.Close()
	w := mp4.NewWriter(f)
	_, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMoov()})
	require.NoError(t, err)
	_, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMvhd()})
	require.NoError(t, err)
	_, err = mp4.Marshal(w, &mp4.Mvhd{Timescale: 1000, DurationV0: 20500}, mp4.Context{})
	require.NoError(t, err)
	_, err = w.EndBox()
	require.NoError(t, err)
	_, err = w.EndBox()
	require.NoError(t, err)
	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)
	seconds, err := autoDLMP4Duration(f)
	require.NoError(t, err)
	require.Equal(t, 21, seconds)
	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)
	fixture, err := io.ReadAll(f)
	require.NoError(t, err)
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(fixture))}}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	seconds, err = svc.MeasureAutoDLVideoDuration(context.Background(), "https://cdn.example/motion.mp4")
	require.NoError(t, err)
	require.Equal(t, 21, seconds)
	require.Empty(t, upstream.requests[0].Header.Get("Authorization"))
	_, err = autoDLMP4Duration(strings.NewReader("not-mp4"))
	require.Error(t, err)
	for _, url := range []string{"http://cdn.example/v.mp4", "https://127.0.0.1/private", "https://user:secret@cdn.example/video"} {
		_, err := svc.MeasureAutoDLVideoDuration(context.Background(), url)
		require.Error(t, err)
	}
	require.Equal(t, 21, NormalizeModelVideoBillingDuration("wan2.2animate-v4-motion_retargeting", 21))
	require.Equal(t, 0, NormalizeModelVideoBillingDuration("wan2.2animate-v4-motion_retargeting", 0))
	require.Equal(t, 15, NormalizeModelVideoBillingDuration("grok-imagine-video", 21))
	for _, resolution := range []string{"768p", "1088p", "1440p"} {
		require.Equal(t, resolution, NormalizeVideoBillingResolutionOrDefault(resolution))
	}
}
