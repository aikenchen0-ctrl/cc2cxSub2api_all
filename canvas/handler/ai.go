package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/service"
)

const userModelChannelHeader = "X-User-Model-Channel-ID"

func selectAIRequestChannel(user model.AuthUser, modelName string, channelID string, userChannelID string) (model.ModelChannel, string, error) {
	// Ordinary users always use the server-managed Sub2API relay when it is
	// configured. This keeps provider credentials and billing ownership out of
	// the browser and makes all user model calls follow one policy.
	if service.Sub2APIRelayEnabled() {
		channel, err := service.SelectSub2APIRelayChannelForModel(modelName)
		if err != nil {
			return channel, "", err
		}
		return service.OverlayUserSub2APIKey(channel, user.ID), "", nil
	}
	userChannelID = strings.TrimSpace(userChannelID)
	if userChannelID != "" {
		channel, err := service.SelectUserLocalModelChannelForModel(user.ID, modelName, userChannelID)
		return channel, userChannelID, err
	}
	if !service.UserCanUseRemoteModelChannel(user) {
		return model.ModelChannel{}, "", fmt.Errorf("当前账号未开放云端渠道")
	}
	channel, err := service.SelectModelChannelForModel(modelName, channelID)
	return channel, "", err
}

func failAIChannelSelect(w http.ResponseWriter, err error, fallback string) {
	message := strings.TrimSpace(err.Error())
	switch message {
	case "当前账号未开放云端渠道", "请先登录", "缺少模型名称", "缺少模型渠道", "本地渠道不存在", "本地渠道配置不完整", "本地渠道不支持该模型", "指定模型渠道不可用":
		Fail(w, message)
	default:
		Fail(w, fallback)
	}
}

func AIImagesGenerations(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/images/generations")
}

func AIImagesEdits(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/images/edits")
}

func AIChatCompletions(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/chat/completions")
}

func AIResponses(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/responses")
}

func AIVideos(w http.ResponseWriter, r *http.Request) {
	proxyAIVideoTaskRequest(w, r)
}

func AIVideo(w http.ResponseWriter, r *http.Request, id string) {
	if serveAIVideoTask(w, r, id) {
		return
	}
	if isClientVideoTaskID(id) {
		OK(w, map[string]any{"id": id, "task_id": id, "object": "video", "status": "queued", "progress": 0})
		return
	}
	proxyAIGetRequest(w, r, "/videos/"+id)
}

func AIVideoContent(w http.ResponseWriter, r *http.Request, id string) {
	if serveSub2APIVideoContent(w, r, id) {
		return
	}
	for _, adapter := range builtinAIProtocols {
		if adapter.videoContent != nil && adapter.videoContent(w, r, id) {
			return
		}
	}
	proxyAIGetRequest(w, r, "/videos/"+id+"/content")
}

func AIAudioSpeech(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/audio/speech")
}

func AITTSVoices(w http.ResponseWriter, r *http.Request) {
	modelName := strings.TrimSpace(r.URL.Query().Get("model"))
	if modelName == "" {
		Fail(w, "缺少模型名称")
		return
	}
	proxyAIGetRequest(w, r, "/tts/voices?model="+url.QueryEscape(modelName))
}

