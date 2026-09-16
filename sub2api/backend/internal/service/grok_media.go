package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service/mediaadapter"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type GrokMediaEndpoint string

const (
	GrokMediaEndpointImagesGenerations GrokMediaEndpoint = "images_generations"
	GrokMediaEndpointImagesEdits       GrokMediaEndpoint = "images_edits"
	GrokMediaEndpointVideosGenerations GrokMediaEndpoint = "videos_generations"
	GrokMediaEndpointVideosEdits       GrokMediaEndpoint = "videos_edits"
	GrokMediaEndpointVideosExtensions  GrokMediaEndpoint = "videos_extensions"
	GrokMediaEndpointVideoStatus       GrokMediaEndpoint = "video_status"
	GrokMediaEndpointVideoContent      GrokMediaEndpoint = "video_content"

	// Official xAI Imagine image-edit limit.
	grokMediaMaxEditSourceImages = 3
)

func (e GrokMediaEndpoint) RequiresRequestBody() bool {
	return !e.IsVideoLookupRequest()
}

func (e GrokMediaEndpoint) IsVideoLookupRequest() bool {
	return e == GrokMediaEndpointVideoStatus || e == GrokMediaEndpointVideoContent
}

func (e GrokMediaEndpoint) IsGenerationRequest() bool {
	switch e {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits, GrokMediaEndpointVideosGenerations, GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions:
		return true
	default:
		return false
	}
}

type GrokMediaRequestInfo struct {
	Model           string
	Prompt          string
	N               int
	Size            string
	SizeTier        string
	AspectRatio     string
	ImageResolution string
	Resolution      string
	// ResolutionRaw preserves the client value for validation before runtime
	// billing normalization applies defaults.
	ResolutionRaw    string
	DurationSeconds int
	InputImageURLs  []string
	MaskImageURL    string
	Uploads         []OpenAIImagesUpload
	MaskUpload      *OpenAIImagesUpload
}

func (r GrokMediaRequestInfo) ModerationBody() []byte {
	payload := map[string]any{}
	if prompt := strings.TrimSpace(r.Prompt); prompt != "" {
		payload["prompt"] = prompt
	}

	images := make([]map[string]string, 0, len(r.InputImageURLs)+len(r.Uploads)+1)
	for _, imageURL := range r.InputImageURLs {
		if imageURL = strings.TrimSpace(imageURL); imageURL != "" {
			images = append(images, map[string]string{"image_url": imageURL})
		}
	}
	for _, upload := range r.Uploads {
		if dataURL := upload.ModerationDataURL(); dataURL != "" {
			images = append(images, map[string]string{"image_url": dataURL})
		}
	}
	if maskURL := strings.TrimSpace(r.MaskImageURL); maskURL != "" {
		images = append(images, map[string]string{"image_url": maskURL})
	}
	if r.MaskUpload != nil {
		if dataURL := r.MaskUpload.ModerationDataURL(); dataURL != "" {
			images = append(images, map[string]string{"image_url": dataURL})
		}
	}
	if len(images) > 0 {
		payload["images"] = images
	}
	if len(payload) == 0 {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return body
}

func (e GrokMediaEndpoint) httpMethod() string {
	if e.IsVideoLookupRequest() {
		return http.MethodGet
	}
	return http.MethodPost
}

func ExtractGrokMediaModel(contentType string, body []byte) string {
	return ParseGrokMediaRequest(contentType, body).Model
}

func ParseGrokMediaRequest(contentType string, body []byte) GrokMediaRequestInfo {
	info := GrokMediaRequestInfo{N: 1}
	if gjson.ValidBytes(body) {
		parseGrokMediaJSONRequest(body, &info)
	} else {
		parseGrokMediaMultipartRequest(contentType, body, &info)
	}
	info.Model = strings.TrimSpace(info.Model)
	info.Prompt = strings.TrimSpace(info.Prompt)
	info.Size = strings.TrimSpace(info.Size)
	info.SizeTier = NormalizeImageBillingTierOrDefault(info.Size)
	info.AspectRatio = strings.TrimSpace(info.AspectRatio)
	info.ImageResolution = grokImagineImageResolution(info.ImageResolution)
	info.ResolutionRaw = strings.TrimSpace(info.Resolution)
	info.Resolution = NormalizeVideoBillingResolutionOrDefault(info.Resolution)
	info.DurationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(info.DurationSeconds)
	if info.N <= 0 {
		info.N = 1
	}
	return info
}

func parseGrokMediaJSONRequest(body []byte, info *GrokMediaRequestInfo) {
	if info == nil {
		return
	}
	info.Model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	info.Prompt = strings.TrimSpace(gjson.GetBytes(body, "prompt").String())
	info.Size = strings.TrimSpace(gjson.GetBytes(body, "size").String())
	info.AspectRatio = strings.TrimSpace(gjson.GetBytes(body, "aspect_ratio").String())
	if info.AspectRatio == "" {
		info.AspectRatio = strings.TrimSpace(gjson.GetBytes(body, "ratio").String())
	}
	assignGrokMediaResolution(strings.TrimSpace(gjson.GetBytes(body, "resolution").String()), info)
	if duration := gjson.GetBytes(body, "duration"); duration.Exists() && duration.Type == gjson.Number {
		info.DurationSeconds = int(duration.Int())
	}
	if info.DurationSeconds <= 0 {
		seconds := gjson.GetBytes(body, "seconds")
		if seconds.Exists() {
			if seconds.Type == gjson.Number {
				info.DurationSeconds = int(seconds.Int())
			} else {
				var parsed int
				if _, scanErr := fmt.Sscanf(strings.TrimSpace(seconds.String()), "%d", &parsed); scanErr == nil {
					info.DurationSeconds = parsed
				}
			}
		}
	}
	if n := gjson.GetBytes(body, "n"); n.Exists() && n.Type == gjson.Number {
		info.N = int(n.Int())
	}
	appendJSONImageURLs := func(value gjson.Result) {
		if !value.Exists() {
			return
		}
		switch {
		case value.IsArray():
			for _, item := range value.Array() {
				if imageURL := extractGrokMediaImageURL(item); imageURL != "" {
					info.InputImageURLs = append(info.InputImageURLs, imageURL)
				}
			}
		default:
			if imageURL := extractGrokMediaImageURL(value); imageURL != "" {
				info.InputImageURLs = append(info.InputImageURLs, imageURL)
			}
		}
	}
	appendJSONImageURLs(gjson.GetBytes(body, "image"))
	appendJSONImageURLs(gjson.GetBytes(body, "images"))
	appendJSONImageURLs(gjson.GetBytes(body, "reference_images"))
	appendJSONImageURLs(gjson.GetBytes(body, "input_reference"))
	info.MaskImageURL = extractGrokMediaImageURL(gjson.GetBytes(body, "mask"))
}

func extractGrokMediaImageURL(value gjson.Result) string {
	if !value.Exists() {
		return ""
	}
	if value.Type == gjson.String {
		return strings.TrimSpace(value.String())
	}
	if imageURL := strings.TrimSpace(value.Get("url").String()); imageURL != "" {
		return imageURL
	}
	if nested := value.Get("image_url"); nested.Exists() {
		if nested.Type == gjson.String {
			return strings.TrimSpace(nested.String())
		}
		if imageURL := strings.TrimSpace(nested.Get("url").String()); imageURL != "" {
			return imageURL
		}
	}
	return strings.TrimSpace(value.Get("image_url").String())
}

func grokMediaImageObject(imageURL string) map[string]string {
	return map[string]string{"url": imageURL, "type": "image_url"}
}

func parseGrokMediaMultipartRequest(contentType string, body []byte, info *GrokMediaRequestInfo) {
	if info == nil {
		return
	}
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return
		}
		if err != nil {
			return
		}
		name := strings.TrimSpace(part.FormName())
		if name == "" {
			_ = part.Close()
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, openAIImageMaxUploadPartSize))
		_ = part.Close()
		if err != nil {
			return
		}
		fileName := strings.TrimSpace(part.FileName())
		partContentType := strings.TrimSpace(part.Header.Get("Content-Type"))
		if fileName != "" {
			upload := OpenAIImagesUpload{
				FieldName:   name,
				FileName:    fileName,
				ContentType: partContentType,
				Data:        data,
			}
			if name == "mask" {
				info.MaskUpload = &upload
				continue
			}
			if name == "image" || strings.HasPrefix(name, "image[") {
				info.Uploads = append(info.Uploads, upload)
			}
			continue
		}

		value := strings.TrimSpace(string(data))
		switch name {
		case "model":
			info.Model = value
		case "prompt":
			info.Prompt = value
		case "size":
			info.Size = value
		case "aspect_ratio":
			info.AspectRatio = value
		case "resolution":
			assignGrokMediaResolution(value, info)
		case "duration":
			if duration, err := strconv.Atoi(value); err == nil {
				info.DurationSeconds = duration
			}
		case "n":
			if n, err := strconv.Atoi(value); err == nil {
				info.N = n
			}
		case "image", "image_url":
			if value != "" {
				info.InputImageURLs = append(info.InputImageURLs, value)
			}
		case "mask", "mask_image_url":
			info.MaskImageURL = value
		}
	}
}

func GrokMediaVideoRequestSessionHash(requestID string, userID, apiKeyID int64) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	ownerSeed := fmt.Sprintf("%d:%d:%s", userID, apiKeyID, requestID)
	return "grok-video:" + DeriveSessionHashFromSeed(ownerSeed)
}

