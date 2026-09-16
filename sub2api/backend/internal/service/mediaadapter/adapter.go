// Package mediaadapter contains protocol-neutral image/video adapters.
package mediaadapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

type Profile string

const (
	ProfileOpenAICompatible Profile = "openai_compatible"
	ProfileGrokMedia        Profile = "grok_media"
)

type Capability struct {
	Model      string
	Modality   string
	Operations []string
	Profile    Profile
}

var catalog = func() map[string]Capability {
	m := map[string]Capability{}
	images := []string{"gpt-image-1", "gpt-image-1.5", "gpt-image-2", "gemini-3.1-flash-image", "grok-imagine-image-quality", "grok-imagine-image-2.0", "grok-imagine-image", "grok-imagine", "grok-imagine-image-1.5"}
	for _, model := range images {
		profile := ProfileOpenAICompatible
		if strings.HasPrefix(model, "grok-") && model != "grok-imagine-image-1.5" {
			profile = ProfileGrokMedia
		}
		m[model] = Capability{Model: model, Modality: "image", Operations: []string{"generation", "edit"}, Profile: profile}
	}
	for _, model := range []string{"kling-v1", "kling-v1-5", "kling-v1-6", "kling-v2-5-turbo", "kling-v2-6", "kling-v3", "kling-v3-omni", "seedance-2.0", "seedance-2.0-fast", "seedance2.0-900-720p", "sd-2.5-30秒", "minimax-h3-933-图文", "grok-video-1.5", "minimax-h3", "video-v3", "seedance2.5-9图"} {
		m[model] = Capability{Model: model, Modality: "video", Operations: []string{"generation", "image_to_video"}, Profile: ProfileOpenAICompatible}
	}
	for _, model := range []string{"grok-imagine-video", "grok-imagine-video-1.5"} {
		m[model] = Capability{Model: model, Modality: "video", Operations: []string{"generation", "image_to_video"}, Profile: ProfileGrokMedia}
	}
	for model := range autodlWorkflows {
		m[model] = Capability{Model: model, Modality: "video", Operations: []string{"generation", "image_to_video", "audio_to_video"}, Profile: ProfileAutoDL}
	}
	return m
}()

func Lookup(model string) (Capability, bool) {
	name := strings.TrimSpace(model)
	switch strings.ToLower(name) {
	case "minimax_h3", "minimax":
		name = "minimax-h3"
	}
	c, ok := catalog[name]
	return c, ok
}
func Catalog() []Capability {
	out := make([]Capability, 0, len(catalog))
	for _, c := range catalog {
		out = append(out, c)
	}
	return out
}

type Config struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

type Adapter struct{ cfg Config }

func New(cfg Config) *Adapter {
	if cfg.Client == nil {
		cfg.Client = http.DefaultClient
	}
	return &Adapter{cfg: cfg}
}

type ImageRequest struct {
	Model          string   `json:"model"`
	Prompt         string   `json:"prompt"`
	N              int      `json:"n,omitempty"`
	Size           string   `json:"size,omitempty"`
	ResponseFormat string   `json:"response_format,omitempty"`
	Images         []string `json:"images,omitempty"`
}

type ImageItem struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
}
type ImageResponse struct {
	Created int64       `json:"created"`
	Data    []ImageItem `json:"data"`
}

