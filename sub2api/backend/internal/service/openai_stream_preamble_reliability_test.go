//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func runPreambleTestStream(t *testing.T, route, stream string, requestSize int, canceled bool) (*httptest.ResponseRecorder, *gin.Context, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if canceled {
		ctx, cancel := context.WithCancel(c.Request.Context())
		cancel()
		c.Request = c.Request.WithContext(ctx)
	}
	err := invokePreambleTestStream(route, stream, requestSize, c)
	return rec, c, err
}

func invokePreambleTestStream(route, stream string, requestSize int, c *gin.Context) error {
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
	account := rawChatCompletionsTestAccount()
	var err error
	switch route {
	case "raw_chat":
		_, err = svc.streamRawChatCompletions(c, resp, account, "test-model", "test-model", "test-model", nil, nil, time.Now(), requestSize)
	case "responses_chat":
		_, err = svc.handleChatStreamingResponse(resp, c, account, "test-model", "test-model", "test-model", time.Now(), requestSize)
	case "responses_messages":
		_, err = svc.handleAnthropicStreamingResponse(resp, c, account, "test-model", "test-model", "test-model", time.Now())
	case "chat_responses":
		_, err = svc.streamChatCompletionsAsResponses(c, resp, account, "test-model", nil, nil, false, nil, "test-model", "test-model", nil, nil, time.Now())
	case "chat_messages":
		_, err = svc.streamChatCompletionsAsAnthropic(c, resp, account, "test-model", "test-model", "test-model", nil, nil, time.Now())
	default:
		return fmt.Errorf("unknown route %s", route)
	}
	return err
}

func preambleFixture(route string) (string, string, string) {
	if strings.HasPrefix(route, "responses_") {
		return "data: {\"type\":\"response.created\",\"response\":{\"id\":\"failed-attempt\",\"model\":\"test-model\",\"status\":\"in_progress\",\"output\":[]}}\n\n" +
				"data: {\"type\":\"response.in_progress\"}\n\n",
			"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"partial\"}\n\n",
			"data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"fc1\",\"call_id\":\"call1\",\"name\":\"send\",\"arguments\":\"{}\"}}\n\n"
	}
	return "data: {\"id\":\"failed-attempt\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\",\"reasoning_content\":\"\"}}]}\n\n",
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n",
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call1\",\"type\":\"function\",\"function\":{\"name\":\"send\",\"arguments\":\"{}\"}}]}}]}\n\n"
}

// 任一兼容路径在只有前导时均可切换，正文或工具已提交后不得重放。
func TestStreamPreambleReliability_AllRoutes(t *testing.T) {
	for _, route := range []string{"raw_chat", "responses_chat", "responses_messages", "chat_responses", "chat_messages"} {
		for _, size := range []int{32, 70 * 1024} {
			for _, tc := range []struct {
				name       string
				before     string
				errorFrame string
				failover   bool
			}{
				{name: "bare_overload", errorFrame: `data: {"type":"error","error":{"code":"server_is_overloaded","message":"try again later"}}` + "\n\n", failover: true},
				{name: "named_overload", errorFrame: "event: error\ndata: {\"error\":{\"type\":\"server_error\",\"message\":\"temporary failure\"}}\n\n", failover: true},
				{name: "failed_event", errorFrame: `data: {"type":"response.failed","response":{"error":{"code":"server_error","message":"try again later"}}}` + "\n\n", failover: true},
				{name: "clean_eof", failover: true},
				{name: "after_text", before: "text", errorFrame: `data: {"type":"error","error":{"code":"server_is_overloaded","message":"try again later"}}` + "\n\n"},
				{name: "after_tool", before: "tool", errorFrame: `data: {"type":"error","error":{"code":"server_is_overloaded","message":"try again later"}}` + "\n\n"},
				{name: "invalid_input", errorFrame: `data: {"type":"error","error":{"code":"context_length_exceeded","message":"input exceeds the context window"}}` + "\n\n"},
			} {
				t.Run(route+"/"+tc.name+"/"+time.Duration(size).String(), func(t *testing.T) {
					preamble, text, tool := preambleFixture(route)
					if tc.before == "text" {
						preamble += text
					}
					if tc.before == "tool" {
						preamble += tool
					}
					rec, c, err := runPreambleTestStream(t, route, preamble+tc.errorFrame, size, false)
					require.Error(t, err)
					var failover *UpstreamFailoverError
					require.Equal(t, tc.failover, errors.As(err, &failover), "%T: %v", err, err)
					if tc.failover {
						require.False(t, c.Writer.Written())
						require.Empty(t, rec.Body.String())
					} else if tc.before != "" {
						require.True(t, c.Writer.Written())
					}
					require.NotContains(t, rec.Body.String(), "response.completed")
					require.NotContains(t, rec.Body.String(), "message_stop")
					require.NotContains(t, rec.Body.String(), "[DONE]")
				})
			}
		}
	}
}