func proxyAIGetRequest(w http.ResponseWriter, r *http.Request, path string) {
	startedAt := time.Now()
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	modelName := r.URL.Query().Get("model")
	if strings.TrimSpace(modelName) == "" {
		modelName = "Agnes-Video-V2.0"
	}
	channel, _, err := selectAIRequestChannel(user, modelName, r.Header.Get("X-Model-Channel-ID"), r.Header.Get(userModelChannelHeader))
	if err != nil {
		log.Printf("AI proxy select channel failed: model=%s err=%v", modelName, err)
		failAIChannelSelect(w, err, "AI 接口请求失败")
		return
	}
	upstreamPath := resolveAIProxyPath(channel, modelName, path)
	if service.IsSub2APIChannel(channel) && strings.HasPrefix(path, "/videos/") {
		FailWithStatus(w, http.StatusNotFound, "视频任务不存在")
		return
	}
	request, err := http.NewRequest(http.MethodGet, resolveAIProxyURL(channel, modelName, upstreamPath), nil)
	if err != nil {
		Fail(w, "AI 接口请求失败")
		return
	}
	if err := service.SetModelChannelAuthHeader(request, channel); err != nil {
		log.Printf("AI proxy auth failed: model=%s err=%v", modelName, err)
		FailWithStatus(w, http.StatusServiceUnavailable, "AI relay 身份尚未准备好")
		return
	}
	copyAIResponse(w, request, channel, aiLogContext{StartedAt: startedAt, Endpoint: path, Method: http.MethodGet, Model: modelName, Channel: channel, UserID: user.ID, UserDisplayName: firstNonEmpty(user.DisplayName, user.Username), RequestBody: summarizeQueryParams(r.URL.Query())}, nil)
}