func (s *OpenAIGatewayService) BindGrokMediaVideoRequestAccount(
	ctx context.Context,
	groupID *int64,
	requestID string,
	userID, apiKeyID, accountID int64,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("grok video request binding cache is unavailable")
	}
	sessionHash := GrokMediaVideoRequestSessionHash(requestID, userID, apiKeyID)
	cacheKey := s.openAISessionCacheKey(sessionHash)
	if cacheKey == "" || accountID <= 0 {
		return fmt.Errorf("grok video request binding is invalid")
	}
	// Video jobs may complete well after WS sticky TTL (default 1h). Bind at least
	// as long as the pending-billing snapshot so late status/content polls resolve.
	ttl := grokVideoPendingBillingTTL(s.cfg)
	if s.cfg != nil && s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds > 0 {
		if sticky := time.Duration(s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds) * time.Second; sticky > ttl {
			ttl = sticky
		}
	}
	return s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), cacheKey, accountID, ttl)
}

func (s *OpenAIGatewayService) ResolveGrokMediaVideoRequestAccount(
	ctx context.Context,
	groupID *int64,
	requestID string,
	userID, apiKeyID int64,
) (int64, error) {
	if s == nil || s.cache == nil {
		return 0, fmt.Errorf("grok video request binding cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(GrokMediaVideoRequestSessionHash(requestID, userID, apiKeyID))
	if cacheKey == "" {
		return 0, fmt.Errorf("grok video request binding is invalid")
	}
	return s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
}

// GrokVideoPendingBilling is the create-time snapshot used when status polling
// first observes a completed video URL. Status may omit model/duration; we fall
// back to this snapshot, then defaults.
type GrokVideoPendingBilling struct {
	Model                string `json:"model"`
	BillingModel         string `json:"billing_model,omitempty"`
	UpstreamModel        string `json:"upstream_model,omitempty"`
	VideoResolution      string `json:"video_resolution,omitempty"`
	VideoDurationSeconds int    `json:"video_duration_seconds,omitempty"`
	OriginalModel        string `json:"original_model,omitempty"`
	// CreatedAt is when the gateway accepted the async create (RFC3339Nano UTC).
	// duration_ms for deferred billing is measured from this instant until the
	// first official done+video.url observation (status poll or content download),
	// not the latency of that single discovery request alone.
	CreatedAt string `json:"created_at,omitempty"`
}

// GrokVideoPendingCreatedAtNow formats a create-accept timestamp for pending billing.
func GrokVideoPendingCreatedAtNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// GrokVideoE2EDuration returns wall time from create accept to discovery of completion.
// Returns 0 when CreatedAt is missing or unparseable (caller keeps poll-only Duration).
func GrokVideoE2EDuration(createdAt string, discoveredAt time.Time) time.Duration {
	createdAt = strings.TrimSpace(createdAt)
	if createdAt == "" {
		return 0
	}
	if discoveredAt.IsZero() {
		discoveredAt = time.Now()
	}
	var created time.Time
	var err error
	if created, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		if created, err = time.Parse(time.RFC3339, createdAt); err != nil {
			return 0
		}
	}
	if created.IsZero() {
		return 0
	}
	d := discoveredAt.Sub(created)
	if d < 0 {
		return 0
	}
	return d
}

func grokVideoPendingBillingKey(requestID string, userID, apiKeyID int64) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d:%s", userID, apiKeyID, requestID)
}

func grokVideoPendingBillingTTL(cfg *config.Config) time.Duration {
	// Video generation can take several minutes; keep create-time pricing for a day.
	_ = cfg
	return 24 * time.Hour
}

func grokVideoBilledClaimTTL(cfg *config.Config) time.Duration {
	_ = cfg
	return 48 * time.Hour
}

// StoreGrokVideoPendingBilling persists create-time billing params for deferred status billing.
func (s *OpenAIGatewayService) StoreGrokVideoPendingBilling(
	ctx context.Context,
	requestID string,
	userID, apiKeyID int64,
	pending GrokVideoPendingBilling,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("grok video pending billing cache is unavailable")
	}
	key := grokVideoPendingBillingKey(requestID, userID, apiKeyID)
	if key == "" {
		return fmt.Errorf("grok video pending billing key is invalid")
	}
	pending.Model = strings.TrimSpace(pending.Model)
	pending.BillingModel = strings.TrimSpace(pending.BillingModel)
	pending.UpstreamModel = strings.TrimSpace(pending.UpstreamModel)
	pending.OriginalModel = strings.TrimSpace(pending.OriginalModel)
	if pending.VideoResolution != "" {
		pending.VideoResolution = NormalizeVideoBillingResolutionOrDefault(pending.VideoResolution)
	}
	if pending.VideoDurationSeconds > 0 {
		pending.VideoDurationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(pending.VideoDurationSeconds)
	}
	// Always stamp create-accept time when missing so deferred duration_ms is E2E.
	if strings.TrimSpace(pending.CreatedAt) == "" {
		pending.CreatedAt = GrokVideoPendingCreatedAtNow()
	} else {
		pending.CreatedAt = strings.TrimSpace(pending.CreatedAt)
	}
	payload, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	return s.cache.SetGrokVideoPendingBilling(ctx, key, payload, grokVideoPendingBillingTTL(s.cfg))
}

// LoadGrokVideoPendingBilling returns the create-time snapshot (may be nil on miss).
func (s *OpenAIGatewayService) LoadGrokVideoPendingBilling(
	ctx context.Context,
	requestID string,
	userID, apiKeyID int64,
) (*GrokVideoPendingBilling, error) {
	if s == nil || s.cache == nil {
		return nil, fmt.Errorf("grok video pending billing cache is unavailable")
	}
	key := grokVideoPendingBillingKey(requestID, userID, apiKeyID)
	if key == "" {
		return nil, fmt.Errorf("grok video pending billing key is invalid")
	}
	payload, err := s.cache.GetGrokVideoPendingBilling(ctx, key)
	if err != nil || len(payload) == 0 {
		return nil, err
	}
	var pending GrokVideoPendingBilling
	if err := json.Unmarshal(payload, &pending); err != nil {
		return nil, err
	}
	return &pending, nil
}

// ClaimGrokVideoBilling returns true once for a completed video request so status
// polls do not double-bill. Fail-closed: claim errors are treated as already billed.
func (s *OpenAIGatewayService) ClaimGrokVideoBilling(
	ctx context.Context,
	requestID string,
	userID, apiKeyID int64,
) (bool, error) {
	if s == nil || s.cache == nil {
		return false, fmt.Errorf("grok video billing claim cache is unavailable")
	}
	key := grokVideoPendingBillingKey(requestID, userID, apiKeyID)
	if key == "" {
		return false, fmt.Errorf("grok video billing claim key is invalid")
	}
	return s.cache.ClaimGrokVideoBilled(ctx, key, grokVideoBilledClaimTTL(s.cfg))
}

// ReleaseGrokVideoBilling clears a claim after a failed durable RecordUsage so a
// later status/content poll can retry billing.
func (s *OpenAIGatewayService) ReleaseGrokVideoBilling(
	ctx context.Context,
	requestID string,
	userID, apiKeyID int64,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("grok video billing claim cache is unavailable")
	}
	key := grokVideoPendingBillingKey(requestID, userID, apiKeyID)
	if key == "" {
		return fmt.Errorf("grok video billing claim key is invalid")
	}
	return s.cache.ReleaseGrokVideoBilled(ctx, key)
}

// StableGrokVideoBillingRequestID is the durable usage_logs / dedup key for one
// async video task (not the per-poll gateway request id).
func StableGrokVideoBillingRequestID(taskRequestID string) string {
	taskRequestID = strings.TrimSpace(taskRequestID)
	if taskRequestID == "" {
		return ""
	}
	if strings.HasPrefix(taskRequestID, "grok-video:") {
		return taskRequestID
	}
	return "grok-video:" + taskRequestID
}

// Official xAI async video status success shape (docs.x.ai Video Generation):
//
//	{"status":"done","model":"grok-imagine-video-1.5","video":{"url":"...","duration":8,"respect_moderation":true}}
//
// Request may include resolution ("480p"|"720p"|"1080p"); completed status does not
// document a resolution field — bill resolution from the create-time request snapshot.

// IsGrokVideoStatusBillable matches official success: status == "done" AND non-empty video.url.
// pending / expired / failed, or done without a video URL, are not billable.
func IsGrokVideoStatusBillable(statusBody []byte) bool {
	if len(statusBody) == 0 || !gjson.ValidBytes(statusBody) {
		return false
	}
	if !isOfficialGrokVideoStatusDone(statusBody) {
		return false
	}
	return strings.TrimSpace(gjson.GetBytes(statusBody, "video.url").String()) != ""
}

func isOfficialGrokVideoStatusDone(statusBody []byte) bool {
	// Official enum: pending | done | expired | failed.
	return strings.EqualFold(strings.TrimSpace(gjson.GetBytes(statusBody, "status").String()), "done")
}

