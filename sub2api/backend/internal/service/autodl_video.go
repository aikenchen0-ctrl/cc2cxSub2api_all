package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	mp4 "github.com/abema/go-mp4"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const OpenAIEndpointCapabilityAutoDLVideo OpenAIEndpointCapability = "autodl_video"
const autoDLTaskPrefix = "autodl_"

var autoDLTaskIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)
var autoDLWorkflowIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,200}$`)
var autoDLResolutionPattern = regexp.MustCompile(`(?i)^(\d+)p`)

func IsAutoDLVideoModel(model string) bool {
	_, ok := domain.FindAutoDLVideoWorkflow(model)
	return ok
}

func IsAutoDLVideoTask(id string) bool { return strings.HasPrefix(id, autoDLTaskPrefix) }

// AutoDL accounts use the existing OpenAI-compatible API-key account type.
// An explicit protocol marker also supports operator-configured AutoDL mirrors.
func (a *Account) IsAutoDLVideoAccount() bool {
	if a == nil || a.Type != AccountTypeAPIKey {
		return false
	}
	u, err := url.Parse(a.GetCredential("base_url"))
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "autodl.art" || strings.HasSuffix(host, ".autodl.art") || a.GetExtraString("video_protocol") == "autodl"
}

func autoDLVideoURL(account *Account, cfg *config.Config, id string, poll bool) (string, error) {
	if !account.IsAutoDLVideoAccount() {
		return "", fmt.Errorf("AutoDL API-key account required")
	}
	base, err := redactedGrokBaseURLValidator(grokOperatorPolicyValidator(cfg))(account.GetCredential("base_url"))
	if err != nil {
		return "", err
	}
	u, err := url.Parse(base)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid AutoDL base URL")
	}
	path := strings.TrimRight(u.Path, "/")
	if path != "" && path != "/v1" && path != "/api/v1" && path != "/api/v1/comfyui" {
		return "", fmt.Errorf("AutoDL base URL must be an origin or API root")
	}
	if poll {
		id = strings.TrimPrefix(id, autoDLTaskPrefix)
	}
	if poll && !autoDLTaskIDPattern.MatchString(id) || !poll && (!autoDLWorkflowIDPattern.MatchString(id) || strings.Contains(id, "..")) {
		return "", fmt.Errorf("invalid AutoDL workflow/task ID")
	}
	u.Path = "/api/v1/comfyui/comfyui_workflow/"
	if poll {
		u.Path += "result/"
	}
	u.Path += id
	u.RawPath = ""
	return u.String(), nil
}

// Validate and translate the public /v1/videos request before selecting an upstream.
// Only documented workflow inputs are forwarded; references must be public URLs.
func PrepareAutoDLVideoRequest(body []byte) ([]byte, int, string, error) {
	var input map[string]any
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, 0, "", fmt.Errorf("AutoDL 请求必须是 JSON")
	}
	model, _ := input["model"].(string)
	workflow, ok := domain.FindAutoDLVideoWorkflow(model)
	if !ok {
		return nil, 0, "", fmt.Errorf("未支持的 AutoDL 视频工作流")
	}
	rules := workflow.InputRules
	output := map[string]any{}
	for field := range input {
		if strings.HasPrefix(field, "ref_") || field == "first_frame" || field == "last_frame" {
			if _, exists := rules[field]; !exists {
				return nil, 0, "", fmt.Errorf("当前工作流不支持 %s", field)
			}
		}
	}
	for field := range rules {
		if value, exists := input[field]; exists {
			output[field] = value
		}
	}
	for source, target := range map[string]string{"first_frame_url": "first_frame", "last_frame_url": "last_frame", "resolution_name": "resolution"} {
		if value, ok := input[source].(string); ok && strings.TrimSpace(value) != "" {
			output[target] = value
		}
	}
	for source, prefix := range map[string]string{"input_reference[]": "ref_image", "video_reference[]": "ref_video", "audio_reference[]": "ref_audio"} {
		if input[source] == nil {
			continue
		}
		values, ok := input[source].([]any)
		if !ok {
			return nil, 0, "", fmt.Errorf("%s 必须是数组", source)
		}
		for i, value := range values {
			field := prefix + "_" + strconv.Itoa(i)
			if _, exists := rules[prefix]; exists && i == 0 {
				field = prefix
			}
			if _, exists := rules[field]; !exists {
				return nil, 0, "", fmt.Errorf("%s 的参考素材数量超出工作流限制", source)
			}
			output[field] = value
		}
	}
	for _, field := range []string{"duration", "audio_duration"} {
		if _, exists := rules[field]; exists {
			if value := input["seconds"]; value != nil && fmt.Sprint(value) != "" {
				output[field] = value
			}
		}
	}
	// Do not silently discard frames or references for a workflow that cannot use them.
	for field := range output {
		if _, exists := rules[field]; !exists {
			return nil, 0, "", fmt.Errorf("当前工作流不支持 %s", field)
		}
	}
	duration := 0
	resolution := ""
	for field, rule := range rules {
		value := output[field]
		if value == nil || autoDLEmptyString(value) {
			if field != "seed" && !strings.HasPrefix(field, "ref_") && field != "first_frame" && field != "last_frame" && rule.Default != nil {
				value = rule.Default
				output[field] = value
			}
		}
		if value == nil || autoDLEmptyString(value) {
			if rule.Required {
				return nil, 0, "", fmt.Errorf("AutoDL 缺少必填参数：%s", field)
			}
			continue
		}
		switch rule.Type {
		case "integer", "number", "float":
			n, err := strconv.ParseFloat(fmt.Sprint(value), 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || (rule.Type == "integer" && math.Trunc(n) != n) || (rule.Min != nil && n < *rule.Min) || (rule.Max != nil && n > *rule.Max) {
				return nil, 0, "", fmt.Errorf("AutoDL 参数 %s 超出允许范围", field)
			}
			output[field] = n
			if field == "duration" || field == "audio_duration" {
				duration = int(math.Ceil(n))
			}
		default:
			text, ok := value.(string)
			if !ok {
				return nil, 0, "", fmt.Errorf("AutoDL 参数 %s 必须是字符串", field)
			}
			length := utf8.RuneCountInString(strings.TrimSpace(text))
			if (rule.Required && length == 0) || length < rule.MinLength || (rule.MaxLength > 0 && length > rule.MaxLength) {
				return nil, 0, "", fmt.Errorf("AutoDL 参数 %s 长度不符合要求", field)
			}
			if len(rule.Options) > 0 {
				found := false
				for _, option := range rule.Options {
					if text == option.Label {
						found = true
						break
					}
				}
				if !found {
					return nil, 0, "", fmt.Errorf("AutoDL 参数 %s 不是有效选项", field)
				}
			}
			if strings.HasPrefix(field, "ref_") || field == "first_frame" || field == "last_frame" {
				u, err := url.Parse(text)
				if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
					return nil, 0, "", fmt.Errorf("AutoDL 参数 %s 需要可公开访问的 URL", field)
				}
			}
			if field == "resolution" {
				resolution = strings.ToLower(autoDLResolutionPattern.FindString(text))
				// The motion-transfer workflow expresses its 480p tier as pixels.
				if resolution == "" && workflow.UUID == "wan2.2animate-v4-motion_retargeting" {
					resolution = "480p"
				}
			}
		}
	}
	encoded, err := json.Marshal(output)
	return encoded, duration, resolution, err
}

func autoDLEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) == ""
}

func normalizeAutoDLVideoResponse(body []byte, requestID string, create bool) ([]byte, string, bool, error) {
	var payload struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			TaskID  string `json:"task_id"`
			Status  string `json:"status"`
			Message string `json:"message"`
			Results []struct {
				URL  string `json:"url"`
				Type string `json:"type"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", false, fmt.Errorf("AutoDL 返回了无效的任务响应")
	}
	if !strings.EqualFold(payload.Code, "Success") {
		return nil, "", false, fmt.Errorf("AutoDL: %s", firstNonEmpty(payload.Msg, payload.Code, "任务请求失败"))
	}
	id := requestID
	if create {
		if !autoDLTaskIDPattern.MatchString(payload.Data.TaskID) {
			return nil, "", false, fmt.Errorf("AutoDL 未返回有效任务 ID")
		}
		id = autoDLTaskPrefix + payload.Data.TaskID
	} else if payload.Data.TaskID != "" && payload.Data.TaskID != strings.TrimPrefix(requestID, autoDLTaskPrefix) {
		return nil, "", false, fmt.Errorf("AutoDL 返回了不匹配的任务 ID")
	}
	status := "processing"
	message := ""
	videoURL := ""
	switch strings.ToLower(payload.Data.Status) {
	case "completed", "success", "succeeded", "done":
		for _, result := range payload.Data.Results {
			if result.Type == "video" && result.URL != "" {
				videoURL = result.URL
				break
			}
		}
		if videoURL != "" {
			status = "done"
		} else {
			status, message = "failed", "AutoDL 任务完成但没有返回视频地址"
		}
	case "failed", "error", "cancelled", "canceled", "expired":
		status, message = "failed", firstNonEmpty(payload.Data.Message, payload.Msg, "AutoDL 视频生成失败")
	case "pending", "queued", "":
		status = "pending"
	}
	output := map[string]any{"id": id, "request_id": id, "status": status}
	// Immediate completion still goes through owner-bound polling and billing.
	if create && status == "done" {
		status, videoURL = "pending", ""
		output["status"] = status
	}
	if videoURL != "" {
		output["video"] = map[string]string{"url": videoURL}
	}
	if message != "" {
		output["error"] = map[string]string{"message": message}
	}
	encoded, err := json.Marshal(output)
	return encoded, id, status == "done", err
}