func proxyAIRequest(w http.ResponseWriter, r *http.Request, path string) {
	startedAt := time.Now()
	body, contentType, modelName, err := readAIRequest(r)
	if err != nil {
		log.Printf("AI proxy request read failed: %v", err)
		Fail(w, "AI 接口请求失败")
		return
	}
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	channel, userChannelID, err := selectAIRequestChannel(user, modelName, r.Header.Get("X-Model-Channel-ID"), r.Header.Get(userModelChannelHeader))
	if err != nil {
		log.Printf("AI proxy select channel failed: model=%s err=%v", modelName, err)
		failAIChannelSelect(w, err, "AI 接口请求失败")
		return
	}
	credits := 0.0
	if userChannelID == "" {
		credits, err = service.ModelCost(modelName)
		if err != nil {
			log.Printf("AI proxy read model cost failed: model=%s err=%v", modelName, err)
			Fail(w, "AI 接口请求失败")
			return
		}
		credits *= float64(readAIRequestCount(body, contentType, false))
	}
	upstreamPath := resolveAIProxyPath(channel, modelName, path)
	prepared, _, err := prepareAIProtocolRequest(aiProtocolRequest{
		mode: aiProtocolProxyRequest, body: body, contentType: contentType, modelName: modelName,
		channel: channel, endpoint: path, path: upstreamPath,
	})
	if err != nil {
		log.Printf("AI proxy normalize %s request failed: model=%s err=%v", prepared.failureLabel, modelName, err)
		message := "AI 接口请求失败"
		if prepared.failureLabel == "MiMo TTS" || prepared.failureLabel == "AutoDL" {
			message = err.Error()
		}
		Fail(w, message)
		return
	}
	body, contentType, upstreamPath = prepared.body, prepared.contentType, prepared.path
	request, err := http.NewRequest(http.MethodPost, service.BuildModelChannelURL(channel, upstreamPath), bytes.NewReader(body))
	if err != nil {
		log.Printf("AI proxy build request failed: url=%s err=%v", service.BuildModelChannelURL(channel, upstreamPath), err)
		Fail(w, "AI 接口请求失败")
		return
	}
	if err := service.SetModelChannelAuthHeader(request, channel); err != nil {
		log.Printf("AI proxy auth failed: model=%s err=%v", modelName, err)
		FailWithStatus(w, http.StatusServiceUnavailable, "AI relay 身份尚未准备好")
		return
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if credits > 0 {
		if err := service.ConsumeUserCredits(user.ID, modelName, credits, upstreamPath); err != nil {
			FailError(w, err)
			return
		}
	}
	copyAIResponse(w, request, channel, aiLogContext{
		StartedAt:       startedAt,
		Endpoint:        path,
		Method:          http.MethodPost,
		Model:           modelName,
		Channel:         channel,
		UserID:          user.ID,
		UserDisplayName: firstNonEmpty(user.DisplayName, user.Username),
		Credits:         credits,
		RequestBody:     summarizeAIRequest(body, contentType),
	}, func() {
		if credits > 0 {
			if err := service.RefundUserCredits(user.ID, modelName, credits, upstreamPath); err != nil {
				log.Printf("AI proxy refund credits failed: user=%s model=%s credits=%g err=%v", user.ID, modelName, credits, err)
			}
		}
	})
}

func geminiStreamRequested(body []byte) bool {
	var payload struct {
		Stream *bool `json:"stream"`
	}
	return json.Unmarshal(body, &payload) == nil && payload.Stream != nil && *payload.Stream
}

type aiLogContext struct {
	StartedAt       time.Time
	Endpoint        string
	Method          string
	Model           string
	Channel         model.ModelChannel
	UserID          string
	UserDisplayName string
	Credits         float64
	RequestBody     string
}

func copyAIResponse(w http.ResponseWriter, request *http.Request, channel model.ModelChannel, logContext aiLogContext, onFailure func()) {
	// Refuse to call a relay URL that cannot be the model gateway. A panel
	// origin or an /api/v1 base silently becomes /api/v1/v1/... and the panel
	// answers with its HTML login page instead of an API error.
	if service.IsSub2APIChannel(channel) {
		if err := service.ValidateSub2APIRequestURL(request.URL.String()); err != nil {
			log.Printf("AI proxy rejected relay URL: url=%s err=%v", request.URL.String(), err)
			if onFailure != nil {
				onFailure()
			}
			saveAIProxyLog(logContext, http.StatusBadGateway, "", err.Error())
			FailWithStatus(w, http.StatusBadGateway, "模型网关地址配置错误："+err.Error())
			return
		}
	}
	response, err := service.HTTPClientForChannel(channel).Do(request)
	if err != nil {
		log.Printf("AI proxy request failed: url=%s err=%v", request.URL.String(), err)
		if onFailure != nil {
			onFailure()
		}
		saveAIProxyLog(logContext, 0, "", err.Error())
		Fail(w, "AI 接口请求失败")
		return
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 256*1024))
		log.Printf("AI upstream error: url=%s status=%d body=%s", request.URL.String(), response.StatusCode, strings.TrimSpace(string(payload)))
		if onFailure != nil {
			onFailure()
		}
		saveAIProxyLog(logContext, response.StatusCode, string(payload), strings.TrimSpace(string(payload)))
		status := response.StatusCode
		if status < http.StatusBadRequest {
			status = http.StatusBadGateway
		}
		FailWithStatus(w, status, readUpstreamAIErrorMessage(payload, response.StatusCode))
		return
	}

	// A redirect (302 to a login page, or to another host) is never a model
	// answer either. The relay client intentionally does not follow redirects.
	if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
		location := redactedURLLocation(response.Header.Get("Location"))
		log.Printf("AI upstream redirected instead of answering: url=%s status=%d location=%s", request.URL.String(), response.StatusCode, location)
		if onFailure != nil {
			onFailure()
		}
		message := fmt.Sprintf("上游把请求重定向到其他页面（HTTP %d），通常是未登录或网关地址配置错误；请检查模型网关地址与凭据", response.StatusCode)
		if location != "" {
			message += "（跳转目标：" + location + "）"
		}
		saveAIProxyLog(logContext, http.StatusBadGateway, "", message)
		FailWithStatus(w, http.StatusBadGateway, message)
		return
	}

	// Upstream 2xx bodies are expected to be API payloads. A wrong base URL,
	// an expired panel session, or a proxy in front of the gateway can answer
	// with an HTML login page instead; parsing that page as JSON only reports
	// `invalid character '<'` after the fact, so fail fast and stay explicit.
	bodyReader := bufio.NewReaderSize(response.Body, 32*1024)
	response.Body = struct {
		io.Reader
		io.Closer
	}{bodyReader, response.Body}
	if isUpstreamWebPage(response, bodyReader) {
		page, _ := io.ReadAll(io.LimitReader(bodyReader, 64*1024))
		message := nonAPIWebPageMessage(channel, page)
		log.Printf("AI upstream returned a web page instead of an API payload: url=%s status=%d contentType=%q title=%q", request.URL.String(), response.StatusCode, response.Header.Get("Content-Type"), htmlPageTitle(page))
		if onFailure != nil {
			onFailure()
		}
		saveAIProxyLog(logContext, http.StatusBadGateway, string(page), message)
		FailWithStatus(w, http.StatusBadGateway, message)
		return
	}

	if copyAIProtocolResponse(w, response, request, channel, logContext, onFailure) {
		return
	}

	for key, values := range response.Header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	responseBody := copyAIResponseBody(w, response.Body, !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "video/"))
	saveAIProxyLog(logContext, response.StatusCode, responseBody, "")
}