func (a *Adapter) GenerateImage(ctx context.Context, req ImageRequest) (ImageResponse, error) {
	return a.image(ctx, "/v1/images/generations", req)
}
func (a *Adapter) EditImage(ctx context.Context, req ImageRequest) (ImageResponse, error) {
	var out ImageResponse
	if strings.TrimSpace(req.Model) == "" {
		return out, fmt.Errorf("model is required")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", req.Model); err != nil {
		return out, err
	}
	if err := writer.WriteField("prompt", req.Prompt); err != nil {
		return out, err
	}
	for i, source := range req.Images {
		data, contentType, err := decodeDataURL(source)
		if err != nil {
			return out, fmt.Errorf("image %d: %w", i, err)
		}
		part, err := writer.CreateFormFile("image", fmt.Sprintf("reference-%d", i))
		if err != nil {
			return out, err
		}
		if _, err := part.Write(data); err != nil {
			return out, err
		}
		_ = contentType
	}
	if err := writer.Close(); err != nil {
		return out, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint("/v1/images/edits"), &body)
	if err != nil {
		return out, err
	}
	a.auth(request)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := a.cfg.Client.Do(request)
	if err != nil {
		return out, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return out, decodeHTTPError(response)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return out, err
	}
	if err := decodeJSONPayload(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func decodeDataURL(raw string) ([]byte, string, error) {
	parts := strings.SplitN(raw, ",", 2)
	if len(parts) != 2 || !strings.HasPrefix(strings.ToLower(parts[0]), "data:") {
		return nil, "", fmt.Errorf("reference image must be a data URL")
	}
	meta := strings.TrimPrefix(parts[0], "data:")
	contentType := strings.TrimSuffix(strings.SplitN(meta, ";", 2)[0], ";")
	if !strings.Contains(meta, ";base64") {
		return nil, "", fmt.Errorf("reference image must use base64 encoding")
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, "", fmt.Errorf("invalid base64 image: %w", err)
	}
	return data, contentType, nil
}

func (a *Adapter) image(ctx context.Context, path string, req ImageRequest) (ImageResponse, error) {
	var out ImageResponse
	if strings.TrimSpace(req.Model) == "" {
		return out, fmt.Errorf("model is required")
	}
	err := a.doJSON(ctx, http.MethodPost, path, req, &out)
	return out, err
}

type VideoRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Duration    int      `json:"duration,omitempty"`
	AspectRatio string   `json:"aspect_ratio,omitempty"`
	Images      []string `json:"images,omitempty"`
}
type VideoStatus string

const (
	VideoQueued     VideoStatus = "queued"
	VideoInProgress VideoStatus = "in_progress"
	VideoCompleted  VideoStatus = "completed"
	VideoFailed     VideoStatus = "failed"
	VideoCancelled  VideoStatus = "cancelled"
)

type VideoResponse struct {
	ID       string      `json:"id"`
	Status   VideoStatus `json:"status"`
	VideoURL string      `json:"video_url,omitempty"`
	Video    *struct {
		URL string `json:"url"`
	} `json:"video,omitempty"`
	Data *struct {
		VideoURL string `json:"video_url"`
		URL      string `json:"url"`
	} `json:"data,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (a *Adapter) CreateVideo(ctx context.Context, req VideoRequest) (VideoResponse, error) {
	var out VideoResponse
	err := a.doJSON(ctx, http.MethodPost, "/v1/videos", req, &out)
	normalizeVideoResponse(&out)
	return out, err
}
func (a *Adapter) GetVideo(ctx context.Context, id string) (VideoResponse, error) {
	var out VideoResponse
	err := a.doJSON(ctx, http.MethodGet, "/v1/videos/"+url.PathEscape(id), nil, &out)
	normalizeVideoResponse(&out)
	return out, err
}
func (a *Adapter) GetVideoContent(ctx context.Context, id string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoint("/v1/videos/"+url.PathEscape(id)+"/content"), nil)
	if err != nil {
		return nil, "", err
	}
	a.auth(req)
	resp, err := a.cfg.Client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", decodeHTTPError(resp)
	}
	body, err := io.ReadAll(resp.Body)
	return body, resp.Header.Get("Content-Type"), err
}

func (a *Adapter) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.endpoint(path), body)
	if err != nil {
		return err
	}
	a.auth(req)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.cfg.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeHTTPError(resp)
	}
	if out == nil {
		return nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return decodeJSONPayload(data, out)
}

func decodeJSONPayload(data []byte, out any) error {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && len(envelope.Data) > 0 {
		trimmed := bytes.TrimSpace(envelope.Data)
		if len(trimmed) > 0 && trimmed[0] == '{' {
			if err := json.Unmarshal(envelope.Data, out); err != nil {
				return fmt.Errorf("invalid upstream data response: %w", err)
			}
			return nil
		}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("invalid upstream JSON response: %w", err)
	}
	return nil
}

func normalizeVideoResponse(video *VideoResponse) {
	if video == nil {
		return
	}
	if video.VideoURL == "" && video.Video != nil {
		video.VideoURL = video.Video.URL
	}
	if video.VideoURL == "" && video.Data != nil {
		video.VideoURL = video.Data.VideoURL
		if video.VideoURL == "" {
			video.VideoURL = video.Data.URL
		}
	}
	switch strings.ToLower(strings.TrimSpace(string(video.Status))) {
	case "done", "succeeded", "success", "complete":
		video.Status = VideoCompleted
	case "processing", "running", "started", "in_progress":
		video.Status = VideoInProgress
	case "pending", "created", "queued":
		video.Status = VideoQueued
	case "cancelled", "canceled":
		video.Status = VideoCancelled
	case "failed", "failure", "error":
		video.Status = VideoFailed
	}
}

func (a *Adapter) endpoint(path string) string {
	base := strings.TrimRight(strings.TrimSpace(a.cfg.BaseURL), "/")
	path = "/" + strings.TrimLeft(path, "/")
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	return base + path
}
func (a *Adapter) auth(req *http.Request) {
	if key := strings.TrimSpace(a.cfg.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
}

func decodeHTTPError(resp *http.Response) error {
	b, _ := io.ReadAll(resp.Body)
	var payload struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	_ = json.Unmarshal(b, &payload)
	message := strings.TrimSpace(payload.Message)
	if len(payload.Error) > 0 {
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(payload.Error, &e) == nil {
			message = strings.TrimSpace(e.Message)
		}
		if message == "" {
			var s string
			if json.Unmarshal(payload.Error, &s) == nil {
				message = strings.TrimSpace(s)
			}
		}
	}
	if message == "" {
		message = strings.TrimSpace(string(b))
	}
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}
	return fmt.Errorf("upstream %d: %s", resp.StatusCode, message)
}
