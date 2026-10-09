package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCompatFirstOutputBudgetScope(t *testing.T) {
	s := &OpenAIGatewayService{cfg: &config.Config{}}
	s.cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 60
	s.cfg.Gateway.OpenAIHighEffortFirstOutputTimeoutSeconds = 120
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"fast_failover": true}}
	for _, tc := range []struct {
		name   string
		body   string
		stream bool
		want   time.Duration
	}{
		{"normal", `{"stream":true}`, true, time.Minute},
		{"high", `{"reasoning":{"effort":"high"}}`, true, 2 * time.Minute},
		{"chat_high", `{"reasoning_effort":"xhigh"}`, true, 2 * time.Minute},
		{"buffered", `{}`, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := s.withOpenAICompatFirstOutputBudget(context.Background(), account, []byte(tc.body), tc.stream)
			limit, _ := ctx.Value(openAICompatFirstOutputBudgetKey{}).(time.Duration)
			require.Equal(t, tc.want, limit)
		})
	}
	account.Type = AccountTypeOAuth
	ctx := s.withOpenAICompatFirstOutputBudget(context.Background(), account, []byte(`{}`), true)
	require.Nil(t, ctx.Value(openAICompatFirstOutputBudgetKey{}))
}

func TestCompatFirstOutputBudgetDefaultScope(t *testing.T) {
	s := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"fast_failover": true}}
	require.Equal(t, time.Minute, s.openAIFirstOutputTimeoutForAccount(account, "medium"))
	require.Equal(t, 2*time.Minute, s.openAIFirstOutputTimeoutForAccount(account, "xhigh"))
	account.Type = AccountTypeOAuth
	require.Zero(t, s.openAIFirstOutputTimeoutForAccount(account, "medium"))
	account.Type = AccountTypeAPIKey
	account.Credentials["fast_failover"] = false
	require.Zero(t, s.openAIFirstOutputTimeoutForAccount(account, "medium"))
	s.cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 90
	require.Equal(t, 90*time.Second, s.openAIFirstOutputTimeoutForAccount(account, "medium"))
}

func TestCompatFirstOutputBudgetExpiresBeforeCommit(t *testing.T) {
	ctx, budget := newOpenAICompatFirstOutputBudget(context.Background(), time.Millisecond)
	defer budget.close()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("首语义计时未触发")
	}
	resp := &http.Response{Body: &openAICompatBudgetBody{ReadCloser: io.NopCloser(strings.NewReader("late")), budget: budget}}
	require.ErrorIs(t, commitOpenAICompatFirstOutput(resp), errOpenAICompatFirstOutputTimeout)
	n, err := resp.Body.Read(make([]byte, 16))
	require.Zero(t, n)
	require.ErrorIs(t, err, errOpenAICompatFirstOutputTimeout)
}

func TestCompatFirstOutputBudgetAllowsLongCommittedStream(t *testing.T) {
	ctx, budget := newOpenAICompatFirstOutputBudget(context.Background(), 20*time.Millisecond)
	resp := &http.Response{Body: &openAICompatBudgetBody{ReadCloser: io.NopCloser(strings.NewReader("body")), budget: budget}}
	require.NoError(t, commitOpenAICompatFirstOutput(resp))
	time.Sleep(40 * time.Millisecond)
	require.NoError(t, ctx.Err())
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "body", string(body))
	require.NoError(t, resp.Body.Close())
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}

type compatBudgetUpstream struct{ HTTPUpstream }

func (*compatBudgetUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestCompatFirstOutputBudgetIncludesHeaders(t *testing.T) {
	ctx := context.WithValue(context.Background(), openAICompatFirstOutputBudgetKey{}, time.Millisecond)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://upstream.invalid/v1/chat/completions", nil)
	require.NoError(t, err)
	s := &OpenAIGatewayService{httpUpstream: &compatBudgetUpstream{}}
	resp, err := s.doOpenAIUpstream(req, "", &Account{ID: 1})
	require.Nil(t, resp)
	require.ErrorIs(t, err, errOpenAICompatFirstOutputTimeout)
}