func copyAIResponseBody(w http.ResponseWriter, body io.Reader, capture bool) string {
	flusher, canFlush := w.(http.Flusher)
	buffer := make([]byte, 32*1024)
	var logBuffer strings.Builder
	for {
		n, err := body.Read(buffer)
		if n > 0 {
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				return logBuffer.String()
			}
			if capture && logBuffer.Len() < 64*1024 {
				_, _ = logBuffer.Write(buffer[:min(n, 64*1024-logBuffer.Len())])
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			return logBuffer.String()
		}
	}
}

// redactedURLLocation keeps a redirect target identifiable without leaking
// query strings that may carry session tokens.
func redactedURLLocation(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "[relative]"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

// isUpstreamWebPage reports whether a 2xx upstream response is a web page
// (typically a panel login page) instead of an API payload.
func isUpstreamWebPage(response *http.Response, reader *bufio.Reader) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(response.Header.Get("Content-Type")))
	if err == nil {
		switch strings.ToLower(mediaType) {
		case "text/html", "application/xhtml+xml":
			return true
		}
	}
	if isStreamingUpstreamContentType(strings.ToLower(mediaType)) {
		// Never block an event stream or a binary download to sniff its prefix.
		return false
	}
	head, _ := reader.Peek(64)
	return looksLikeWebPageBody(head)
}

func isStreamingUpstreamContentType(mediaType string) bool {
	return mediaType == "text/event-stream" ||
		mediaType == "application/octet-stream" ||
		mediaType == "application/zip" ||
		strings.HasPrefix(mediaType, "video/") ||
		strings.HasPrefix(mediaType, "audio/") ||
		strings.HasPrefix(mediaType, "image/")
}

func looksLikeWebPageBody(head []byte) bool {
	trimmed := bytes.TrimLeft(bytes.TrimSpace(head), "\xef\xbb\xbf")
	return len(trimmed) > 0 && trimmed[0] == '<'
}

func htmlPageTitle(page []byte) string {
	lower := bytes.ToLower(page)
	start := bytes.Index(lower, []byte("<title"))
	if start < 0 {
		return ""
	}
	open := bytes.IndexByte(lower[start:], '>')
	if open < 0 {
		return ""
	}
	content := page[start+open+1:]
	if end := bytes.Index(bytes.ToLower(content), []byte("</title>")); end >= 0 {
		content = content[:end]
	}
	title := strings.Join(strings.Fields(string(content)), " ")
	if utf8.RuneCountInString(title) > 60 {
		title = string([]rune(title)[:60]) + "…"
	}
	return title
}

func nonAPIWebPageMessage(channel model.ModelChannel, page []byte) string {
	title := htmlPageTitle(page)
	reason := "上游返回的是网页而不是接口数据"
	if title != "" {
		reason = fmt.Sprintf("上游返回的是网页「%s」而不是接口数据", title)
	}
	if service.IsSub2APIChannel(channel) {
		return reason + "：请检查 SUB2API_RELAY_BASE_URL 是否指向模型网关（如 https://api.cc2.cx 或 http://sub2api:8080/v1），确认应用凭据未失效，不要指向面板首页或登录页。"
	}
	return reason + "：请检查该渠道 Base URL 是否为 /v1 接口地址，以及上游登录凭据是否已失效。"
}