func (s *OpenAIGatewayService) forwardAutoDLVideo(ctx context.Context, c *gin.Context, account *Account, endpoint GrokMediaEndpoint, requestID string, body []byte) (*OpenAIForwardResult, error) {
	started := time.Now()
	if endpoint != GrokMediaEndpointVideosGenerations && !endpoint.IsVideoLookupRequest() {
		return nil, fmt.Errorf("unsupported AutoDL endpoint")
	}
	token := strings.TrimSpace(account.GetCredential("api_key"))
	if token == "" {
		return nil, fmt.Errorf("AutoDL account token missing")
	}
	if endpoint == GrokMediaEndpointVideoContent {
		return s.forwardGrokMediaVideoContent(ctx, c, account, token, requestID, started)
	}
	model := ExtractGrokMediaModel("application/json", body)
	upstreamModel := model
	duration, resolution := 0, ""
	var err error
	if endpoint == GrokMediaEndpointVideosGenerations {
		body, duration, resolution, err = PrepareAutoDLVideoRequest(body)
		if err != nil {
			return nil, err
		}
		// Workflow UUIDs are their native endpoint IDs, not chat model aliases.
		// Never apply an OpenAI text model wildcard to a ComfyUI workflow.
	}
	id := requestID
	if !endpoint.IsVideoLookupRequest() {
		id = upstreamModel
	}
	target, err := autoDLVideoURL(account, s.cfg, id, endpoint.IsVideoLookupRequest())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(ctx), endpoint.httpMethod(), target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	account.ApplyHeaderOverrides(req.Header)
	proxyURL := ""
	if account.Proxy != nil && account.ProxyID != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(started).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return s.handleGrokMediaErrorResponse(ctx, resp, c, account, resp.Header.Get("x-request-id"), model)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("AutoDL redirects are not allowed")
	}
	payload, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	normalized, taskID, done, err := normalizeAutoDLVideoResponse(payload, requestID, endpoint == GrokMediaEndpointVideosGenerations)
	if err != nil {
		// A successful HTTP status can still contain an AutoDL business error.
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"type": "upstream_error", "message": err.Error()}})
		return nil, err
	}
	outputURL := gjson.GetBytes(normalized, "video.url").String()
	if endpoint == GrokMediaEndpointVideoStatus {
		normalized = rewriteGrokMediaVideoContentURLs(normalized, taskID, grokMediaContentProxyURL(c, taskID))
	}
	resp.Header.Del("Content-Length")
	resp.Header.Set("Content-Type", "application/json")
	writeGrokMediaResponse(c, resp, normalized, s.responseHeaderFilter)
	result := &OpenAIForwardResult{RequestID: resp.Header.Get("x-request-id"), ResponseID: taskID, Model: model, BillingModel: model, UpstreamModel: upstreamModel, VideoDurationSeconds: duration, VideoResolution: resolution, Duration: time.Since(started), UpstreamHeaders: resp.Header, ResponseHeaders: resp.Header.Clone()}
	if done && endpoint == GrokMediaEndpointVideoStatus {
		result.VideoCount = 1
		result.VideoOutputURL = outputURL
	}
	return result, nil
}

