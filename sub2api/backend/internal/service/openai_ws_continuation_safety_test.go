package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAINativeWSLaterTurnFailoverUsesObservedHistory(t *testing.T) {
	for _, knownParent := range []bool{true, false} {
		name := "unknown_parent_is_not_replayed"
		if knownParent {
			name = "known_parent_replays_current_tool_result"
		}
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
			firstConn := &openAIWSCaptureConn{events: [][]byte{
				[]byte(`{"type":"response.completed","response":{"id":"resp_first","model":"gpt-5.1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"saved alpha"}]},{"type":"function_call","call_id":"call_a","name":"inspect","arguments":"{}"}],"usage":{"input_tokens":1,"output_tokens":1}}}`),
				[]byte(`{"type":"error","error":{"code":"rate_limit_exceeded","type":"rate_limit_error","message":"rate limit exceeded"}}`),
			}}
			secondConn := &openAIWSCaptureConn{events: [][]byte{
				[]byte(`{"type":"response.completed","response":{"id":"resp_second","model":"gpt-5.1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"finished beta"}]}],"usage":{"input_tokens":4,"output_tokens":1}}}`),
			}}
			dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{firstConn, secondConn}}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(dialer)
			svc := &OpenAIGatewayService{
				cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{},
				openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), openaiWSPool: pool,
				toolCorrector: NewCodexToolCorrector(),
			}
			account := &Account{
				ID: 9401, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1,
				Credentials: map[string]any{"api_key": "test-key-a"},
				Extra:       map[string]any{"responses_websockets_v2_enabled": true},
			}
			nextAccount := *account
			nextAccount.ID = 9402
			nextAccount.Credentials = map[string]any{"api_key": "test-key-b"}
			serverErrCh := make(chan error, 1)
			retryCh := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					serverErrCh <- err
					return
				}
				defer conn.CloseNow()
				ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ginCtx.Request = r.Clone(r.Context())
				_, initial, err := conn.Read(r.Context())
				if err != nil {
					serverErrCh <- err
					return
				}
				err = svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "test-key-a", initial, nil)
				var failover *UpstreamFailoverError
				if !errors.As(err, &failover) {
					serverErrCh <- errors.New("expected upstream failover")
					return
				}
				retry, currentTurn := OpenAIWSCurrentTurnRetryPayload(err)
				if !currentTurn {
					serverErrCh <- errors.New("missing current-turn failure envelope")
					return
				}
				retryCh <- retry
				if !knownParent {
					serverErrCh <- nil
					return
				}
				serverErrCh <- svc.ProxyResponsesWebSocketFromClient(
					r.Context(), ginCtx, conn, &nextAccount, "test-key-b", retry, nil)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			client, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer client.CloseNow()
			initial := `{"type":"response.create","model":"gpt-5.1","input":[{"role":"user","content":"remember alpha"}]}`
			require.NoError(t, client.Write(ctx, websocket.MessageText, []byte(initial)))
			_, first, err := client.Read(ctx)
			require.NoError(t, err)
			require.Equal(t, "resp_first", gjson.GetBytes(first, "response.id").String())
			parent := "resp_external"
			if knownParent {
				parent = "resp_first"
			}
			next := `{"type":"response.create","model":"gpt-5.1","previous_response_id":"` + parent + `","input":[{"type":"function_call_output","call_id":"call_a","output":"beta"}]}`
			require.NoError(t, client.Write(ctx, websocket.MessageText, []byte(next)))
			if knownParent {
				_, final, readErr := client.Read(ctx)
				require.NoError(t, readErr)
				require.Equal(t, "resp_second", gjson.GetBytes(final, "response.id").String())
				require.NotContains(t, string(final), `"type":"function_call"`, "切换后不能再次下发历史工具动作")
				require.NoError(t, client.Close(websocket.StatusNormalClosure, "done"))
			}
			select {
			case proxyErr := <-serverErrCh:
				require.NoError(t, proxyErr)
			case <-ctx.Done():
				t.Fatal("等待原生 WS 切换验证超时")
			}
			retry := <-retryCh
			if !knownParent {
				require.Empty(t, retry, "未知父轮必须阻止 handler 退回首帧重放")
				require.Equal(t, 1, dialer.DialCount())
				return
			}
			require.Equal(t, 2, dialer.DialCount())
			require.False(t, gjson.GetBytes(retry, "previous_response_id").Exists())
			input := gjson.GetBytes(retry, "input").Array()
			require.Len(t, input, 4)
			require.Equal(t, "remember alpha", input[0].Get("content").String())
			require.Equal(t, "saved alpha", input[1].Get("content.0.text").String())
			require.Equal(t, "call_a", input[2].Get("call_id").String())
			require.Equal(t, "beta", input[3].Get("output").String())
		})
	}
}

