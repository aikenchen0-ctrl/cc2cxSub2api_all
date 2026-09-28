package admin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type modelRefreshSequenceUpstream struct {
	mu    sync.Mutex
	calls int
}

func (u *modelRefreshSequenceUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.calls++
	if u.calls <= accountModelRefreshMaxAttempts {
		return nil, errors.New("first account upstream unavailable")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"model-new"}]}`)),
	}, nil
}

func (u *modelRefreshSequenceUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

type modelRefreshAdminService struct {
	*stubAdminService
	mu       sync.Mutex
	updates  map[int64]*service.UpdateAccountInput
	accounts map[int64]*service.Account
}

func (s *modelRefreshAdminService) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account := s.accounts[id]
	if account == nil {
		return nil, errors.New("account not found")
	}
	copyAccount := *account
	copyAccount.Credentials = cloneAccountCredentialsForModelRefresh(account.Credentials)
	return &copyAccount, nil
}

func (s *modelRefreshAdminService) UpdateAccount(_ context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	s.mu.Lock()
	s.updates[id] = input
	s.mu.Unlock()
	return &service.Account{ID: id}, nil
}

func TestRunModelRefreshJobContinuesAfterFailureAndReplacesModels(t *testing.T) {
	adminSvc := &modelRefreshAdminService{
		stubAdminService: newStubAdminService(),
		updates:          make(map[int64]*service.UpdateAccountInput),
		accounts:         make(map[int64]*service.Account),
	}
	upstream := &modelRefreshSequenceUpstream{}
	accountTestSvc := service.NewAccountTestService(
		nil, nil, nil, nil, nil, upstream,
		&config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		nil,
	)
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, accountTestSvc, nil, nil, nil, nil, nil)
	accounts := []service.Account{
		{ID: 1, Name: "broken", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "one", "model_mapping": map[string]any{"old": "old"}}},
		{ID: 2, Name: "working", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "two", "model_mapping": map[string]any{"old": "old"}}},
	}
	for i := range accounts {
		account := accounts[i]
		adminSvc.accounts[account.ID] = &account
	}
	job := handler.modelRefreshJobs.create(accounts)

	handler.runModelRefreshJob(job.ID, accounts)

	snapshot, ok := handler.modelRefreshJobs.snapshot(job.ID)
	require.True(t, ok)
	require.Equal(t, "completed", snapshot.Status)
	require.Equal(t, 2, snapshot.Completed)
	require.Equal(t, 1, snapshot.Succeeded)
	require.Equal(t, 1, snapshot.Failed)
	require.Equal(t, "failed", snapshot.Items[0].Status)
	require.Equal(t, "success", snapshot.Items[1].Status)

	adminSvc.mu.Lock()
	update := adminSvc.updates[2]
	adminSvc.mu.Unlock()
	require.NotNil(t, update)
	require.Equal(t, map[string]any{"model-new": "model-new"}, update.Credentials["model_mapping"])
	require.NotContains(t, update.Credentials["model_mapping"], "old")
}
