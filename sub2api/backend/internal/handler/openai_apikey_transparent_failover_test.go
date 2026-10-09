//go:build unit

package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 仅替代网络和存储边界，实际执行处理器、选号、排除故障账号和协议转发循环。
type transparentFailoverUpstream struct {
	service.HTTPUpstream
	mu      sync.Mutex
	hits    []int64
	failure string
}

func (u *transparentFailoverUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.hits = append(u.hits, accountID)
	u.mu.Unlock()
	isChat := strings.HasSuffix(req.URL.Path, "/chat/completions")
	preamble := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"failed-attempt\",\"status\":\"in_progress\",\"output\":[]}}\n\n"
	semantic := "data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"partial-output\"}\n\n"
	if isChat {
		preamble = "data: {\"id\":\"failed-attempt\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"}}]}\n\n"
		semantic = "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial-output\"}}]}\n\n"
	}
	if accountID == 1 && u.failure != "" {
		switch u.failure {
		case "transport":
			return nil, errors.New("upstream connection reset before headers")
		case "http_502", "http_429":
			status := http.StatusBadGateway
			if u.failure == "http_429" {
				status = http.StatusTooManyRequests
			}
			return &http.Response{StatusCode: status,
				Header: http.Header{"Content-Type": {"application/json"}, "Retry-After": {"1"}},
				Body:   io.NopCloser(strings.NewReader(`{"error":{"type":"server_error","message":"temporary upstream unavailable"}}`))}, nil
		case "preamble_error":
			preamble += "event: error\ndata: {\"type\":\"error\",\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"try again later\"}}\n\n"
		case "semantic_error":
			preamble += semantic + "event: error\ndata: {\"type\":\"error\",\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"try again later\"}}\n\n"
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(preamble))}, nil
	}
	body := strings.ReplaceAll(preamble, "failed-attempt", "healthy-attempt") + strings.ReplaceAll(semantic, "partial-output", "healthy-output")
	if isChat {
		body += "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	} else {
		body += "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"healthy-attempt\",\"model\":\"test-model\",\"status\":\"completed\",\"output\":[{\"id\":\"msg1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"healthy-output\"}]}]}}\n\n"
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(body))}, nil
}

func (u *transparentFailoverUpstream) calls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.hits...)
}

type transparentFailoverSlots struct {
	testutil.StubConcurrencyCache
	mu      sync.Mutex
	fullID  int64
	waiters int
}

func (s *transparentFailoverSlots) AcquireAccountSlot(_ context.Context, id int64, _ int, _ string) (bool, error) {
	return id != s.fullID, nil
}

func (s *transparentFailoverSlots) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waiters++
	return true, nil
}

type transparentFailoverBinding struct {
	testutil.StubGatewayCache
}

func (transparentFailoverBinding) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 1, nil
}

func newTransparentFailoverHandler(t *testing.T, upstream *transparentFailoverUpstream, rawChat bool, slots *transparentFailoverSlots) *OpenAIGatewayHandler {
	t.Helper()
	accounts := make([]service.Account, 2)
	for i := range accounts {
		accounts[i] = service.Account{ID: int64(i + 1), Name: "upstream", Platform: service.PlatformOpenAI,
			Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
			Concurrency: 0, Priority: i, GroupIDs: []int64{3131},
			Credentials: map[string]any{"api_key": "test-key", "base_url": "https://upstream.example.test",
				"pool_mode": true, "pool_mode_retry_count": float64(5), "fast_failover": true},
			Extra: map[string]any{"openai_passthrough": true, "openai_responses_supported": !rawChat}}
		if slots != nil {
			accounts[i].Concurrency = 100
		}
	}
	repo := openAIImagesFailoverAccountRepo{accounts: accounts}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.MaxAccountSwitches = 3
	cfg.Gateway.Scheduling.StickySessionWaitTimeout = 30 * time.Second
	cfg.Gateway.Scheduling.FallbackWaitTimeout = 30 * time.Second
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 3
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 3
	cfg.Gateway.OpenAIScheduler.StickyEscapeEnabled = true
	var cache service.GatewayCache
	var concurrency *service.ConcurrencyService
	if slots != nil {
		cache = transparentFailoverBinding{}
		concurrency = service.NewConcurrencyService(slots)
	} else {
		concurrency = service.NewConcurrencyService(nil)
	}
	gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, cache, cfg, nil,
		concurrency, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	return NewOpenAIGatewayHandler(gateway, concurrency, billing,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
}

func transparentFailoverContext(path string) (*gin.Context, *httptest.ResponseRecorder) {
	groupID := int64(3131)
	body := `{"model":"test-model","stream":true,"input":"hello"}`
	if path != "/v1/responses" {
		body = `{"model":"test-model","stream":true,"max_tokens":128,"messages":[{"role":"user","content":"hello"}]}`
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("session-id", "unchanged-client-session")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 99, UserID: 100, GroupID: &groupID,
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, AllowMessagesDispatch: true}, User: &service.User{ID: 100}})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100, Concurrency: 0})
	return c, rec
}

