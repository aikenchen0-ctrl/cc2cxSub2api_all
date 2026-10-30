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

func (s *modelRefreshAdminService) UpdateAccount(_ context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	if id == s.failSave {
		return nil, errors.New("save failed")
	}
	s.mu.Lock()
	s.updates[id] = input
	s.mu.Unlock()
	return &service.Account{ID: id}, nil
}

func (s *modelRefreshAdminService) updateFor(id int64) *service.UpdateAccountInput {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updates[id]
}

// newModelRefreshHandler builds a handler whose upstream sync is stubbed per account.
func newModelRefreshHandler(t *testing.T, svc *modelRefreshAdminService, sync func(context.Context, *service.Account) (*service.UpstreamModelCatalog, error)) *AccountHandler {
	t.Helper()
	h := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetModelCatalogSync(sync)
	return h
}

func catalogOf(models ...string) *service.UpstreamModelCatalog {
	return &service.UpstreamModelCatalog{Models: models}
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

// The bulk job must reproduce "Edit -> Clear all models -> Sync upstream models":
// the stale whitelist disappears and only the live upstream list survives.
func TestRunModelRefreshJobClearsWhitelistAndUsesLiveUpstreamModels(t *testing.T) {
	svc := &modelRefreshAdminService{
		stubAdminService: newStubAdminService(),
		updates:          make(map[int64]*service.UpdateAccountInput),
		accounts:         make(map[int64]*service.Account),
	}
	stale := service.Account{
		ID:       1,
		Name:     "stale",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":       "secret-key",
			"base_url":      "https://upstream.example.com",
			"model_mapping": map[string]any{"retired-model": "retired-model", "gpt-4o": "gpt-4o"},
		},
	}
	svc.accounts[1] = &stale
	accounts := []service.Account{stale}

	h := newModelRefreshHandler(t, svc, func(_ context.Context, account *service.Account) (*service.UpstreamModelCatalog, error) {
		return catalogOf("gpt-5", "gpt-5-mini", "gpt-5"), nil
	})
	job := h.modelRefreshJobs.create(accounts)
	h.runModelRefreshJob(job.ID, accounts)

	snapshot, ok := h.modelRefreshJobs.snapshot(job.ID)
	require.True(t, ok)
	require.Equal(t, "completed", snapshot.Status)
	require.Equal(t, 1, snapshot.Succeeded)
	require.Equal(t, "success", snapshot.Items[0].Status)

	update := svc.updateFor(1)
	require.NotNil(t, update)
	mapping := update.Credentials["model_mapping"].(map[string]any)
	require.Equal(t, map[string]any{"gpt-5": "gpt-5", "gpt-5-mini": "gpt-5-mini"}, mapping)
	require.NotContains(t, mapping, "retired-model", "the stale whitelist must be cleared, not merged")
	// Unrelated credentials must survive the write untouched.
	require.Equal(t, "secret-key", update.Credentials["api_key"])
	require.Equal(t, "https://upstream.example.com", update.Credentials["base_url"])
	// The reloaded account is a copy, so the listed snapshot must stay untouched.
	require.Contains(t, stale.Credentials["model_mapping"], "retired-model")
}

// "Clear all models" only empties the whitelist tab; custom aliases live in the
// mapping tab and survive. A custom alias also wins over an identity entry.
func TestRunModelRefreshJobKeepsCustomAliasesAndLetsThemWin(t *testing.T) {
	svc := &modelRefreshAdminService{
		stubAdminService: newStubAdminService(),
		updates:          make(map[int64]*service.UpdateAccountInput),
		accounts:         make(map[int64]*service.Account),
	}
	account := service.Account{
		ID:       7,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "k",
			"model_mapping": map[string]any{
				"my-alias":  "gpt-5",
				"gpt-5":     "gpt-4o",
				"obsolete":  "obsolete",
				"bad-alias": "gpt-*",
			},
		},
	}
	svc.accounts[7] = &account
	accounts := []service.Account{account}

	h := newModelRefreshHandler(t, svc, func(context.Context, *service.Account) (*service.UpstreamModelCatalog, error) {
		return catalogOf("gpt-5", "gpt-6"), nil
	})
	job := h.modelRefreshJobs.create(accounts)
	h.runModelRefreshJob(job.ID, accounts)

	mapping := svc.updateFor(7).Credentials["model_mapping"].(map[string]any)
	require.Equal(t, "gpt-5", mapping["my-alias"], "custom aliases survive clear-all")
	require.Equal(t, "gpt-4o", mapping["gpt-5"], "a custom alias wins over the identity entry")
	require.Equal(t, "gpt-6", mapping["gpt-6"])
	require.NotContains(t, mapping, "obsolete", "stale identity entries are cleared")
	require.NotContains(t, mapping, "bad-alias", "wildcard targets cannot be persisted")
}