// Measure only an authenticated task's output URL. No AutoDL token is sent to
// the CDN; redirects/private addresses and unbounded downloads are forbidden.
func (s *OpenAIGatewayService) MeasureAutoDLVideoDuration(ctx context.Context, rawURL string) (int, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.User != nil || u.Fragment != "" {
		return 0, fmt.Errorf("invalid AutoDL output URL")
	}
	target, err := urlvalidator.ValidateHTTPSURL(rawURL, urlvalidator.ValidationOptions{})
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(WithHTTPUpstreamPublicHostsOnly(WithHTTPUpstreamRedirectsDisabled(ctx)), 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	resp, err := s.httpUpstream.Do(req, "", 0, 1)
	if err != nil {
		return 0, fmt.Errorf("AutoDL output duration download failed")
	}
	defer resp.Body.Close()
	const maxBytes = 128 << 20
	if resp.StatusCode != http.StatusOK || resp.ContentLength > maxBytes {
		return 0, fmt.Errorf("AutoDL output duration download rejected")
	}
	f, err := os.CreateTemp("", "autodl-duration-*.mp4")
	if err != nil {
		return 0, err
	}
	defer func() { f.Close(); os.Remove(f.Name()) }()
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil || n > maxBytes {
		return 0, fmt.Errorf("AutoDL output duration download incomplete or too large")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	return autoDLMP4Duration(f)
}

func autoDLMP4Duration(r io.ReadSeeker) (int, error) {
	boxes, err := mp4.ExtractBoxWithPayload(r, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeMvhd()})
	if err != nil {
		return 0, fmt.Errorf("AutoDL output is not a readable MP4")
	}
	if len(boxes) != 1 {
		return 0, fmt.Errorf("AutoDL output duration missing")
	}
	header, ok := boxes[0].Payload.(*mp4.Mvhd)
	if !ok || header.Timescale == 0 {
		return 0, fmt.Errorf("AutoDL output timescale invalid")
	}
	seconds := math.Ceil(float64(header.GetDuration()) / float64(header.Timescale))
	if seconds <= 0 || seconds > 86400 {
		return 0, fmt.Errorf("AutoDL output duration invalid")
	}
	return int(seconds), nil
}

func autoDLVideoContentURL(body []byte) (string, error) {
	var status struct {
		Video struct {
			URL string `json:"url"`
		} `json:"video"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return "", err
	}
	u, err := url.Parse(status.Video.URL)
	if err != nil || u.User != nil || u.Fragment != "" || u.Hostname() == "" {
		return "", fmt.Errorf("AutoDL 返回了无效的视频下载地址")
	}
	return urlvalidator.ValidateHTTPSURL(status.Video.URL, urlvalidator.ValidationOptions{})
}
