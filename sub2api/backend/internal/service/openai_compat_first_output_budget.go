package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"
)

var errOpenAICompatFirstOutputTimeout = errors.New("upstream produced no semantic output before the deadline")

type openAICompatFirstOutputBudgetKey struct{}

// 保留已有全局配置；未配置时，仅快速切换账号采用分级首语义期限。
// 高推理请求允许更长计算时间，其他账号不会因本次灰度获得新超时。
func (s *OpenAIGatewayService) openAIFirstOutputTimeoutForAccount(account *Account, effort string) time.Duration {
	if configured := s.openAIFirstOutputTimeout(effort); configured > 0 {
		return configured
	}
	if !account.ShouldFailoverImmediately(http.StatusBadGateway) {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "high", "xhigh", "max":
		return 120 * time.Second
	default:
		return 60 * time.Second
	}
}

// 兼容接口只在显式快速切换账号、流式客户端上启用首语义计时。
// 计时覆盖响应头和空前导，正常正文开始后解除，避免截断长回答。
func (s *OpenAIGatewayService) withOpenAICompatFirstOutputBudget(ctx context.Context, account *Account, body []byte, stream bool) context.Context {
	if !stream || !account.ShouldFailoverImmediately(http.StatusBadGateway) {
		return ctx
	}
	effort := strings.TrimSpace(gjson.GetBytes(body, "reasoning.effort").String())
	if effort == "" {
		effort = strings.TrimSpace(gjson.GetBytes(body, "reasoning_effort").String())
	}
	limit := s.openAIFirstOutputTimeoutForAccount(account, effort)
	if limit <= 0 {
		return ctx
	}
	return context.WithValue(ctx, openAICompatFirstOutputBudgetKey{}, limit)
}

type openAICompatFirstOutputBudget struct {
	// 0 等待，1 已提交，2 已超时。提交与超时只能有一方成功。
	state  atomic.Int32
	timer  *time.Timer
	cancel context.CancelFunc
	once   sync.Once
}

func newOpenAICompatFirstOutputBudget(ctx context.Context, limit time.Duration) (context.Context, *openAICompatFirstOutputBudget) {
	ctx, cancel := context.WithCancel(ctx)
	budget := &openAICompatFirstOutputBudget{cancel: cancel}
	budget.timer = time.AfterFunc(limit, func() {
		if budget.state.CompareAndSwap(0, 2) {
			cancel()
		}
	})
	return ctx, budget
}

func (b *openAICompatFirstOutputBudget) commit() error {
	if b == nil {
		return nil
	}
	if b.state.CompareAndSwap(0, 1) {
		b.timer.Stop()
		return nil
	}
	if b.state.Load() == 2 {
		return errOpenAICompatFirstOutputTimeout
	}
	return nil
}

func (b *openAICompatFirstOutputBudget) close() {
	if b == nil {
		return
	}
	b.once.Do(func() {
		b.timer.Stop()
		b.state.CompareAndSwap(0, 1)
		b.cancel()
	})
}

type openAICompatBudgetBody struct {
	io.ReadCloser
	budget *openAICompatFirstOutputBudget
}

func (r *openAICompatBudgetBody) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if r.budget.state.Load() == 2 {
		return 0, errOpenAICompatFirstOutputTimeout
	}
	return n, err
}

func (r *openAICompatBudgetBody) Close() error {
	r.budget.close()
	return r.ReadCloser.Close()
}

// 仅在将正文、工具结构或合法终态提交给客户端之前调用。
// 前导块及心跳不能解除计时；超过期限的迟到正文不得再提交。
func commitOpenAICompatFirstOutput(resp *http.Response) error {
	if resp == nil {
		return nil
	}
	if body, ok := resp.Body.(*openAICompatBudgetBody); ok {
		return body.budget.commit()
	}
	return nil
}