func saveAIProxyLog(context aiLogContext, status int, responseBody string, errorMessage string) {
	if context.StartedAt.IsZero() {
		context.StartedAt = time.Now()
	}
	if service.IsGeminiChannel(context.Channel) {
		if context.Endpoint == "/images/generations" || context.Endpoint == "/images/edits" || context.Endpoint == "/audio/speech" {
			responseBody = "[redacted Gemini media response]"
		} else if strings.TrimSpace(responseBody) != "" {
			responseBody = summarizeAIRequest([]byte(responseBody), "application/json")
		}
	}
	service.SaveAICallLog(service.AICallLogInput{
		UserID:          context.UserID,
		UserDisplayName: context.UserDisplayName,
		Endpoint:        context.Endpoint,
		Method:          context.Method,
		Model:           context.Model,
		ChannelID:       context.Channel.ID,
		ChannelName:     context.Channel.Name,
		Status:          status,
		DurationMs:      time.Since(context.StartedAt).Milliseconds(),
		Credits:         context.Credits,
		RequestBody:     context.RequestBody,
		ResponseBody:    responseBody,
		Error:           errorMessage,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func summarizeAIRequest(body []byte, contentType string) string {
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return summarizeMultipartAIRequest(body, contentType)
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err == nil {
		redactLargeImages(&payload)
		if encoded, err := json.MarshalIndent(payload, "", "  "); err == nil {
			return string(encoded)
		}
	}
	return string(body)
}

func summarizeQueryParams(values map[string][]string) string {
	if len(values) == 0 {
		return ""
	}
	encoded, _ := json.MarshalIndent(values, "", "  ")
	return string(encoded)
}

func summarizeMultipartAIRequest(body []byte, contentType string) string {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "multipart/form-data"
	}
	form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(32 << 20)
	if err != nil {
		return "multipart/form-data"
	}
	defer form.RemoveAll()
	summary := map[string]any{"fields": form.Value}
	files := []map[string]any{}
	for field, headers := range form.File {
		for _, header := range headers {
			files = append(files, map[string]any{"field": field, "filename": header.Filename, "size": header.Size, "contentType": header.Header.Get("Content-Type")})
		}
	}
	summary["files"] = files
	encoded, _ := json.MarshalIndent(summary, "", "  ")
	return string(encoded)
}

func readUpstreamAIErrorMessage(body []byte, statusCode int) string {
	var payload struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Msg     string `json:"msg"`
		Message string `json:"message"`
	}
	if len(body) > 0 && json.Unmarshal(body, &payload) == nil {
		if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
			return payload.Error.Message
		}
		if strings.TrimSpace(payload.Msg) != "" {
			return payload.Msg
		}
		if strings.TrimSpace(payload.Message) != "" {
			return payload.Message
		}
	}
	if statusCode > 0 {
		return fmt.Sprintf("AI 接口请求失败：%d", statusCode)
	}
	return "AI 接口请求失败"
}

func redactLargeImages(value *any) {
	switch typed := (*value).(type) {
	case map[string]any:
		for key, item := range typed {
			if text, ok := item.(string); ok && (strings.HasPrefix(text, "data:image/") || strings.HasPrefix(text, "data:video/") || strings.HasPrefix(text, "data:audio/") || len(text) > 2048 && looksLikeBase64(text)) {
				typed[key] = fmt.Sprintf("[redacted media/string len=%d]", len(text))
				continue
			}
			redactLargeImages(&item)
			typed[key] = item
		}
	case []any:
		for index, item := range typed {
			redactLargeImages(&item)
			typed[index] = item
		}
	}
}

func looksLikeBase64(value string) bool {
	for _, char := range value[:min(len(value), 200)] {
		if !(char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '+' || char == '/' || char == '=') {
			return false
		}
	}
	return true
}