func TestStreamPreambleReliability_CanceledDoesNotRetry(t *testing.T) {
	for _, route := range []string{"raw_chat", "responses_chat", "responses_messages", "chat_responses", "chat_messages"} {
		t.Run(route, func(t *testing.T) {
			preamble, _, _ := preambleFixture(route)
			_, _, err := runPreambleTestStream(t, route, preamble, 32, true)
			require.Error(t, err)
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
		})
	}
}

func TestStreamPreambleReliability_JSONErrorDoesNotBecomeSuccess(t *testing.T) {
	for _, route := range []string{"raw_chat", "responses", "messages"} {
		for _, retry := range []bool{true, false} {
			t.Run(route+"/"+map[bool]string{true: "retry", false: "invalid"}[retry], func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+route, nil)
				payload := `{"error":{"code":"context_length_exceeded","message":"input exceeds the context window"}}`
				if retry {
					payload = `{"error":{"code":"server_is_overloaded","message":"try again later"}}`
				}
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(payload))}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
				var err error
				if route == "raw_chat" {
					_, err = svc.bufferRawChatCompletions(c, resp, rawChatCompletionsTestAccount(), "test-model", "test-model", "test-model", nil, nil, time.Now())
				} else if route == "responses" {
					_, err = svc.bufferChatCompletionsAsResponses(c, resp, rawChatCompletionsTestAccount(), "test-model", nil, nil, false, nil, "test-model", "test-model", nil, nil, time.Now())
				} else {
					_, err = svc.bufferChatCompletionsAsAnthropic(c, resp, rawChatCompletionsTestAccount(), "test-model", "test-model", "test-model", nil, nil, time.Now())
				}
				require.Error(t, err)
				var failover *UpstreamFailoverError
				require.Equal(t, retry, errors.As(err, &failover))
				if retry {
					require.False(t, c.Writer.Written())
				} else {
					require.GreaterOrEqual(t, rec.Code, 400)
				}
				require.NotContains(t, rec.Body.String(), `"status":"completed"`)
			})
		}
	}
}

func TestStreamPreambleReliability_UsageOnRoleIsNotTerminal(t *testing.T) {
	for _, route := range []string{"raw_chat", "chat_responses", "chat_messages"} {
		t.Run(route, func(t *testing.T) {
			stream := "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":0}}\n\n"
			rec, c, err := runPreambleTestStream(t, route, stream, 32, false)
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.False(t, c.Writer.Written())
			require.Empty(t, rec.Body.String())
		})
	}
}

func TestStreamPreambleReliability_MalformedObjectDoesNotBecomeSuccess(t *testing.T) {
	for _, route := range []string{"raw_chat", "chat_responses", "chat_messages"} {
		for _, payload := range []string{
			`{"choices":broken}`,
			`{"choices":[{"delta":{},"finish_reason":"error"}]}`,
		} {
			t.Run(route+"/"+payload, func(t *testing.T) {
				stream := "data: " + payload + "\n\ndata: [DONE]\n\n"
				rec, c, err := runPreambleTestStream(t, route, stream, 32, false)
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.False(t, c.Writer.Written())
				require.Empty(t, rec.Body.String())
			})
		}
	}
}