// ExtractGrokVideoBillingFromStatusBody builds usage units from an official done status.
// Field priority (official docs):
//   - duration: video.duration (seconds)
//   - model: top-level model
//   - resolution: not in status response → create-time pending snapshot → default 480p
func ExtractGrokVideoBillingFromStatusBody(statusBody []byte, pending *GrokVideoPendingBilling, requestID string) *OpenAIForwardResult {
	if !IsGrokVideoStatusBillable(statusBody) {
		return nil
	}
	model := ""
	billingModel := ""
	upstreamModel := ""
	resolution := ""
	durationSeconds := 0

	if gjson.ValidBytes(statusBody) {
		// Official: top-level model.
		model = strings.TrimSpace(gjson.GetBytes(statusBody, "model").String())
		// Official: video.duration (number of seconds).
		if v := gjson.GetBytes(statusBody, "video.duration"); v.Exists() && v.Type == gjson.Number {
			durationSeconds = int(v.Int())
			if durationSeconds == 0 && v.Float() > 0 {
				// Sub-second values are unexpected for this API; still accept truncated int path above.
				durationSeconds = int(v.Float())
			}
		}
	}
	if pending != nil {
		if model == "" {
			model = firstNonEmpty(pending.BillingModel, pending.Model, pending.OriginalModel)
		}
		if billingModel == "" {
			billingModel = firstNonEmpty(pending.BillingModel, pending.Model)
		}
		if upstreamModel == "" {
			upstreamModel = pending.UpstreamModel
		}
		// Official status has no resolution — always take create request when available.
		resolution = pending.VideoResolution
		if durationSeconds <= 0 {
			durationSeconds = pending.VideoDurationSeconds
		}
	}
	if model == "" {
		// Official default video model family when status omits model.
		model = "grok-imagine-video"
	}
	if billingModel == "" {
		billingModel = model
	}
	// Resolution is request-only per docs; empty → handler applies official default 480p.
	if resolution != "" {
		resolution = NormalizeVideoBillingResolutionOrDefault(resolution)
	}
	if durationSeconds > 0 {
		durationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(durationSeconds)
	}
	responseID := extractGrokMediaVideoRequestID(statusBody)
	if responseID == "" {
		responseID = strings.TrimSpace(requestID)
	}
	return &OpenAIForwardResult{
		ResponseID:           responseID,
		Model:                model,
		BillingModel:         billingModel,
		UpstreamModel:        upstreamModel,
		VideoCount:           1,
		VideoResolution:      resolution,
		VideoDurationSeconds: durationSeconds,
	}
}

func (s *OpenAIGatewayService) ForwardGrokMedia(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint GrokMediaEndpoint,
	requestID string,
	body []byte,
	contentType string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil {
		return nil, fmt.Errorf("grok account is required")
	}
	// Non-Grok video models use the OpenAI-compatible async JSON protocol
	// (NewToken/VEO/MiniMax/video-*). Keep the xAI path isolated to Grok models.
	if endpoint == GrokMediaEndpointVideosGenerations || endpoint == GrokMediaEndpointVideosEdits || endpoint == GrokMediaEndpointVideosExtensions || endpoint == GrokMediaEndpointVideoStatus || endpoint == GrokMediaEndpointVideoContent {
		model := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "model").String()))
		if (endpoint.IsVideoLookupRequest() && account.Platform != PlatformGrok) || (model != "" && !strings.Contains(model, "grok")) || account.Platform != PlatformGrok {
			return s.forwardCompatibleVideo(ctx, c, account, endpoint, requestID, body, contentType, startTime)
		}
	}
	if account.Platform != PlatformGrok && !(account.Platform == PlatformOpenAI && endpoint.IsVideoLookupRequest() || account.Platform == PlatformOpenAI && endpoint == GrokMediaEndpointVideosGenerations) {
		return nil, fmt.Errorf("account platform %s is not supported for grok media", account.Platform)
	}

	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
	}
	if endpoint == GrokMediaEndpointVideoContent {
		return s.forwardGrokMediaVideoContent(ctx, c, account, token, requestID, startTime)
	}
	targetURL, err := buildGrokMediaURL(account, s.cfg, endpoint, requestID)
	if err != nil {
		return nil, err
	}

	body, contentType, err = prepareGrokMediaForwardBody(endpoint, body, contentType)
	if err != nil {
		return nil, err
	}
	body, contentType, err = normalizeGrokMediaForwardBody(endpoint, body, contentType)
	if err != nil {
		return nil, err
	}
	requestInfo := ParseGrokMediaRequest(contentType, body)
	upstreamModel := requestInfo.Model
	if endpoint.RequiresRequestBody() && gjson.ValidBytes(body) {
		if mappedModel := strings.TrimSpace(account.GetMappedModel(requestInfo.Model)); mappedModel != "" {
			upstreamModel = mappedModel
		}
		if upstreamModel != requestInfo.Model {
			body, err = sjson.SetBytes(body, "model", upstreamModel)
			if err != nil {
				return nil, fmt.Errorf("rewrite grok media account mapped model: %w", err)
			}
		}
	}
	body, contentType, err = sanitizeGrokMediaForwardBody(endpoint, body, contentType)
	if err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if endpoint.RequiresRequestBody() {
		bodyReader = bytes.NewReader(body)
	}
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, endpoint.httpMethod(), targetURL, bodyReader)
	if err != nil {
		return nil, err
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	upstreamReq.Header.Set("Accept", "application/json")
	if account.IsGrokOAuth() && isGrokCLIProxyTarget(targetURL) {
		applyGrokCLIHeaders(upstreamReq.Header)
	}
	if endpoint.RequiresRequestBody() {
		contentType = strings.TrimSpace(contentType)
		if contentType == "" {
			contentType = "application/json"
		}
		upstreamReq.Header.Set("Content-Type", contentType)
	}
	// 账号级请求头覆写最后应用，配置值优先于内置默认头。
	account.ApplyHeaderOverrides(upstreamReq.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	requestIDHeader := firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id"))
	requestModel := requestInfo.Model
	if resp.StatusCode >= 400 {
		return s.handleGrokMediaErrorResponse(ctx, resp, c, account, requestIDHeader, requestModel)
	}

	s.updateGrokUsageFromResponse(withGrokTeamRateLimitModel(ctx, requestModel), account, resp.Header, resp.StatusCode)
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if endpoint == GrokMediaEndpointImagesGenerations || endpoint == GrokMediaEndpointImagesEdits {
		if countOpenAIResponseImageOutputsFromJSONBytes(respBody) <= 0 {
			setOpsUpstreamError(c, http.StatusBadGateway, "xAI upstream returned no image output", truncateString(string(respBody), 512))
			return nil, &UpstreamFailoverError{
				StatusCode:      http.StatusBadGateway,
				ResponseBody:    respBody,
				ResponseHeaders: resp.Header.Clone(),
			}
		}
	}
	if endpoint == GrokMediaEndpointVideoStatus {
		respBody = rewriteGrokMediaVideoContentURLs(
			respBody,
			requestID,
			grokMediaContentProxyURL(c, requestID),
		)
	}
	writeGrokMediaResponse(c, resp, respBody, s.responseHeaderFilter)
	usage := grokMediaUsageFromResponse(endpoint, requestInfo, respBody)
	resultModel := requestModel
	resultBillingModel := requestModel
	if endpoint == GrokMediaEndpointVideoStatus {
		// Status has no request body model; use upstream status fields when billable.
		if m := strings.TrimSpace(usage.Model); m != "" {
			resultModel = m
		}
		if m := strings.TrimSpace(usage.BillingModel); m != "" {
			resultBillingModel = m
		}
	}
	return &OpenAIForwardResult{
		RequestID:            requestIDHeader,
		ResponseID:           usage.ResponseID,
		Usage:                usage.Usage,
		Model:                resultModel,
		BillingModel:         resultBillingModel,
		UpstreamModel:        upstreamModel,
		ResponseHeaders:      resp.Header.Clone(),
		Duration:             time.Since(startTime),
		ImageCount:           usage.ImageCount,
		ImageSize:            usage.ImageSize,
		ImageInputSize:       usage.ImageInputSize,
		ImageOutputSizes:     usage.ImageOutputSizes,
		VideoCount:           usage.VideoCount,
		VideoResolution:      usage.VideoResolution,
		VideoDurationSeconds: usage.VideoDurationSeconds,
	}, nil
}

func (s *OpenAIGatewayService) forwardCompatibleVideo(ctx context.Context, c *gin.Context, account *Account, endpoint GrokMediaEndpoint, requestID string, body []byte, contentType string, startTime time.Time) (*OpenAIForwardResult, error) {
	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(strings.TrimSpace(account.GetOpenAIBaseURL()), "/")
	base = strings.TrimSuffix(base, "/v1")
	if base == "" {
		return nil, fmt.Errorf("video account base URL is empty")
	}
	if isAutoDLVideoAccount(account, body) {
		return s.forwardAutoDLVideo(ctx, c, account, token, endpoint, requestID, body, contentType, startTime)
	}
	if endpoint == GrokMediaEndpointVideosGenerations && isVividVideoAccount(account) {
		body, err = prepareVividVideoBody(body)
		if err != nil {
			return nil, err
		}
	}
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	path := "/videos"
	method := http.MethodPost
	if endpoint.IsVideoLookupRequest() {
		method = http.MethodGet
		path += "/" + url.PathEscape(strings.TrimSpace(requestID))
		if endpoint == GrokMediaEndpointVideoContent {
			path += "/content"
		}
	}
	targetURL := base + path
	var reader io.Reader
	if method == http.MethodPost {
		reader = bytes.NewReader(body)
	}
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	req, err := http.NewRequestWithContext(upstreamCtx, method, targetURL, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		if strings.TrimSpace(contentType) == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}
	account.ApplyHeaderOverrides(req.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return s.handleGrokMediaErrorResponse(ctx, resp, c, account, resp.Header.Get("x-request-id"), gjson.GetBytes(body, "model").String())
	}
	if endpoint == GrokMediaEndpointVideoContent && isVividVideoAccount(account) {
		if err := writeGrokMediaContentResponse(c, resp); err != nil {
			return nil, err
		}
		return &OpenAIForwardResult{
			RequestID:       resp.Header.Get("x-request-id"),
			ResponseHeaders: resp.Header.Clone(),
			ResponseID:      strings.TrimSpace(requestID),
			VideoCount:      1,
			Duration:        time.Since(startTime),
		}, nil
	}
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if endpoint == GrokMediaEndpointVideoStatus {
		respBody = rewriteGrokMediaVideoContentURLs(respBody, requestID, grokMediaContentProxyURL(c, requestID))
	}
	writeGrokMediaResponse(c, resp, respBody, s.responseHeaderFilter)
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" {
		model = strings.TrimSpace(gjson.GetBytes(respBody, "model").String())
	}
	id := extractGrokMediaVideoRequestID(respBody)
	if id == "" {
		id = strings.TrimSpace(requestID)
	}
	status := strings.ToLower(strings.TrimSpace(firstNonEmptyString(gjson.GetBytes(respBody, "status").String(), gjson.GetBytes(respBody, "state").String(), gjson.GetBytes(respBody, "task_status").String())))
	videoURL := firstNonEmptyString(gjson.GetBytes(respBody, "video_url").String(), gjson.GetBytes(respBody, "url").String(), gjson.GetBytes(respBody, "data.video_url").String(), gjson.GetBytes(respBody, "data.url").String(), gjson.GetBytes(respBody, "metadata.result_urls.0").String(), gjson.GetBytes(respBody, "data.metadata.result_urls.0").String())
	result := &OpenAIForwardResult{RequestID: resp.Header.Get("x-request-id"), ResponseID: id, Model: model, BillingModel: model, UpstreamModel: model, ResponseHeaders: resp.Header.Clone(), Duration: time.Since(startTime)}
	if endpoint == GrokMediaEndpointVideosGenerations {
		result.VideoResolution = NormalizeVideoBillingResolutionOrDefault(gjson.GetBytes(body, "resolution").String())
		result.VideoDurationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(int(gjson.GetBytes(body, "duration").Int()))
	}
	if status == "completed" || videoURL != "" {
		result.VideoCount = 1
	}
	return result, nil
}

