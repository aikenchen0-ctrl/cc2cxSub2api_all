package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// 前导仅保存协议元数据；限制内存，禁止靠强制下发前导来解除积压。
const openAIStreamPreambleMaxBytes = 64 * 1024

type openAICompatStreamFailure struct {
	payload []byte
	message string
	retry   bool
}

func (e *openAICompatStreamFailure) Error() string {
	return "upstream response failed: " + e.message
}

// 命名错误和载荷错误必须在转换为普通块之前识别，防止 HTTP 200 被误记成功。
func openAICompatStreamError(payload []byte, eventType string) *openAICompatStreamFailure {
	typ := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	eventType = strings.TrimSpace(eventType)
	if !gjson.GetBytes(payload, "error").IsObject() && typ != "error" && typ != "response.failed" && eventType != "error" && eventType != "response.failed" {
		return nil
	}
	message := extractOpenAISSEErrorMessage(payload)
	retry := openAIStreamErrorEventShouldFailover(payload, message)
	if typ == "response.failed" || eventType == "response.failed" {
		retry = openAIStreamFailedEventShouldFailover(payload, message)
	}
	return &openAICompatStreamFailure{payload: append([]byte(nil), payload...), message: message, retry: retry}
}

// 仅客户端尚未收到输出且请求未取消时，才把协议或读取失败交给同组切换。
func (s *OpenAIGatewayService) compatStreamReadFailure(c *gin.Context, account *Account, resp *http.Response, upstreamModel string, cause error) error {
	if cause == nil {
		return nil
	}
	if c == nil || c.Writer == nil || c.Writer.Written() || (c.Request != nil && c.Request.Context().Err() != nil) || errors.Is(cause, context.Canceled) {
		return cause
	}
	var failure *openAICompatStreamFailure
	if errors.As(cause, &failure) {
		if !failure.retry {
			return cause
		}
		return s.newOpenAIStreamFailoverErrorWithModel(c, account, false, resp.Header.Get("x-request-id"), failure.payload, failure.message, upstreamModel, resp.Header)
	}
	payload, _ := json.Marshal(gin.H{"error": gin.H{
		"type": "upstream_error", "code": "upstream_stream_incomplete", "message": cause.Error(),
	}})
	return s.newOpenAIStreamFailoverErrorWithModel(c, account, false, resp.Header.Get("x-request-id"), payload, cause.Error(), upstreamModel, resp.Header)
}

// 无输出的非重试错误按客户端协议返回；已提交的流由现有处理器终止，不合成成功。
func writeCCStreamNonRetryableFailure(c *gin.Context, err error, writeError compatErrorWriter) {
	var failure *openAICompatStreamFailure
	var failover *UpstreamFailoverError
	if c == nil || c.Writer == nil || c.Writer.Written() || errors.As(err, &failover) || !errors.As(err, &failure) {
		return
	}
	status := openAIStreamFailureStatus(failure.payload, failure.message)
	if status < 400 {
		status = http.StatusBadGateway
	}
	writeError(c, status, "upstream_error", failure.message)
}

func openAIStreamPreambleLimitError() error {
	return fmt.Errorf("upstream stream preamble exceeds %d bytes", openAIStreamPreambleMaxBytes)
}

// HTTP 200 只能证明传输成功；畸形正文和失败结束标记不能作为正常完成下发。
// 兼容上游可以省略 finish_reason，但非流式响应必须实际包含 message。
func validateOpenAIChatResponse(payload []byte, stream bool) error {
	if !gjson.ValidBytes(payload) || !gjson.ParseBytes(payload).IsObject() {
		return errors.New("invalid upstream chat response JSON")
	}
	choices := gjson.GetBytes(payload, "choices")
	if !stream && (!choices.IsArray() || len(choices.Array()) == 0) {
		return errors.New("upstream chat response contains no choices")
	}
	for _, choice := range choices.Array() {
		switch strings.ToLower(strings.TrimSpace(choice.Get("finish_reason").String())) {
		case "error", "failed":
			return errors.New("upstream chat response has a failed finish reason")
		}
		if !stream && !choice.Get("message").IsObject() {
			return errors.New("upstream chat response contains no message")
		}
	}
	return nil
}
