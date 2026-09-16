package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestVividVideoAccountRecognizedByBaseURL(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://aigc.easysu.cn/v1"}}
	require.True(t, isVividVideoAccount(account))
	require.True(t, OpenAICompatibleVideoModelSupported(account, "seedance2.0-900-720p"))
}

func TestVividVideoBodyMapsDurationToSeconds(t *testing.T) {
	body, err := prepareVividVideoBody([]byte(`{"model":"seedance2.0-900-720p","prompt":"test","duration":10,"size":"1280x720"}`))
	require.NoError(t, err)
	require.Equal(t, "10", gjson.GetBytes(body, "seconds").String())
	require.Equal(t, "1280x720", gjson.GetBytes(body, "size").String())
	body, err = prepareVividVideoBody([]byte(`{"model":"minimax-h3","duration":"10s"}`))
	require.NoError(t, err)
	require.Equal(t, float64(10), gjson.GetBytes(body, "seconds").Float())
	body, err = prepareVividVideoBody([]byte(`{"model":"seedance-2.0","duration":10}`))
	require.NoError(t, err)
	require.Equal(t, "seedance2.0-900-720p", gjson.GetBytes(body, "model").String())
}

func TestVividVideoBodyNormalizesOpenAIReferenceImages(t *testing.T) {
	body, err := prepareVividVideoBody([]byte(`{"model":"seedance2.0-900-720p","prompt":"test","reference_images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AAA"}}]}`))
	require.NoError(t, err)
	require.Equal(t, "data:image/png;base64,AAA", gjson.GetBytes(body, "input_reference.0").String())
	require.False(t, gjson.GetBytes(body, "reference_images").Exists())
}

func TestVividSeedance25UsesPureBase64ReferenceImages(t *testing.T) {
	body, err := prepareVividVideoBody([]byte(`{"model":"seedance2.5-9图","prompt":"test","images":[{"url":"data:image/png;base64,AAA"}]}`))
	require.NoError(t, err)
	require.Equal(t, "AAA", gjson.GetBytes(body, "reference_images.0").String())
	require.False(t, gjson.GetBytes(body, "input_reference").Exists())
}

func TestVividMiniMaxUsesInputReferenceForPureBase64Images(t *testing.T) {
	body, err := prepareVividVideoBody([]byte(`{"model":"minimax-h3-933-图文","prompt":"test","reference_images":["AAA"]}`))
	require.NoError(t, err)
	require.Equal(t, "AAA", gjson.GetBytes(body, "input_reference.0").String())
	require.False(t, gjson.GetBytes(body, "reference_images").Exists())
}

func TestVividVideoModelRoutingDoesNotUseGrokAccounts(t *testing.T) {
	require.True(t, IsOpenAICompatibleVideoModel("grok-video-1.5"))
	require.False(t, IsOpenAICompatibleVideoModel("grok-imagine-video"))
}

func TestNativeGrokVideoRemovesVividCompatibilityFields(t *testing.T) {
	body, contentType, err := sanitizeGrokMediaForwardBody(GrokMediaEndpointVideosGenerations, []byte(`{"model":"grok-imagine-video","prompt":"test","seconds":"8","size":"1280x720","duration":8,"aspect_ratio":"16:9"}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, "application/json", contentType)
	require.False(t, gjson.GetBytes(body, "seconds").Exists())
	require.False(t, gjson.GetBytes(body, "size").Exists())
	require.Equal(t, "8", gjson.GetBytes(body, "duration").String())
}

func TestVividVideoContentUsesContentEndpointAndStreamsMP4(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"video/mp4"}, "Content-Length": []string{"4"}},
		Body:       io.NopCloser(bytes.NewReader([]byte("MP4!"))),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "vivid-key", "base_url": "https://aigc.easysu.cn/v1",
	}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/job-1/content", nil)
	result, err := svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideoContent, "job-1", nil, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://aigc.easysu.cn/v1/videos/job-1/content", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer vivid-key", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "MP4!", rec.Body.String())
	require.Equal(t, "video/mp4", rec.Header().Get("Content-Type"))
}

func TestVividVideoGenerationUsesOpenAIAsyncEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"job-2","status":"queued"}`))),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "vivid-key", "base_url": "https://aigc.easysu.cn/v1",
	}}
	body := []byte(`{"model":"seedance2.0-900-720p","prompt":"a city at night","duration":10,"ratio":"16:9","input_reference":"https://img/ref.png"}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	result, err := svc.ForwardGrokMedia(context.Background(), c, account, GrokMediaEndpointVideosGenerations, "", body, "application/json")
	require.NoError(t, err)
	require.Equal(t, "https://aigc.easysu.cn/v1/videos", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer vivid-key", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "10", gjson.GetBytes(upstream.lastBody, "seconds").String())
	require.Equal(t, "16:9", gjson.GetBytes(upstream.lastBody, "ratio").String())
	require.Equal(t, "https://img/ref.png", gjson.GetBytes(upstream.lastBody, "input_reference.0").String())
	require.Equal(t, "job-2", result.ResponseID)
}

func TestVividVideoAccountIgnoresLegacyMappingForKnownVideoModel(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"base_url":      "https://aigc.easysu.cn/v1",
		"model_mapping": map[string]any{"gpt-4o": "gpt-4o"},
	}}
	require.True(t, account.IsModelSupported("seedance2.0-900-720p"))
	require.False(t, account.IsModelSupported("unknown-video"))
}