func isAutoDLVideoAccount(account *Account, body []byte) bool {
	if account == nil {
		return false
	}
	return isAutoDLHost(account.GetOpenAIBaseURL())
}

// AutoDLVideoModelSupported reports whether an AutoDL account can serve the
// requested video model. Non-AutoDL accounts are deliberately treated as
// supported because their own OpenAI-compatible endpoint handles model names.
func AutoDLVideoModelSupported(account *Account, model string) bool {
	if account == nil || !isAutoDLHost(account.GetOpenAIBaseURL()) {
		return true
	}
	_, ok := mediaadapter.AutoDLWorkflowForModel(model)
	return ok
}

func isAutoDLHost(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	return host == "autodl.art" || strings.HasSuffix(host, ".autodl.art")
}

// IsAutoDLBaseURL reports whether a configured endpoint is an AutoDL host.
// Callers outside this package should use this parser-backed check rather than
// substring matching, which can classify autodl.art.example.com incorrectly.
func IsAutoDLBaseURL(raw string) bool {
	return isAutoDLHost(raw)
}

// ShouldPreferAutoDLVideoModel reports whether the model has an AutoDL
// workflow. The handler uses the canonical workflow name as the scheduler
// model constraint so accounts with unrelated explicit model mappings are
// excluded before forwarding.
func ShouldPreferAutoDLVideoModel(model string) bool {
	_, ok := mediaadapter.AutoDLWorkflowForModel(model)
	return ok
}

// OpenAICompatibleVideoModelSupported validates video models against the
// shared media catalog first. Known video models (for example video-v3) are
// safe to pass through even when an account has no explicit mapping; unknown
// models still require an explicit mapping to avoid selecting text/image-only
// accounts by accident.
func OpenAICompatibleVideoModelSupported(account *Account, model string) bool {
	if account == nil || account.Platform != PlatformOpenAI {
		return false
	}
	if isAutoDLVideoAccount(account, nil) {
		return AutoDLVideoModelSupported(account, model)
	}
	if isVividVideoAccount(account) {
		capability, ok := mediaadapter.Lookup(strings.TrimSpace(model))
		return ok && capability.Modality == "video" && capability.Profile == mediaadapter.ProfileOpenAICompatible
	}
	if capability, ok := mediaadapter.Lookup(strings.TrimSpace(model)); ok && capability.Modality == "video" && capability.Profile == mediaadapter.ProfileOpenAICompatible {
		return true
	}
	if len(account.GetModelMapping()) == 0 {
		return false
	}
	return account.IsModelSupported(model)
}

// IsOpenAICompatibleVideoModel identifies video models that should be routed
// through ordinary OpenAI-compatible accounts, even when their name contains
// "grok" (for example Vivid's grok-video-1.5).
func IsOpenAICompatibleVideoModel(model string) bool {
	capability, ok := mediaadapter.Lookup(strings.TrimSpace(model))
	return ok && capability.Modality == "video" && capability.Profile == mediaadapter.ProfileOpenAICompatible
}

func isVividVideoAccount(account *Account) bool {
	if account == nil || account.Platform != PlatformOpenAI {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(account.GetOpenAIBaseURL())), "aigc.easysu.cn")
}

// IsVividVideoAccount exposes the provider check to the gateway scheduler so
// video requests can prefer Vivid while retaining failover to other providers.
func IsVividVideoAccount(account *Account) bool { return isVividVideoAccount(account) }