// A failed upstream call must leave the account exactly as it was: closing the
// editor without saving is the manual equivalent.
func TestRunModelRefreshJobLeavesAccountUntouchedWhenUpstreamFails(t *testing.T) {
	svc := &modelRefreshAdminService{
		stubAdminService: newStubAdminService(),
		updates:          make(map[int64]*service.UpdateAccountInput),
		accounts:         make(map[int64]*service.Account),
	}
	account := service.Account{
		ID:          2,
		Platform:    service.PlatformAnthropic,
		Credentials: map[string]any{"model_mapping": map[string]any{"keep-me": "keep-me"}},
	}
	svc.accounts[2] = &account
	accounts := []service.Account{account}

	h := newModelRefreshHandler(t, svc, func(context.Context, *service.Account) (*service.UpstreamModelCatalog, error) {
		return nil, &service.UpstreamModelSyncError{Kind: service.UpstreamModelSyncErrorUpstream, Message: "upstream rejected the request"}
	})
	job := h.modelRefreshJobs.create(accounts)
	h.runModelRefreshJob(job.ID, accounts)

	snapshot, _ := h.modelRefreshJobs.snapshot(job.ID)
	require.Equal(t, 1, snapshot.Failed)
	require.Equal(t, "failed", snapshot.Items[0].Status)
	require.Equal(t, "upstream rejected the request", snapshot.Items[0].Message)
	require.Nil(t, svc.updateFor(2), "a failed sync must not persist anything")
}

func TestRunModelRefreshJobFailsWhenUpstreamReturnsNoModels(t *testing.T) {
	svc := &modelRefreshAdminService{
		stubAdminService: newStubAdminService(),
		updates:          make(map[int64]*service.UpdateAccountInput),
		accounts:         make(map[int64]*service.Account),
	}
	account := service.Account{ID: 3, Platform: service.PlatformGemini, Credentials: map[string]any{}}
	svc.accounts[3] = &account
	accounts := []service.Account{account}

	h := newModelRefreshHandler(t, svc, func(context.Context, *service.Account) (*service.UpstreamModelCatalog, error) {
		return catalogOf(), nil
	})
	job := h.modelRefreshJobs.create(accounts)
	h.runModelRefreshJob(job.ID, accounts)

	snapshot, _ := h.modelRefreshJobs.snapshot(job.ID)
	require.Equal(t, 1, snapshot.Failed)
	require.Nil(t, svc.updateFor(3), "an empty upstream list must not wipe the mapping")
}

// Passthrough and shadow accounts do not use a model whitelist at all, so the
// job reports them as skipped instead of writing a meaningless mapping.
func TestRunModelRefreshJobSkipsAccountsWithoutModelRestriction(t *testing.T) {
	svc := &modelRefreshAdminService{
		stubAdminService: newStubAdminService(),
		updates:          make(map[int64]*service.UpdateAccountInput),
		accounts:         make(map[int64]*service.Account),
	}
	parentID := int64(100)
	passthrough := service.Account{
		ID:          4,
		Platform:    service.PlatformOpenAI,
		Credentials: map[string]any{"api_key": "k"},
		Extra:       map[string]any{"openai_passthrough": true},
	}
	shadow := service.Account{
		ID:              5,
		Platform:        service.PlatformOpenAI,
		ParentAccountID: &parentID,
		Credentials:     map[string]any{},
	}
	svc.accounts[4] = &passthrough
	svc.accounts[5] = &shadow
	accounts := []service.Account{passthrough, shadow}

	syncCalls := 0
	h := newModelRefreshHandler(t, svc, func(context.Context, *service.Account) (*service.UpstreamModelCatalog, error) {
		syncCalls++
		return catalogOf("gpt-5"), nil
	})
	job := h.modelRefreshJobs.create(accounts)
	h.runModelRefreshJob(job.ID, accounts)

	snapshot, _ := h.modelRefreshJobs.snapshot(job.ID)
	require.Equal(t, 2, snapshot.Skipped)
	require.Zero(t, snapshot.Succeeded)
	require.Zero(t, snapshot.Failed)
	require.Equal(t, "skipped", snapshot.Items[0].Status)
	require.NotEmpty(t, snapshot.Items[0].Message)
	require.Zero(t, syncCalls, "skipped accounts must not hit the upstream")
	require.Nil(t, svc.updateFor(4))
	require.Nil(t, svc.updateFor(5))
}

