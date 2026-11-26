package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tigerowo/infinite-canvas/config"
	"github.com/tigerowo/infinite-canvas/model"
)

const upstreamLoginPage = `<!doctype html><html><head><title>登录·MoMo 面板</title></head><body><div>请先登录</div></body></html>`

func newUpstreamRecorderTest(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	config.Cfg.AILogDir = t.TempDir()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func TestCopyAIResponseFailsFastOnUpstreamLoginPage(t *testing.T) {
	var calls atomic.Int32
	server := newUpstreamRecorderTest(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, upstreamLoginPage)
	})
	channel := model.ModelChannel{ID: "sub2api-relay", Name: "Sub2API", Protocol: "openai", BaseURL: server.URL + "/v1"}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/images/generations", strings.NewReader(`{"model":"gpt-image-2"}`))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	refunded := false
	copyAIResponse(recorder, request, channel, aiLogContext{Model: "gpt-image-2", Endpoint: "/images/generations"}, func() { refunded = true })

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (body=%s)", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, recorder.Body.String())
	}
	if payload.Code != 1 {
		t.Fatalf("code = %d", payload.Code)
	}
	for _, want := range []string{"登录·MoMo 面板", "不是接口数据", "SUB2API_RELAY_BASE_URL"} {
		if !strings.Contains(payload.Msg, want) {
			t.Fatalf("message %q missing %q", payload.Msg, want)
		}
	}
	if !refunded {
		t.Fatal("failed upstream page did not trigger the refund callback")
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls = %d", calls.Load())
	}
}

func TestCopyAIResponseDetectsHTMLBodyWithJSONContentType(t *testing.T) {
	server := newUpstreamRecorderTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "\n  <html><title>Sign in</title></html>")
	})
	channel := model.ModelChannel{ID: "sub2api-relay", Name: "Sub2API", Protocol: "openai", BaseURL: server.URL + "/v1"}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	copyAIResponse(recorder, request, channel, aiLogContext{Model: "gpt-6.1-sol", Endpoint: "/chat/completions"}, nil)
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "Sign in") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCopyAIResponseRejectsPanelBaseURLWithoutCallingUpstream(t *testing.T) {
	var calls atomic.Int32
	server := newUpstreamRecorderTest(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, upstreamLoginPage)
	})
	channel := model.ModelChannel{ID: "sub2api-relay", Name: "Sub2API", Protocol: "openai", BaseURL: server.URL + "/api/v1"}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/v1/images/generations", strings.NewReader(`{"model":"gpt-image-2"}`))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	copyAIResponse(recorder, request, channel, aiLogContext{Model: "gpt-image-2", Endpoint: "/images/generations"}, nil)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "模型网关地址配置错误") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("a misconfigured relay URL still reached the upstream: %d calls", calls.Load())
	}
}

func TestCopyAIResponsePassesJSONPayloadThroughUnchanged(t *testing.T) {
	const payloadBody = `{"data":[{"url":"https://cdn.example.com/a.png"}]}`
	server := newUpstreamRecorderTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, payloadBody)
	})
	channel := model.ModelChannel{ID: "sub2api-relay", Name: "Sub2API", Protocol: "openai", BaseURL: server.URL + "/v1"}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/images/generations", strings.NewReader(`{"model":"gpt-image-2"}`))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	copyAIResponse(recorder, request, channel, aiLogContext{Model: "gpt-image-2", Endpoint: "/images/generations"}, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != payloadBody {
		t.Fatalf("body changed: %s", recorder.Body.String())
	}
}

func TestCopyAIResponseKeepsEventStream(t *testing.T) {
	const stream = "data: {\"choices\":[]}\n\ndata: [DONE]\n\n"
	server := newUpstreamRecorderTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, stream)
	})
	channel := model.ModelChannel{ID: "sub2api-relay", Name: "Sub2API", Protocol: "openai", BaseURL: server.URL + "/v1"}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	copyAIResponse(recorder, request, channel, aiLogContext{Model: "gpt-6.1-sol", Endpoint: "/chat/completions"}, nil)
	if recorder.Code != http.StatusOK || recorder.Body.String() != stream {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestCopyAIResponseFailsFastOnLoginRedirect(t *testing.T) {
	server := newUpstreamRecorderTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://panel.example.com/login?token=secret-token")
		w.WriteHeader(http.StatusFound)
		_, _ = io.WriteString(w, "<html></html>")
	})
	channel := model.ModelChannel{ID: "sub2api-relay", Name: "Sub2API", Protocol: "openai", BaseURL: server.URL + "/v1"}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/images/generations", strings.NewReader(`{"model":"gpt-image-2"}`))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	copyAIResponse(recorder, request, channel, aiLogContext{Model: "gpt-image-2", Endpoint: "/images/generations"}, nil)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "重定向") || !strings.Contains(body, "panel.example.com/login") {
		t.Fatalf("body = %s", body)
	}
	if strings.Contains(body, "secret-token") {
		t.Fatalf("redirect query leaked into the error: %s", body)
	}
}