func prepareVividVideoBody(body []byte) ([]byte, error) {
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("video request body must be valid JSON")
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	// MiniMax H3 expects input_reference. Canvas may already provide Vivid's
	// pure-base64 reference_images form, which the generic media parser cannot
	// identify as a URL; normalize that array explicitly before forwarding.
	if strings.EqualFold(model, "minimax-h3-933-图文") {
		refs := gjson.GetBytes(body, "reference_images")
		if refs.Exists() && refs.IsArray() {
			var err error
			body, err = sjson.SetBytes(body, "input_reference", refs.Raw)
			if err != nil {
				return nil, fmt.Errorf("normalize MiniMax input references: %w", err)
			}
			body, err = sjson.DeleteBytes(body, "reference_images")
			if err != nil {
				return nil, fmt.Errorf("remove MiniMax reference_images: %w", err)
			}
		}
	}
	// Vivid expects reference media as input_reference: [string]. Canvas and
	// OpenAI-compatible clients may send image/images/reference_images using
	// image objects, so normalize those forms before forwarding upstream.
	mediaInfo := ParseGrokMediaRequest("application/json", body)
	if len(mediaInfo.InputImageURLs) > 0 {
		var err error
		references := mediaInfo.InputImageURLs
		referenceField := "input_reference"
		if strings.EqualFold(model, "seedance2.5-9图") {
			referenceField = "reference_images"
			references = make([]string, 0, len(mediaInfo.InputImageURLs))
			for _, reference := range mediaInfo.InputImageURLs {
				references = append(references, stripImageDataURLPrefix(reference))
			}
		}
		body, err = sjson.SetBytes(body, referenceField, references)
		if err != nil {
			return nil, fmt.Errorf("normalize Vivid input references: %w", err)
		}
		for _, field := range []string{"image", "images", "reference_images", "input_reference"} {
			if field == referenceField {
				continue
			}
			body, err = sjson.DeleteBytes(body, field)
			if err != nil {
				return nil, fmt.Errorf("remove Vivid reference alias %s: %w", field, err)
			}
		}
	}
	if !gjson.GetBytes(body, "seconds").Exists() {
		if duration := gjson.GetBytes(body, "duration"); duration.Exists() {
			value := duration.Value()
			if duration.Type == gjson.String {
				raw := strings.TrimSpace(duration.String())
				raw = strings.TrimSuffix(strings.ToLower(raw), "s")
				if seconds, parseErr := strconv.Atoi(strings.TrimSpace(raw)); parseErr == nil {
					value = seconds
				}
			}
			var err error
			body, err = sjson.SetBytes(body, "seconds", value)
			if err != nil {
				return nil, err
			}
		}
	}
	if model == "seedance-2.0" {
		var err error
		body, err = sjson.SetBytes(body, "model", "seedance2.0-900-720p")
		if err != nil {
			return nil, err
		}
	}
	if strings.EqualFold(model, "video-v3") {
		// Vivid's video-v3 endpoint rejects OpenAI compatibility aliases.
		for _, field := range []string{"seconds", "size", "aspect_ratio", "images"} {
			var err error
			body, err = sjson.DeleteBytes(body, field)
			if err != nil {
				return nil, err
			}
		}
		if reference := gjson.GetBytes(body, "input_reference"); reference.Exists() && reference.Type != gjson.JSON {
			var err error
			body, err = sjson.SetBytes(body, "input_reference", []string{reference.String()})
			if err != nil {
				return nil, err
			}
		}
	}
	if strings.HasPrefix(strings.ToLower(model), "minimax-") || strings.HasPrefix(strings.ToLower(model), "minimax_") {
		var err error
		body, err = sjson.DeleteBytes(body, "ratio")
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

func stripImageDataURLPrefix(value string) string {
	trimmed := strings.TrimSpace(value)
	if comma := strings.Index(trimmed, ","); strings.HasPrefix(strings.ToLower(trimmed), "data:") && comma >= 0 {
		return trimmed[comma+1:]
	}
	return trimmed
}

// AutoDLVideoSchedulingModel returns the canonical scheduler constraint for
// an AutoDL model, or the trimmed request model for other providers.
func AutoDLVideoSchedulingModel(model string) string {
	if workflow, ok := mediaadapter.AutoDLWorkflowForModel(model); ok {
		return workflow
	}
	return strings.TrimSpace(model)
}

func (s *OpenAIGatewayService) forwardAutoDLVideo(ctx context.Context, c *gin.Context, account *Account, token string, endpoint GrokMediaEndpoint, requestID string, body []byte, contentType string, startTime time.Time) (*OpenAIForwardResult, error) {
	base := normalizeAutoDLBaseURL(account.GetOpenAIBaseURL())
	if base == "" {
		return nil, fmt.Errorf("AutoDL base URL is empty")
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	workflow, ok := mediaadapter.AutoDLWorkflowForModel(model)
	if endpoint.IsVideoLookupRequest() {
		workflow = ""
	}
	if endpoint == GrokMediaEndpointVideosGenerations && !ok {
		return nil, fmt.Errorf("unsupported AutoDL video model: %s", model)
	}
	path := "/api/v1/comfyui/comfyui_workflow/" + workflow
	method := http.MethodPost
	var reader io.Reader
	if endpoint.IsVideoLookupRequest() {
		method = http.MethodGet
		path = "/api/v1/comfyui/comfyui_workflow/result/" + url.PathEscape(strings.TrimSpace(requestID))
	} else {
		var err error
		body, err = prepareAutoDLVideoBody(body, workflow)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		if strings.TrimSpace(contentType) == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}
	account.ApplyHeaderOverrides(req.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return s.handleGrokMediaErrorResponse(ctx, resp, c, account, resp.Header.Get("x-request-id"), model)
	}
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	normalized, err := mediaadapter.NormalizeAutoDLResponse(respBody)
	if err != nil {
		return nil, err
	}
	if !normalized.Success {
		statusCode := http.StatusBadGateway
		if strings.EqualFold(normalized.Code, "RequestParameterIsWrong") || strings.EqualFold(normalized.Code, "InvalidParameter") {
			statusCode = http.StatusBadRequest
		}
		message := normalized.Message
		if message == "" {
			message = normalized.Code
		}
		if message == "" {
			message = "AutoDL workflow request failed"
		}
		MarkResponseCommitted(c)
		writeGrokMediaErrorResponse(c, statusCode, grokMediaErrorType(statusCode), message)
		return nil, fmt.Errorf("AutoDL workflow error: %s", message)
	}
	if normalized.ID == "" {
		normalized.ID = requestID
	}
	result := &OpenAIForwardResult{RequestID: resp.Header.Get("x-request-id"), ResponseID: normalized.ID, Model: model, BillingModel: model, UpstreamModel: model, ResponseHeaders: resp.Header.Clone(), Duration: time.Since(startTime)}
	if normalized.Status == mediaadapter.VideoCompleted || normalized.VideoURL != "" {
		result.VideoCount = 1
	}
	if endpoint == GrokMediaEndpointVideoContent && normalized.VideoURL != "" {
		// AutoDL returns a short-lived result URL from its status endpoint. The
		// Sub2API content endpoint must fetch that URL and stream the MP4, so
		// clients never need direct AutoDL credentials or result URLs.
		_ = resp.Body.Close()
		if err := validateAutoDLVideoResultURL(normalized.VideoURL, base); err != nil {
			return nil, err
		}
		contentReq, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(ctx), http.MethodGet, normalized.VideoURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build AutoDL video content request: %w", err)
		}
		contentReq.Header.Set("Accept", "*/*")
		// Preserve byte-range playback requests so browsers and media
		// players do not receive the full MP4 for every seek operation.
		if c != nil && c.Request != nil {
			if rangeValue := strings.TrimSpace(c.GetHeader("Range")); rangeValue != "" {
				contentReq.Header.Set("Range", rangeValue)
			}
		}
		contentResp, err := s.httpUpstream.Do(contentReq, proxyURL, account.ID, account.Concurrency)
		if err != nil {
			return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
		}
		defer contentResp.Body.Close()
		if contentResp.StatusCode >= 400 {
			return s.handleGrokMediaErrorResponse(ctx, contentResp, c, account, contentResp.Header.Get("x-request-id"), model)
		}
		if err := writeGrokMediaContentResponse(c, contentResp); err != nil {
			return nil, err
		}
		result.ResponseHeaders = contentResp.Header.Clone()
		return result, nil
	}
	if endpoint == GrokMediaEndpointVideoStatus && normalized.VideoURL != "" {
		respBody = rewriteAutoDLVideoResultURL(respBody, normalized.ID, grokMediaContentProxyURL(c, normalized.ID))
	}
	if endpoint == GrokMediaEndpointVideosGenerations {
		result.VideoDurationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(int(gjson.GetBytes(body, "duration").Int()))
		result.VideoResolution = NormalizeVideoBillingResolutionOrDefault(gjson.GetBytes(body, "resolution").String())
	}
	// Status/content responses are written only after normalization so status
	// polling can expose the stable Sub2API content URL instead of AutoDL's URL.
	writeGrokMediaResponse(c, resp, respBody, s.responseHeaderFilter)
	return result, nil
}

// validateAutoDLVideoResultURL prevents a workflow response from turning the
// content proxy into an arbitrary URL fetcher. AutoDL result links are HTTPS
// URLs served by autodl.art or one of its CDN subdomains.
func validateAutoDLVideoResultURL(raw, baseURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("invalid AutoDL video result URL")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	base, baseErr := url.Parse(strings.TrimSpace(baseURL))
	baseHost := ""
	if baseErr == nil {
		baseHost = strings.ToLower(strings.TrimSuffix(base.Hostname(), "."))
	}
	if host != "autodl.art" && !strings.HasSuffix(host, ".autodl.art") && (baseHost == "" || host != baseHost) {
		return fmt.Errorf("AutoDL video result host is not allowlisted")
	}
	return nil
}

func normalizeAutoDLBaseURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	base = strings.TrimSuffix(base, "/api/v1")
	base = strings.TrimSuffix(base, "/api")
	base = strings.TrimSuffix(base, "/v1")
	return strings.TrimRight(base, "/")
}

func prepareAutoDLVideoBody(body []byte, workflows ...string) ([]byte, error) {
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("AutoDL video request body must be JSON")
	}
	workflow := ""
	if len(workflows) > 0 {
		workflow = strings.TrimSpace(workflows[0])
	}
	updated, err := sjson.DeleteBytes(body, "model")
	if err != nil {
		return nil, fmt.Errorf("remove AutoDL model field: %w", err)
	}
	if duration := gjson.GetBytes(updated, "duration"); duration.Exists() {
		// AutoDL's workflow API declares duration as an integer. Canvas also
		// sends the OpenAI-compatible `seconds` string, which is removed below.
		updated, err = sjson.SetBytes(updated, "duration", int(duration.Int()))
		if err != nil {
			return nil, fmt.Errorf("normalize AutoDL duration: %w", err)
		}
	}
	resolution := autoDLResolutionValue(
		gjson.GetBytes(updated, "resolution").String(),
		gjson.GetBytes(updated, "aspect_ratio").String(),
		gjson.GetBytes(updated, "size").String(),
		workflow,
	)
	if resolution != "" {
		updated, err = sjson.SetBytes(updated, "resolution", resolution)
		if err != nil {
			return nil, fmt.Errorf("normalize AutoDL resolution: %w", err)
		}
	}
	// These are OpenAI/Vivid compatibility fields and are not part of the
	// AutoDL workflow input schema. Unknown fields make AutoDL reject the task.
	for _, field := range []string{"aspect_ratio", "seconds", "size", "ratio", "generateAudio", "watermark"} {
		updated, err = sjson.DeleteBytes(updated, field)
		if err != nil {
			return nil, fmt.Errorf("remove AutoDL compatibility field %s: %w", field, err)
		}
	}
	for i, image := range gjson.GetBytes(updated, "images").Array() {
		updated, err = sjson.SetBytes(updated, fmt.Sprintf("ref_image_%d", i), autoDLMediaValue(image))
		if err != nil {
			return nil, fmt.Errorf("map AutoDL image field: %w", err)
		}
	}
	updated, err = sjson.DeleteBytes(updated, "images")
	if err != nil {
		return nil, fmt.Errorf("remove AutoDL images field: %w", err)
	}
	for i, audio := range gjson.GetBytes(updated, "audios").Array() {
		updated, err = sjson.SetBytes(updated, fmt.Sprintf("ref_audio_%d", i), autoDLMediaValue(audio))
		if err != nil {
			return nil, fmt.Errorf("map AutoDL audio field: %w", err)
		}
	}
	updated, err = sjson.DeleteBytes(updated, "audios")
	if err != nil {
		return nil, fmt.Errorf("remove AutoDL audios field: %w", err)
	}
	if workflow == "minimax_h3_b99_002" {
		images := gjson.GetBytes(body, "images").Array()
		if len(images) >= 1 {
			updated, err = sjson.SetBytes(updated, "first_frame", autoDLMediaValue(images[0]))
			if err != nil {
				return nil, fmt.Errorf("map AutoDL first frame: %w", err)
			}
		}
		if len(images) >= 2 {
			updated, err = sjson.SetBytes(updated, "last_frame", autoDLMediaValue(images[1]))
			if err != nil {
				return nil, fmt.Errorf("map AutoDL last frame: %w", err)
			}
		}
		for i := range images {
			field := fmt.Sprintf("ref_image_%d", i)
			updated, err = sjson.DeleteBytes(updated, field)
			if err != nil {
				return nil, fmt.Errorf("remove AutoDL frame alias %s: %w", field, err)
			}
		}
	}
	if workflow == "wan2.2animate-v4-motion_retargeting" {
		// Motion retargeting accepts only seed/ref_image/ref_video/resolution.
		for _, field := range []string{"prompt", "duration", "ref_image_0", "ref_video_0"} {
			updated, err = sjson.DeleteBytes(updated, field)
			if err != nil {
				return nil, fmt.Errorf("remove AutoDL motion field %s: %w", field, err)
			}
		}
	}
	return updated, nil
}