func TestRunModelRefreshJobMigratesLegacyAntigravityWhitelistAndHandlesEmptyJobs(t *testing.T) {
	svc := &modelRefreshAdminService{
		stubAdminService: newStubAdminService(),
		updates:          make(map[int64]*service.UpdateAccountInput),
		accounts:         make(map[int64]*service.Account),
	}
	account := service.Account{ID: 1, Platform: service.PlatformAntigravity, Credentials: map[string]any{
		"model_whitelist": []any{"legacy-model"},
		"access_token":    "preserved",
	}}
	svc.accounts[1] = &account
	accounts := []service.Account{account}

	h := newModelRefreshHandler(t, svc, func(context.Context, *service.Account) (*service.UpstreamModelCatalog, error) {
		return catalogOf("  ", "gemini-3-flash", "gemini-3-pro", "gemini-3-flash"), nil
	})
	job := h.modelRefreshJobs.create(accounts)
	h.runModelRefreshJob(job.ID, accounts)

	updated := svc.updateFor(1).Credentials
	mapping := updated["model_mapping"].(map[string]any)
	require.Equal(t, map[string]any{
		"gemini-3-flash": "gemini-3-flash",
		"gemini-3-pro":   "gemini-3-pro",
	}, mapping, "blank entries are dropped, duplicates collapse, and the legacy whitelist is replaced")
	require.Equal(t, "preserved", updated["access_token"])
	require.NotContains(t, updated, "model_whitelist", "the editor drops the retired whitelist field")
	require.Contains(t, account.Credentials, "model_whitelist", "the source account is untouched")

	empty := h.modelRefreshJobs.create(nil)
	h.runModelRefreshJob(empty.ID, nil)
	finished, ok := h.modelRefreshJobs.snapshot(empty.ID)
	require.True(t, ok)
	require.Equal(t, "completed", finished.Status)
	require.Zero(t, finished.Total)
	_, active := h.modelRefreshJobs.activeSnapshot()
	require.False(t, active)
}

func TestStartModelRefreshJobSurvivesRequestCancellationAndSharesConcurrentStarts(t *testing.T) {
	s := &modelRefreshAdminService{stubAdminService: newStubAdminService(), updates: map[int64]*service.UpdateAccountInput{}, accounts: map[int64]*service.Account{}}
	account := service.Account{ID: 1, Platform: service.PlatformOpenAI, Credentials: map[string]any{}}
	s.accounts[1] = &account
	s.stubAdminService.accounts = []service.Account{account}
	h := NewAccountHandler(s, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetModelCatalogSync(func(context.Context, *service.Account) (*service.UpstreamModelCatalog, error) {
		return catalogOf("gpt-5"), nil
	})
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
	require.Eventually(t, func() bool {
		j, _ := h.modelRefreshJobs.snapshot(first)
		return j.Status == "completed"
	}, time.Second, 10*time.Millisecond)
	job, _ := h.modelRefreshJobs.snapshot(first)
	require.Equal(t, 1, job.Succeeded)
}

func TestStartModelRefreshJobRequiresUpstreamSync(t *testing.T) {
	s := &modelRefreshAdminService{stubAdminService: newStubAdminService(), updates: map[int64]*service.UpdateAccountInput{}, accounts: map[int64]*service.Account{}}
	h := NewAccountHandler(s, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/models/refresh-all", nil)
	h.StartModelRefreshJob(c)
	require.Equal(t, 500, w.Code, "without an upstream sync the job must refuse to run")
}
