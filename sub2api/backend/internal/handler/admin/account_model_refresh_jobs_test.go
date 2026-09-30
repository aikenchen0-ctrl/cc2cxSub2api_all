package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type modelRefreshAdminService struct {
	*stubAdminService
	mu       sync.Mutex
	updates  map[int64]*service.UpdateAccountInput
	accounts map[int64]*service.Account
	failSave int64
	gate     <-chan struct{}
}

func (s *modelRefreshAdminService) GetAccount(ctx context.Context, id int64) (*service.Account, error) {
	if s.gate != nil {
		<-s.gate
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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

func TestListAllAccountsForModelRefreshIncludesEveryPageAndStatus(t *testing.T) {
	s := newStubAdminService()
	s.accounts = nil
	for i := accountModelRefreshPageSize + 1; i > 0; i-- {
		s.accounts = append(s.accounts, service.Account{ID: int64(i), Status: "error"})
	}
	h := NewAccountHandler(s, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	got, err := h.listAllAccountsForModelRefresh(context.Background())
	require.NoError(t, err)
	require.Len(t, got, accountModelRefreshPageSize+1)
	require.Equal(t, int64(1), got[0].ID)
	require.Equal(t, 2, s.lastListAccounts.calls)
	require.Empty(t, s.lastListAccounts.status)
}

func TestStartModelRefreshJobSurvivesRequestCancellationAndSharesConcurrentStarts(t *testing.T) {
	s := &modelRefreshAdminService{stubAdminService: newStubAdminService(), updates: map[int64]*service.UpdateAccountInput{}, accounts: map[int64]*service.Account{}}
	account := service.Account{ID: 1, Platform: "openai", Credentials: map[string]any{}}
	s.accounts[1] = &account
	s.stubAdminService.accounts = []service.Account{account}
	h := NewAccountHandler(s, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	gate := make(chan struct{})
	s.gate = gate
	defer func() {
		if gate != nil {
			close(gate)
		}
	}()
	const count = 12
	ids := make(chan string, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			ctx, cancel := context.WithCancel(context.Background())
			c.Request = httptest.NewRequest("POST", "/models/refresh-all", nil).WithContext(ctx)
			h.StartModelRefreshJob(c)
			cancel()
			var body struct {
				Data accountModelRefreshJob `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Error(err)
			}
			ids <- body.Data.ID
		}()
	}
	wg.Wait()
	close(ids)
	first := ""
	for id := range ids {
		require.NotEmpty(t, id)
		if first == "" {
			first = id
		}
		require.Equal(t, first, id)
	}
	close(gate)
	gate = nil
	require.Eventually(t, func() bool { j, _ := h.modelRefreshJobs.snapshot(first); return j.Status == "completed" }, time.Second, 10*time.Millisecond)
	job, _ := h.modelRefreshJobs.snapshot(first)
	require.Equal(t, 1, job.Succeeded)
}

func (s *modelRefreshAdminService) UpdateAccount(_ context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	if id == s.failSave {
		return nil, errors.New("save failed")
	}
	s.mu.Lock()
	s.updates[id] = input
	s.mu.Unlock()
	return &service.Account{ID: id}, nil
}

func TestRunModelRefreshJobContinuesAfterFailureAndPreservesMappings(t *testing.T) {
	adminSvc := &modelRefreshAdminService{
		stubAdminService: newStubAdminService(),
		updates:          make(map[int64]*service.UpdateAccountInput),
		accounts:         make(map[int64]*service.Account),
	}
	adminSvc.failSave = 1
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	accounts := []service.Account{
		{ID: 1, Name: "broken", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "one", "model_mapping": map[string]any{"old": "old", "gpt-5.5": "custom-target"}}},
		{ID: 2, Name: "working", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "two", "model_mapping": map[string]any{"old": "old", "gpt-5.5": "custom-target"}}},
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
	mapping := update.Credentials["model_mapping"].(map[string]any)
	require.Equal(t, "old", mapping["old"])
	require.Equal(t, "custom-target", mapping["gpt-5.5"])
	require.Equal(t, "gpt-6", mapping["gpt-6"])
	require.Equal(t, "two", update.Credentials["api_key"])
	require.Len(t, accounts[1].Credentials["model_mapping"], 2)
	require.NotContains(t, adminSvc.updates, int64(1))
}

func TestRunModelRefreshJobMigratesLegacyWhitelistAndHandlesEmptyJobs(t *testing.T) {
	account := service.Account{ID: 1, Platform: "antigravity", Credentials: map[string]any{
		"model_whitelist": []any{" custom-model ", "", "legacy-model"},
		"access_token":    "preserved",
	}}
	s := &modelRefreshAdminService{stubAdminService: newStubAdminService(),
		updates: map[int64]*service.UpdateAccountInput{}, accounts: map[int64]*service.Account{1: &account}}
	h := NewAccountHandler(s, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	job := h.modelRefreshJobs.create([]service.Account{account})
	h.runModelRefreshJob(job.ID, []service.Account{account})
	updated := s.updates[1].Credentials
	mapping := updated["model_mapping"].(map[string]any)
	require.Equal(t, "custom-model", mapping["custom-model"])
	require.Equal(t, "legacy-model", mapping["legacy-model"])
	require.Equal(t, "preserved", updated["access_token"])
	require.NotContains(t, updated, "model_whitelist")
	require.Contains(t, account.Credentials, "model_whitelist")
	empty := h.modelRefreshJobs.create(nil)
	h.runModelRefreshJob(empty.ID, nil)
	finished, ok := h.modelRefreshJobs.snapshot(empty.ID)
	require.True(t, ok)
	require.Equal(t, "completed", finished.Status)
	require.Zero(t, finished.Total)
	_, active := h.modelRefreshJobs.activeSnapshot()
	require.False(t, active)
}