// autoDLResolutionValue converts the compact resolution values emitted by
// Canvas into the exact enum labels exposed by AutoDL workflow metadata.
// Already-expanded labels are preserved so workflow-specific dimensions are
// not lost.
func autoDLResolutionValue(raw, aspectRatio, size, workflow string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lowerRaw := strings.ToLower(raw)
	quality := lowerRaw
	if p := strings.IndexByte(lowerRaw, 'p'); p >= 0 {
		quality = lowerRaw[:p]
	}
	explicitPortrait := strings.Contains(raw, "竖")
	explicitLandscape := strings.Contains(raw, "横")
	switch quality {
	case "low":
		quality = "480"
	case "auto", "medium", "high", "720":
		quality = "768"
	case "1080":
		quality = "1080"
	case "1440":
		quality = "1440"
	}
	if quality == "" {
		return raw
	}
	portrait := explicitPortrait || (!explicitLandscape && strings.Contains(strings.ToLower(strings.TrimSpace(aspectRatio)), "9:16"))
	if !portrait {
		parts := strings.Split(strings.TrimSpace(size), "x")
		if len(parts) == 2 {
			width, widthErr := strconv.Atoi(strings.TrimSpace(parts[0]))
			height, heightErr := strconv.Atoi(strings.TrimSpace(parts[1]))
			if widthErr == nil && heightErr == nil {
				portrait = height > width
			}
		}
	}
	direction := "横"
	if portrait {
		direction = "竖"
	}
	// AutoDL's newer Z workflows expose dimensions in the enum label.
	if strings.HasPrefix(workflow, "minimax_h3_z090") {
		dimensions := map[string][2]string{
			"480":  {"480", "864"},
			"768":  {"768", "1344"},
			"1088": {"1088", "1920"},
			"1440": {"1440", "2560"},
		}
		if d, ok := dimensions[quality]; ok {
			if !portrait {
				d[0], d[1] = d[1], d[0]
			}
			return fmt.Sprintf("%sp%s(%s*%s)", quality, direction, d[0], d[1])
		}
	}
	// The ZM and legacy H3 workflows use labels without dimensions.
	if strings.HasPrefix(workflow, "minimax_h3_zm_") {
		if quality == "1080" || quality == "1440" {
			quality = "768"
		}
		return quality + "p" + direction
	}
	if strings.HasPrefix(workflow, "minimax_h3_b99_") {
		return "736p" + direction
	}
	if strings.Contains(workflow, "image_audio_to_video") || strings.Contains(workflow, "lightx2v") {
		if quality == "720" {
			quality = "768"
		}
		return quality + "p" + direction
	}
	if strings.Contains(raw, "(") {
		return raw
	}
	return raw
}

func rewriteAutoDLVideoResultURL(body []byte, requestID, proxyURL string) []byte {
	if len(body) == 0 || strings.TrimSpace(requestID) == "" || strings.TrimSpace(proxyURL) == "" || !gjson.ValidBytes(body) {
		return body
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return body
	}
	changed := false
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				if rawURL, ok := child.(string); ok && shouldRewriteAutoDLVideoURL(key, rawURL) {
					typed[key] = proxyURL
					changed = true
					continue
				}
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(root)
	if !changed {
		return body
	}
	rewritten, err := json.Marshal(root)
	if err != nil {
		return body
	}
	return rewritten
}

func shouldRewriteAutoDLVideoURL(key, rawURL string) bool {
	if strings.TrimSpace(rawURL) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "video_url", "result_url":
		return true
	case "url":
		return strings.Contains(strings.ToLower(rawURL), ".mp4")
	default:
		return false
	}
}

func autoDLMediaValue(value gjson.Result) string {
	if value.Type == gjson.String {
		return strings.TrimSpace(value.String())
	}
	return strings.TrimSpace(firstNonEmptyString(value.Get("url").String(), value.Get("image_url").String(), value.Get("audio_url").String()))
}