func serveTransparentFailover(h *OpenAIGatewayHandler, c *gin.Context) {
	switch c.Request.URL.Path {
	case "/v1/responses":
		h.Responses(c)
	case "/v1/messages":
		h.Messages(c)
	default:
		h.ChatCompletions(c)
	}
}

func TestAPIKeyTransparentFailoverHandlerLoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, rawChat := range []bool{true, false} {
		protocol := "responses"
		if rawChat {
			protocol = "chat"
		}
		for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"} {
			for _, failure := range []string{"transport", "http_502", "http_429", "preamble_error", "preamble_eof", "semantic_error"} {
				t.Run(protocol+path+"/"+failure, func(t *testing.T) {
					upstream := &transparentFailoverUpstream{failure: failure}
					h := newTransparentFailoverHandler(t, upstream, rawChat, nil)
					c, rec := transparentFailoverContext(path)
					serveTransparentFailover(h, c)
					if failure == "semantic_error" {
						require.Equal(t, []int64{1}, upstream.calls(), "已提交正文后不得重放另一个上游")
						require.Contains(t, rec.Body.String(), "partial-output")
						require.NotContains(t, rec.Body.String(), "healthy-output")
						return
					}
					require.Equal(t, []int64{1, 2}, upstream.calls(), "故障账号应只调用一次，随后在同一客户请求内切到备用；响应：%s", rec.Body.String())
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					require.Contains(t, rec.Body.String(), "healthy-output")
					require.NotContains(t, rec.Body.String(), "failed-attempt")
					require.NotContains(t, rec.Body.String(), "try again later")
				})
			}
		}
	}
}

func TestAPIKeyTransparentFailoverBusyStickyUsesHealthyAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &transparentFailoverUpstream{}
	slots := &transparentFailoverSlots{fullID: 1}
	h := newTransparentFailoverHandler(t, upstream, true, slots)
	c, rec := transparentFailoverContext("/v1/chat/completions")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	serveTransparentFailover(h, c)
	require.Equal(t, []int64{2}, upstream.calls(), "粘性账号满载时应使用有空闲槽位的同组账号；响应：%s", rec.Body.String())
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "healthy-output")
	require.Zero(t, slots.waiters, "存在空闲备用时不应进入30秒账号队列")
}

// 500 个真实处理器请求共享同一个网关，验证重选循环不会跨请求混入失败首包。
// 此处只模拟上游网络与仓储，不替代实际模型供应商的容量验收。
func TestAPIKeyTransparentFailoverConcurrent500HandlerRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &transparentFailoverUpstream{failure: "preamble_error"}
	h := newTransparentFailoverHandler(t, upstream, true, nil)
	const count = 500
	start := make(chan struct{})
	failures := make(chan error, count)
	var ready, completed sync.WaitGroup
	ready.Add(count)
	completed.Add(count)
	for i := range count {
		go func(index int) {
			defer completed.Done()
			c, rec := transparentFailoverContext("/v1/chat/completions")
			ready.Done()
			<-start
			serveTransparentFailover(h, c)
			if rec.Code != http.StatusOK || strings.Count(rec.Body.String(), "healthy-output") != 1 ||
				strings.Contains(rec.Body.String(), "failed-attempt") || strings.Contains(rec.Body.String(), "try again later") {
				failures <- fmt.Errorf("请求%d返回不完整，状态%d，正文%s", index, rec.Code, rec.Body.String())
			}
		}(i)
	}
	ready.Wait()
	close(start)
	done := make(chan struct{})
	go func() {
		completed.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("500个处理器请求未在测试期限内完成")
	}
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	healthy, failed := 0, 0
	for _, id := range upstream.calls() {
		if id == 1 {
			failed++
		}
		if id == 2 {
			healthy++
		}
	}
	require.Equal(t, count, healthy)
	require.Positive(t, failed)
	require.LessOrEqual(t, failed, count, "每个客户请求不应重复访问故障账号")
	t.Logf("500个真实处理器请求完成，故障上游尝试%d次，健康上游完成%d次", failed, healthy)
}