func TestStreamPreambleReliability_JSONAnomalies(t *testing.T) {
	for _, route := range []string{"raw_chat", "chat_responses", "chat_messages"} {
		for _, tc := range []struct {
			name string
			body string
		}{
			{"error", `{"error":{"code":"server_is_overloaded","message":"try again later"}}`},
			{"malformed", `{"choices":broken}`},
			{"usage_only", `{"usage":{"prompt_tokens":3,"completion_tokens":0}}`},
			{"failed_finish", `{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":"error"}]}`},
		} {
			t.Run(route+"/"+tc.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
				var err error
				switch route {
				case "raw_chat":
					_, err = svc.bufferRawChatCompletions(c, resp, rawChatCompletionsTestAccount(), "test-model", "test-model", "test-model", nil, nil, time.Now())
				case "chat_responses":
					_, err = svc.bufferChatCompletionsAsResponses(c, resp, rawChatCompletionsTestAccount(), "test-model", nil, nil, false, nil, "test-model", "test-model", nil, nil, time.Now())
				case "chat_messages":
					_, err = svc.bufferChatCompletionsAsAnthropic(c, resp, rawChatCompletionsTestAccount(), "test-model", "test-model", "test-model", nil, nil, time.Now())
				}
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.False(t, c.Writer.Written())
				require.Empty(t, rec.Body.String())
			})
		}
	}
}

func healthyPreambleStream(route string) string {
	preamble, text, _ := preambleFixture(route)
	stream := strings.ReplaceAll(preamble, "failed-attempt", "healthy-attempt") + strings.ReplaceAll(text, "partial", "healthy-output")
	if strings.HasPrefix(route, "responses_") {
		return stream + "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"healthy-attempt\",\"model\":\"test-model\",\"status\":\"completed\",\"output\":[{\"id\":\"msg1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"healthy-output\"}]}],\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n"
	}
	return stream + "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2}}\n\n" + "data: [DONE]\n\n"
}

// 500 个并发客户混合五条实际适配路径，失败候选与健康候选共用同一客户端响应。
// 此测试仅模拟上游数据，不访问网络、不执行模型调用，也不替代调度器集成测试。
func TestStreamPreambleReliability_Concurrent500Failover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const count = 500
	routes := []string{"raw_chat", "responses_chat", "responses_messages", "chat_responses", "chat_messages"}
	assignments := make([]string, count)
	for i := range assignments {
		assignments[i] = routes[i%len(routes)]
	}
	rand.New(rand.NewSource(47)).Shuffle(count, func(i, j int) {
		assignments[i], assignments[j] = assignments[j], assignments[i]
	})
	baseline := runtime.NumGoroutine()
	start := make(chan struct{})
	failures := make(chan error, count)
	var ready, finished sync.WaitGroup
	var completed, attempts atomic.Int64
	ready.Add(count)
	finished.Add(count)
	for i, route := range assignments {
		go func(index int, route string) {
			defer finished.Done()
			defer func() {
				if failure := recover(); failure != nil {
					failures <- fmt.Errorf("%s request %d panicked: %v", route, index, failure)
				}
			}()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			ready.Done()
			<-start
			preamble, _, _ := preambleFixture(route)
			if index%2 == 0 {
				preamble += "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"server_error\",\"code\":\"503\",\"message\":\"temporary unavailable, try again later\"}}\n\n"
			}
			attempts.Add(1)
			err := invokePreambleTestStream(route, preamble, 32, c)
			var failover *UpstreamFailoverError
			if !errors.As(err, &failover) || c.Writer.Written() || rec.Body.Len() != 0 {
				failures <- fmt.Errorf("%s request %d did not preserve the retry window: %v", route, index, err)
				return
			}
			attempts.Add(1)
			if err := invokePreambleTestStream(route, healthyPreambleStream(route), 32, c); err != nil {
				failures <- fmt.Errorf("%s request %d healthy attempt failed: %w", route, index, err)
				return
			}
			body := rec.Body.String()
			if !strings.Contains(body, "healthy-output") || strings.Contains(body, "failed-attempt") || strings.Contains(body, "temporary unavailable") {
				failures <- fmt.Errorf("%s request %d leaked the failed attempt or lost healthy output", route, index)
				return
			}
			completed.Add(1)
		}(i, route)
	}
	ready.Wait()
	started := time.Now()
	close(start)
	finished.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	require.EqualValues(t, count, completed.Load())
	require.EqualValues(t, count*2, attempts.Load())
	require.LessOrEqual(t, runtime.NumGoroutine(), baseline+16)
	t.Logf("并发请求=%d，恢复成功=%d，尝试总数=%d，耗时=%s，测试后协程差=%d", count, completed.Load(), attempts.Load(), time.Since(started), runtime.NumGoroutine()-baseline)
}