func (s *OpenAIGatewayService) forwardGrokMediaVideoContent(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token, requestID string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	statusURL, err := buildGrokMediaURL(account, s.cfg, GrokMediaEndpointVideoStatus, requestID)
	if err != nil {
		return nil, err
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	statusReq, err := http.NewRequestWithContext(
		WithHTTPUpstreamRedirectsDisabled(upstreamCtx),
		http.MethodGet,
		statusURL,
		nil,
	)
	if err != nil {
		return nil, err
	}
	statusReq.Header.Set("Authorization", "Bearer "+token)
	statusReq.Header.Set("Accept", "application/json")
	if account.IsGrokOAuth() && isGrokCLIProxyTarget(statusURL) {
		applyGrokCLIHeaders(statusReq.Header)
	}
	account.ApplyHeaderOverrides(statusReq.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	statusResp, err := s.httpUpstream.Do(statusReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	statusRequestID := firstNonEmpty(statusResp.Header.Get("x-request-id"), statusResp.Header.Get("xai-request-id"))
	if statusResp.StatusCode >= 300 {
		defer func() { _ = statusResp.Body.Close() }()
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		if statusResp.StatusCode < 400 {
			return nil, fmt.Errorf("grok media status redirect is not allowed")
		}
		return s.handleGrokMediaErrorResponse(ctx, statusResp, c, account, statusRequestID, "")
	}
	statusBody, err := ReadUpstreamResponseBody(statusResp.Body, s.cfg, c, openAITooLargeError)
	_ = statusResp.Body.Close()
	if err != nil {
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		return nil, err
	}

	contentURL, err := grokMediaSignedVideoContentURL(statusBody, requestID)
	if err != nil {
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		return nil, err
	}
	signedContent := contentURL != ""
	if !signedContent {
		contentURL, err = buildGrokMediaURL(account, s.cfg, GrokMediaEndpointVideoContent, requestID)
		if err != nil {
			SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
			return nil, err
		}
	}

	contentReq, err := http.NewRequestWithContext(
		WithHTTPUpstreamRedirectsDisabled(upstreamCtx),
		http.MethodGet,
		contentURL,
		nil,
	)
	if err != nil {
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		return nil, err
	}
	contentReq.Header.Set("Accept", "*/*")
	if c != nil {
		if rangeHeader := strings.TrimSpace(c.GetHeader("Range")); rangeHeader != "" {
			contentReq.Header.Set("Range", rangeHeader)
		}
	}
	if !signedContent {
		contentReq.Header.Set("Authorization", "Bearer "+token)
		if account.IsGrokOAuth() && isGrokCLIProxyTarget(contentURL) {
			applyGrokCLIHeaders(contentReq.Header)
		}
		account.ApplyHeaderOverrides(contentReq.Header)
	}

	contentResp, err := s.httpUpstream.Do(contentReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = contentResp.Body.Close() }()
	contentRequestID := firstNonEmpty(contentResp.Header.Get("x-request-id"), contentResp.Header.Get("xai-request-id"), statusRequestID)
	if contentResp.StatusCode >= 300 && contentResp.StatusCode < 400 {
		return nil, fmt.Errorf("grok media signed content redirect is not allowed")
	}
	if contentResp.StatusCode >= 400 && contentResp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		return s.handleGrokMediaErrorResponse(ctx, contentResp, c, account, contentRequestID, "")
	}

	s.updateGrokUsageFromResponse(withGrokTeamRateLimitModel(ctx, ""), account, contentResp.Header, contentResp.StatusCode)
	if err := writeGrokMediaContentResponse(c, contentResp); err != nil {
		return nil, err
	}
	// Content download is an alternate completion observation: when status body is
	// official done+video.url, attach billable units so the handler can claim once
	// (same path as status polling). Pending snapshot is merged in the handler.
	result := &OpenAIForwardResult{
		RequestID:       contentRequestID,
		ResponseHeaders: contentResp.Header.Clone(),
		Duration:        time.Since(startTime),
	}
	if billed := ExtractGrokVideoBillingFromStatusBody(statusBody, nil, requestID); billed != nil {
		result.ResponseID = firstNonEmpty(billed.ResponseID, strings.TrimSpace(requestID))
		result.Model = billed.Model
		result.BillingModel = billed.BillingModel
		result.UpstreamModel = billed.UpstreamModel
		result.VideoCount = billed.VideoCount
		result.VideoResolution = billed.VideoResolution
		result.VideoDurationSeconds = billed.VideoDurationSeconds
	}
	return result, nil
}

func grokMediaSignedVideoContentURL(body []byte, requestID string) (string, error) {
	rawURL := strings.TrimSpace(gjson.GetBytes(body, "video.url").String())
	if rawURL == "" {
		return "", nil
	}
	// An upstream Sub2API rewrites protected content URLs to its own proxy
	// endpoint. Treat that as an authenticated relay path, not as a signed URL;
	// the caller will rebuild it against the configured account base URL and
	// attach the upstream API key.
	if isGrokMediaVideoContentURL(rawURL, requestID) {
		return "", nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") ||
		!strings.EqualFold(parsed.Hostname(), "vidgen.x.ai") ||
		(parsed.Port() != "" && parsed.Port() != "443") || parsed.User != nil {
		return "", fmt.Errorf("grok media status returned an unsupported video content URL")
	}
	return parsed.String(), nil
}

func isGrokCLIProxyTarget(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && strings.EqualFold(parsed.Hostname(), "cli-chat-proxy.grok.com")
}

func prepareGrokMediaForwardBody(endpoint GrokMediaEndpoint, body []byte, contentType string) ([]byte, string, error) {
	if endpoint != GrokMediaEndpointImagesEdits {
		return body, contentType, nil
	}
	if gjson.ValidBytes(body) {
		out, err := normalizeGrokMediaJSONImageRefs(body)
		return out, contentType, err
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return body, contentType, nil
	}

	info := ParseGrokMediaRequest(contentType, body)
	payload := make(map[string]any)
	if info.Model != "" {
		payload["model"] = info.Model
	}
	if info.Prompt != "" {
		payload["prompt"] = info.Prompt
	}
	if info.N > 1 {
		payload["n"] = info.N
	}
	if info.Size != "" {
		payload["size"] = info.Size
	}
	if info.ImageResolution != "" {
		payload["resolution"] = info.ImageResolution
	}
	if info.AspectRatio != "" {
		payload["aspect_ratio"] = info.AspectRatio
	}

	images := make([]map[string]string, 0, len(info.InputImageURLs)+len(info.Uploads))
	for _, imageURL := range info.InputImageURLs {
		if imageURL = strings.TrimSpace(imageURL); imageURL != "" {
			images = append(images, grokMediaImageObject(imageURL))
		}
	}
	for _, upload := range info.Uploads {
		dataURL, err := openAIImageUploadToDataURL(upload)
		if err != nil {
			return nil, "", err
		}
		images = append(images, grokMediaImageObject(dataURL))
	}
	if len(images) > grokMediaMaxEditSourceImages {
		return nil, "", fmt.Errorf("a maximum of %d source images is supported for image edits", grokMediaMaxEditSourceImages)
	}
	if len(images) > 0 {
		payload["image"] = images[0]
		if len(images) > 1 {
			payload["images"] = images
		}
	}

	maskImageURL := strings.TrimSpace(info.MaskImageURL)
	if info.MaskUpload != nil {
		dataURL, err := openAIImageUploadToDataURL(*info.MaskUpload)
		if err != nil {
			return nil, "", err
		}
		maskImageURL = dataURL
	}
	if maskImageURL != "" {
		payload["mask"] = grokMediaImageObject(maskImageURL)
	}

	out, err := marshalOpenAIUpstreamJSON(payload)
	if err != nil {
		return nil, "", err
	}
	return out, "application/json", nil
}

func normalizeGrokMediaJSONImageRefs(body []byte) ([]byte, error) {
	info := ParseGrokMediaRequest("application/json", body)
	if len(info.InputImageURLs) > grokMediaMaxEditSourceImages {
		return nil, fmt.Errorf("a maximum of %d source images is supported for image edits", grokMediaMaxEditSourceImages)
	}
	out := body
	var err error
	for _, field := range []string{"image", "images", "mask"} {
		out, err = rewriteGrokMediaJSONImageField(out, field)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func rewriteGrokMediaJSONImageField(body []byte, path string) ([]byte, error) {
	value := gjson.GetBytes(body, path)
	if !value.Exists() {
		return body, nil
	}
	if value.IsArray() {
		rewritten := make([]map[string]string, 0, len(value.Array()))
		for _, item := range value.Array() {
			imageURL := extractGrokMediaImageURL(item)
			if imageURL == "" {
				return body, nil
			}
			rewritten = append(rewritten, grokMediaImageObject(imageURL))
		}
		out, err := sjson.SetBytes(body, path, rewritten)
		if err != nil {
			return nil, fmt.Errorf("rewrite grok media %s: %w", path, err)
		}
		return out, nil
	}
	imageURL := extractGrokMediaImageURL(value)
	if imageURL == "" {
		return body, nil
	}
	out, err := sjson.SetBytes(body, path, grokMediaImageObject(imageURL))
	if err != nil {
		return nil, fmt.Errorf("rewrite grok media %s: %w", path, err)
	}
	return out, nil
}

func normalizeGrokMediaForwardBody(endpoint GrokMediaEndpoint, body []byte, contentType string) ([]byte, string, error) {
	if !endpoint.RequiresRequestBody() || !gjson.ValidBytes(body) {
		return body, contentType, nil
	}
	var imageFields []string
	switch endpoint {
	case GrokMediaEndpointImagesEdits:
		imageFields = []string{"image", "images", "mask"}
	case GrokMediaEndpointVideosGenerations:
		imageFields = []string{"image", "images", "reference_images"}
	}
	var err error
	body, err = canonicalizeGrokMediaImageURLFields(body, imageFields...)
	if err != nil {
		return nil, "", err
	}
	info := ParseGrokMediaRequest(contentType, body)
	upstreamModel := NormalizeGrokMediaModelForEndpoint(endpoint, info.Model, info.HasInputImage())
	if upstreamModel == "" || upstreamModel == info.Model {
		return body, contentType, nil
	}
	out, err := sjson.SetBytes(body, "model", upstreamModel)
	if err != nil {
		return nil, "", fmt.Errorf("rewrite grok media model: %w", err)
	}
	return out, contentType, nil
}

func canonicalizeGrokMediaImageURLFields(body []byte, fields ...string) ([]byte, error) {
	out := body
	for _, field := range fields {
		value := gjson.GetBytes(out, field)
		if !value.Exists() {
			continue
		}
		if value.IsArray() {
			for index := range value.Array() {
				var err error
				out, err = canonicalizeGrokMediaImageURLObject(out, fmt.Sprintf("%s.%d", field, index))
				if err != nil {
					return nil, err
				}
			}
			continue
		}
		var err error
		out, err = canonicalizeGrokMediaImageURLObject(out, field)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func canonicalizeGrokMediaImageURLObject(body []byte, path string) ([]byte, error) {
	legacyPath := path + ".image_url"
	legacy := gjson.GetBytes(body, legacyPath)
	if !legacy.Exists() {
		return body, nil
	}

	out := body
	if strings.TrimSpace(gjson.GetBytes(out, path+".url").String()) == "" {
		var err error
		out, err = sjson.SetBytes(out, path+".url", legacy.Value())
		if err != nil {
			return nil, fmt.Errorf("normalize grok media image url: %w", err)
		}
	}
	out, err := sjson.DeleteBytes(out, legacyPath)
	if err != nil {
		return nil, fmt.Errorf("remove legacy grok media image url: %w", err)
	}
	return out, nil
}

func sanitizeGrokMediaForwardBody(endpoint GrokMediaEndpoint, body []byte, contentType string) ([]byte, string, error) {
	if !endpoint.RequiresRequestBody() || !gjson.ValidBytes(body) {
		return body, contentType, nil
	}
	switch endpoint {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits:
		out, err := applyGrokImagineImageGeometry(body)
		if err != nil {
			return nil, "", fmt.Errorf("sanitize grok media size: %w", err)
		}
		return out, contentType, nil
	case GrokMediaEndpointVideosGenerations:
		model := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "model").String()))
		if strings.HasPrefix(model, "grok-imagine-video") {
			// Canvas also sends Vivid's seconds/size compatibility fields. The
			// native xAI endpoint rejects unknown fields, so remove them here.
			out, err := sjson.DeleteBytes(body, "seconds")
			if err != nil {
				return nil, "", fmt.Errorf("remove xAI seconds field: %w", err)
			}
			out, err = sjson.DeleteBytes(out, "size")
			if err != nil {
				return nil, "", fmt.Errorf("remove xAI size field: %w", err)
			}
			return out, contentType, nil
		}
		if model != "" && !strings.HasPrefix(model, "grok-imagine-video") {
			if value := gjson.GetBytes(body, "resolution_name"); value.Exists() {
				out, err := sjson.SetBytes(body, "resolution", value.String())
				if err != nil {
					return nil, "", fmt.Errorf("normalize video resolution: %w", err)
				}
				out, err = sjson.DeleteBytes(out, "resolution_name")
				if err != nil {
					return nil, "", fmt.Errorf("remove unsupported video resolution_name: %w", err)
				}
				return out, contentType, nil
			}
		}
		return body, contentType, nil
	default:
		return body, contentType, nil
	}
}

func (r GrokMediaRequestInfo) HasInputImage() bool {
	return len(r.InputImageURLs) > 0 || len(r.Uploads) > 0
}

// NormalizeGrokMediaModelForEndpoint resolves the built-in upstream model alias
// for a media endpoint before account-level model mapping and scheduling.
func NormalizeGrokMediaModelForEndpoint(endpoint GrokMediaEndpoint, model string, hasInputImage bool) string {
	model = strings.TrimSpace(model)
	switch endpoint {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits:
		if model == "grok-imagine" {
			return "grok-imagine-image-quality"
		}
	case GrokMediaEndpointVideosGenerations:
		// xAI's 1.5 model is image-to-video only. Keep the requested model
		// unchanged when the image is missing so the upstream returns its
		// documented invalid-argument response instead of silently switching
		// models and pricing.
		_ = hasInputImage
	}
	return model
}

type grokMediaUsageMetadata struct {
	ResponseID           string
	Usage                OpenAIUsage
	Model                string
	BillingModel         string
	ImageCount           int
	ImageSize            string
	ImageInputSize       string
	ImageOutputSizes     []string
	VideoCount           int
	VideoResolution      string
	VideoDurationSeconds int
}

func grokMediaUsageFromResponse(endpoint GrokMediaEndpoint, requestInfo GrokMediaRequestInfo, responseBody []byte) grokMediaUsageMetadata {
	usage, _ := extractOpenAIUsageFromJSONBytes(responseBody)
	meta := grokMediaUsageMetadata{Usage: usage}
	switch endpoint {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits:
		meta.ImageCount = countOpenAIResponseImageOutputsFromJSONBytes(responseBody)
		meta.ImageSize = requestInfo.SizeTier
		meta.ImageInputSize = requestInfo.Size
		meta.ImageOutputSizes = collectOpenAIResponseImageOutputSizesFromJSONBytes(responseBody)
	case GrokMediaEndpointVideosGenerations, GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions:
		// Async video: capture request_id + create-time pricing params only.
		// Billable VideoCount is set later when status polling observes video.url.
		meta.ResponseID = extractGrokMediaVideoRequestID(responseBody)
		meta.VideoResolution = requestInfo.Resolution
		meta.VideoDurationSeconds = requestInfo.DurationSeconds
	case GrokMediaEndpointVideoStatus:
		// Prefer status-body URL success + upstream duration/resolution when present.
		if IsGrokVideoStatusBillable(responseBody) {
			// provisional units; handler merges with pending snapshot before RecordUsage.
			if billed := ExtractGrokVideoBillingFromStatusBody(responseBody, nil, ""); billed != nil {
				meta.ResponseID = billed.ResponseID
				meta.Model = billed.Model
				meta.BillingModel = billed.BillingModel
				meta.VideoCount = billed.VideoCount
				meta.VideoResolution = billed.VideoResolution
				meta.VideoDurationSeconds = billed.VideoDurationSeconds
			}
		}
	}
	return meta
}

func extractGrokMediaVideoRequestID(body []byte) string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ""
	}
	for _, path := range []string{"request_id", "id", "data.request_id", "data.id", "video.request_id", "video.id", "task_id", "data.task_id", "video.task_id"} {
		if id := strings.TrimSpace(gjson.GetBytes(body, path).String()); id != "" {
			return id
		}
	}
	return ""
}

func (s *OpenAIGatewayService) handleGrokMediaErrorResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestIDHeader string,
	requestedModel string,
) (*OpenAIForwardResult, error) {
	body := s.readUpstreamErrorBody(resp)
	// Reconcile readiness before configurable passthrough branches can return;
	// otherwise a Grok 429 can remain schedulable.
	s.handleGrokAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if upstreamMsg == "" {
		upstreamMsg = fmt.Sprintf("xAI upstream returned status %d", resp.StatusCode)
	}

	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	if isGrokContentPolicyRejection(resp.StatusCode, body) {
		clientMsg := grokContentPolicyClientMessage(body)
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestIDHeader,
			Kind:               "http_error",
			Message:            clientMsg,
			Detail:             upstreamDetail,
		})
		MarkResponseCommitted(c)
		writeGrokMediaErrorResponse(c, http.StatusForbidden, "invalid_request_error", clientMsg)
		return nil, fmt.Errorf("grok content policy rejection: %s", clientMsg)
	}

	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		account.Platform,
		resp.StatusCode,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		MarkResponseCommitted(c)
		writeGrokMediaErrorResponse(c, status, errType, errMsg)
		return nil, fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", resp.StatusCode, upstreamMsg)
	}

	if !account.ShouldHandleErrorCode(resp.StatusCode) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestIDHeader,
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})
		MarkResponseCommitted(c)
		writeGrokMediaErrorResponse(c, http.StatusInternalServerError, "upstream_error", "Upstream gateway error")
		return nil, fmt.Errorf("upstream error: %d (not in custom error codes) message=%s", resp.StatusCode, upstreamMsg)
	}

	kind := "http_error"
	if s.shouldFailoverGrokUpstreamError(resp.StatusCode, body) {
		kind = "failover"
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  requestIDHeader,
		Kind:               kind,
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})
	if kind == "failover" {
		retryable, retryDelay, retryDeadline, retryMax := grokSameAccountRetryMetadata(account, resp.StatusCode, body)
		return nil, &UpstreamFailoverError{
			StatusCode:               resp.StatusCode,
			ResponseBody:             body,
			ResponseHeaders:          resp.Header.Clone(),
			RetryableOnSameAccount:   retryable,
			RequestScopedTransient:   retryable && resp.StatusCode == http.StatusTooManyRequests,
			SameAccountRetryDelay:    retryDelay,
			SameAccountRetryDeadline: retryDeadline,
			SameAccountRetryMax:      retryMax,
		}
	}

	MarkResponseCommitted(c)
	writeGrokMediaErrorResponse(c, resp.StatusCode, grokMediaErrorType(resp.StatusCode), upstreamMsg)
	return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
}