func readAIRequest(r *http.Request) ([]byte, string, string, error) {
	contentType := r.Header.Get("Content-Type")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, "", "", err
	}
	modelName := ""
	if strings.HasPrefix(contentType, "multipart/form-data") {
		modelName = readMultipartModel(body, contentType)
	} else {
		var payload struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &payload)
		modelName = payload.Model
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, "", "", errMissingModel
	}
	return body, contentType, modelName, nil
}

func readMultipartModel(body []byte, contentType string) string {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	form, err := reader.ReadForm(32 << 20)
	if err != nil {
		return ""
	}
	defer form.RemoveAll()
	if values := form.Value["model"]; len(values) > 0 {
		return values[0]
	}
	return ""
}

func readAIRequestCount(body []byte, contentType string, video bool) int {
	count := 1
	var videoPayload map[string]any
	if strings.HasPrefix(contentType, "multipart/form-data") {
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return count
		}
		form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(32 << 20)
		if err != nil {
			return count
		}
		defer form.RemoveAll()
		if video {
			count = 0
			for _, field := range []string{"seconds", "duration"} {
				if values := form.Value[field]; len(values) > 0 {
					if _, err := fmt.Sscan(values[0], &count); err == nil && count != 0 {
						break
					}
				}
			}
		} else if values := form.Value["n"]; len(values) > 0 {
			_, _ = fmt.Sscan(values[0], &count)
		}
	} else if video {
		count = 0
		if json.Unmarshal(body, &videoPayload) == nil {
			for _, field := range []string{"seconds", "duration", "parameters.durationSeconds"} {
				if _, err := fmt.Sscan(readStringPath(videoPayload, field), &count); err == nil && count != 0 {
					break
				}
			}
		}
	} else {
		var payload struct {
			N int `json:"n"`
		}
		_ = json.Unmarshal(body, &payload)
		count = payload.N
	}
	if video && count == -1 {
		return 15
	}
	if video && count < 1 {
		frames := readIntPath(videoPayload, "num_frames")
		frameRate := readIntPath(videoPayload, "frame_rate")
		if frames > 1 && frameRate > 0 {
			count = (frames - 1 + frameRate - 1) / frameRate
		}
	}
	if count < 1 {
		return 1
	}
	return count
}

func resolveAIProxyURL(channel model.ModelChannel, modelName string, path string) string {
	if service.IsSub2APIChannel(channel) {
		return service.BuildModelChannelURL(channel, path)
	}
	for _, adapter := range builtinAIProtocols {
		if adapter.url != nil {
			if resolved, ok := adapter.url(channel, modelName, path); ok {
				return resolved
			}
		}
	}
	return service.BuildModelChannelURL(channel, path)
}

func agnesVideoQueryID(modelName string, path string) (string, bool) {
	if !isAgnesVideoModel(modelName) || !strings.HasPrefix(path, "/videos/") || strings.HasSuffix(path, "/content") {
		return "", false
	}
	id := strings.TrimPrefix(path, "/videos/")
	if strings.HasPrefix(id, "video_") {
		return id, true
	}
	return "", false
}

func resolveAIProxyPath(channel model.ModelChannel, modelName string, path string) string {
	if service.IsSub2APIChannel(channel) {
		return path
	}
	for _, adapter := range builtinAIProtocols {
		if adapter.path != nil {
			if resolved, ok := adapter.path(channel, modelName, path); ok {
				return resolved
			}
		}
	}
	return path
}

func isCogVideoX3Model(modelName string) bool {
	return strings.EqualFold(strings.TrimSpace(modelName), "cogvideox-3")
}

func isAgnesVideoModel(modelName string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(modelName)), "agnes-video")
}

var errMissingModel = &aiError{"缺少模型名称"}

type aiError struct {
	message string
}

func (err *aiError) Error() string {
	return err.message
}