func TestOpenAINativeWSPreambleFailoverDoesNotExposeMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	firstConn := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.created","response":{"id":"resp_bad","model":"gpt-5.1"}}`),
		[]byte(`{"type":"error","error":{"code":"rate_limit_exceeded","type":"rate_limit_error","message":"rate limit exceeded"}}`),
	}}
	secondConn := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.created","response":{"id":"resp_good","model":"gpt-5.1"}}`),
		[]byte(`{"type":"response.output_text.delta","delta":"healthy"}`),
		[]byte(`{"type":"response.completed","response":{"id":"resp_good","model":"gpt-5.1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"healthy"}]}]}}`),
	}}
	dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{firstConn, secondConn}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(dialer)
	svc := &OpenAIGatewayService{
		cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), openaiWSPool: pool,
		toolCorrector: NewCodexToolCorrector(),
	}
	account := &Account{
		ID: 9501, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key-a"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}
	nextAccount := *account
	nextAccount.ID = 9502
	nextAccount.Credentials = map[string]any{"api_key": "test-key-b"}

	serverErrCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			serverErrCh <- err
			return
		}
		defer conn.CloseNow()
		ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ginCtx.Request = r.Clone(r.Context())
		_, initial, err := conn.Read(r.Context())
		if err != nil {
			serverErrCh <- err
			return
		}
		err = svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "test-key-a", initial, nil)
		var failover *UpstreamFailoverError
		if !errors.As(err, &failover) {
			serverErrCh <- errors.New("expected native websocket preamble failover")
			return
		}
		err = svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, &nextAccount, "test-key-b", initial, nil)
		serverErrCh <- err
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer client.CloseNow()
	require.NoError(t, client.Write(ctx, websocket.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"hello"}`)))
	var received [][]byte
	for {
		_, event, readErr := client.Read(ctx)
		require.NoError(t, readErr)
		received = append(received, event)
		if gjson.GetBytes(event, "type").String() == "response.completed" {
			break
		}
	}
	require.NotEmpty(t, received)
	require.Equal(t, "response.created", gjson.GetBytes(received[0], "type").String())
	require.Equal(t, "resp_good", gjson.GetBytes(received[0], "response.id").String())
	for _, event := range received {
		require.NotContains(t, string(event), "resp_bad", "故障账号前导不得泄漏到客户端")
	}
	require.NoError(t, client.Close(websocket.StatusNormalClosure, "done"))
	select {
	case proxyErr := <-serverErrCh:
		require.NoError(t, proxyErr)
	case <-ctx.Done():
		t.Fatal("等待前导切换验证超时")
	}
	require.Equal(t, 2, dialer.DialCount())
}

func TestOpenAINativeWSPreambleFailureBoundaries(t *testing.T) {
	created := []byte(`{"type":"response.created","response":{"id":"resp_first","model":"gpt-5.1"}}`)
	limited := []byte(`{"type":"error","error":{"code":"rate_limit_exceeded","type":"rate_limit_error","message":"rate limit exceeded"}}`)
	failed := []byte(`{"type":"response.failed","response":{"id":"resp_first","error":{"code":"rate_limit_exceeded","message":"rate limit exceeded"}}}`)
	for _, tc := range []struct {
		name         string
		events       [][]byte
		delays       []time.Duration
		cancel       bool
		timeout      bool
		wantFailover bool
		wantNoOutput bool
	}{
		{name: "created_then_eof", events: [][]byte{created}, wantFailover: true, wantNoOutput: true},
		{name: "created_then_failed", events: [][]byte{created, failed}, wantFailover: true, wantNoOutput: true},
		{name: "preamble_limit", events: [][]byte{[]byte(`{"type":"response.created","padding":"` + strings.Repeat("a", openAIStreamPreambleMaxBytes) + `"}`)}, wantFailover: true, wantNoOutput: true},
		{name: "canceled_preamble", events: [][]byte{created, limited}, delays: []time.Duration{0, 2 * time.Second}, cancel: true, wantNoOutput: true},
		{name: "first_semantic_timeout", events: [][]byte{created, limited}, delays: []time.Duration{0, 2 * time.Second}, timeout: true, wantFailover: true, wantNoOutput: true},
		{name: "text_then_error_does_not_replay", events: [][]byte{created, []byte(`{"type":"response.output_text.delta","delta":"partial"}`), limited, failed}},
		{name: "tool_then_error_does_not_replay", events: [][]byte{created, []byte(`{"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_a","name":"exec","arguments":"{\"cmd\":\"pwd\"}"}}`), limited, failed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
			if tc.timeout {
				cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 1
			}
			capture := &openAIWSCaptureConn{events: tc.events, readDelays: tc.delays}
			dialer := &openAIWSCaptureDialer{conn: capture}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(dialer)
			svc := &OpenAIGatewayService{
				cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{},
				openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), openaiWSPool: pool,
				toolCorrector: NewCodexToolCorrector(),
			}
			account := &Account{
				ID: 9601, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1,
				Credentials: map[string]any{"api_key": "test-key-a", "fast_failover": true},
				Extra:       map[string]any{"responses_websockets_v2_enabled": true},
			}
			serverErrCh := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					serverErrCh <- err
					return
				}
				defer conn.CloseNow()
				ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
				controlCtx, cancel := context.WithCancel(r.Context())
				defer cancel()
				ginCtx.Request = r.Clone(controlCtx)
				_, initial, err := conn.Read(r.Context())
				if err != nil {
					serverErrCh <- err
					return
				}
				if tc.cancel {
					timer := time.AfterFunc(30*time.Millisecond, cancel)
					defer timer.Stop()
				}
				serverErrCh <- svc.ProxyResponsesWebSocketFromClient(controlCtx, ginCtx, conn, account, "test-key-a", initial, nil)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			client, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer client.CloseNow()
			require.NoError(t, client.Write(ctx, websocket.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":"hello"}`)))
			var received [][]byte
			for {
				_, event, readErr := client.Read(ctx)
				if readErr != nil {
					break
				}
				received = append(received, event)
				if isOpenAIWSTerminalEvent(gjson.GetBytes(event, "type").String()) {
					require.NoError(t, client.Close(websocket.StatusNormalClosure, "done"))
					break
				}
			}
			select {
			case proxyErr := <-serverErrCh:
				var failover *UpstreamFailoverError
				require.Equal(t, tc.wantFailover, errors.As(proxyErr, &failover))
				if tc.cancel {
					require.ErrorIs(t, proxyErr, context.Canceled)
				}
			case <-ctx.Done():
				t.Fatal("等待原生 WS 边界测试超时")
			}
			if tc.wantNoOutput {
				require.Empty(t, received, "前导未提交时不能泄漏错误账号事件")
			} else {
				require.Len(t, received, 4)
			}
			require.Equal(t, 1, dialer.DialCount(), "已取消或已提交正文时不得偷偷重做同一任务")
		})
	}
}