func grokMediaErrorType(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	default:
		return "upstream_error"
	}
}

func writeGrokMediaErrorResponse(c *gin.Context, statusCode int, errType, message string) {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    strings.TrimSpace(errType),
			"message": strings.TrimSpace(message),
		},
	})
}

func writeGrokMediaResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
	if c == nil || resp == nil {
		return
	}
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, filter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
}

func writeGrokMediaContentResponse(c *gin.Context, resp *http.Response) error {
	if c == nil || resp == nil || resp.Body == nil {
		return fmt.Errorf("grok media content response is incomplete")
	}

	for _, name := range []string{
		"Content-Type",
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"Content-Disposition",
	} {
		if value := strings.TrimSpace(resp.Header.Get(name)); value != "" {
			c.Header(name, value)
		}
	}
	if strings.TrimSpace(c.Writer.Header().Get("Content-Length")) == "" && resp.ContentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	if strings.TrimSpace(c.Writer.Header().Get("Content-Type")) == "" {
		c.Header("Content-Type", "application/octet-stream")
	}
	c.Status(resp.StatusCode)
	MarkResponseCommitted(c)
	_, err := io.Copy(c.Writer, resp.Body)
	return err
}

func rewriteGrokMediaVideoContentURLs(body []byte, requestID, proxyURL string) []byte {
	if len(body) == 0 || strings.TrimSpace(requestID) == "" || strings.TrimSpace(proxyURL) == "" || !gjson.ValidBytes(body) {
		return body
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return body
	}
	changed := rewriteGrokMediaKnownVideoURL(&value, proxyURL)
	if rewriteGrokMediaVideoContentURLValue(&value, requestID, proxyURL) {
		changed = true
	}
	if !changed {
		return body
	}
	rewritten, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return rewritten
}

func rewriteGrokMediaKnownVideoURL(value *any, proxyURL string) bool {
	if value == nil {
		return false
	}
	root, ok := (*value).(map[string]any)
	if !ok {
		return false
	}
	video, ok := root["video"].(map[string]any)
	if !ok {
		return false
	}
	rawURL, ok := video["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return false
	}
	video["url"] = proxyURL
	return true
}

func rewriteGrokMediaVideoContentURLValue(value *any, requestID, proxyURL string) bool {
	if value == nil {
		return false
	}
	switch typed := (*value).(type) {
	case map[string]any:
		changed := false
		for key, child := range typed {
			childValue := child
			if rewriteGrokMediaVideoContentURLValue(&childValue, requestID, proxyURL) {
				typed[key] = childValue
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for index, child := range typed {
			childValue := child
			if rewriteGrokMediaVideoContentURLValue(&childValue, requestID, proxyURL) {
				typed[index] = childValue
				changed = true
			}
		}
		return changed
	case string:
		if isGrokMediaVideoContentURL(typed, requestID) {
			*value = proxyURL
			return true
		}
	}
	return false
}

func isGrokMediaVideoContentURL(rawURL, requestID string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Path == "" {
		return false
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(segments) < 3 {
		return false
	}
	requestID = strings.Trim(requestID, "/")
	decodedID, err := url.PathUnescape(segments[len(segments)-2])
	if err != nil {
		return false
	}
	return segments[len(segments)-3] == "videos" &&
		decodedID == requestID &&
		segments[len(segments)-1] == "content"
}

func grokMediaContentProxyURL(c *gin.Context, requestID string) string {
	if c == nil || c.Request == nil || c.Request.URL == nil || strings.TrimSpace(requestID) == "" {
		return ""
	}
	pathPrefix := ""
	if strings.HasPrefix(c.Request.URL.Path, "/v1/") {
		pathPrefix = "/v1"
	}
	return pathPrefix + "/videos/" + url.PathEscape(strings.Trim(requestID, "/")) + "/content"
}
