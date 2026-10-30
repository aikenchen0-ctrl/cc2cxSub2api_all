package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const accountModelRefreshPageSize = 500

const (
	accountModelRefreshStatusPending    = "pending"
	accountModelRefreshStatusRefreshing = "refreshing"
	accountModelRefreshStatusSuccess    = "success"
	accountModelRefreshStatusFailed     = "failed"
	// accountModelRefreshStatusSkipped marks accounts the manual flow cannot be
	// applied to (credential shadows, OpenAI passthrough). They are left untouched.
	accountModelRefreshStatusSkipped = "skipped"
)

type accountModelRefreshItem struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	UpstreamURL string `json:"upstream_url"`
	Status      string `json:"status"`
	ModelCount  int    `json:"model_count,omitempty"`
	// Message explains a skip or carries the safe upstream failure reason.
	Message string `json:"message,omitempty"`
}

type accountModelRefreshJob struct {
	ID          string                    `json:"id"`
	Status      string                    `json:"status"`
	Total       int                       `json:"total"`
	Completed   int                       `json:"completed"`
	Succeeded   int                       `json:"succeeded"`
	Failed      int                       `json:"failed"`
	Skipped     int                       `json:"skipped"`
	StartedAt   time.Time                 `json:"started_at"`
	CompletedAt *time.Time                `json:"completed_at,omitempty"`
	Items       []accountModelRefreshItem `json:"items"`
}

type accountModelRefreshJobStore struct {
	mu      sync.RWMutex
	jobs    map[string]*accountModelRefreshJob
	active  string
	startMu sync.Mutex
}

func newAccountModelRefreshJobStore() *accountModelRefreshJobStore {
	return &accountModelRefreshJobStore{jobs: make(map[string]*accountModelRefreshJob)}
}

func (s *accountModelRefreshJobStore) snapshot(jobID string) (*accountModelRefreshJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return nil, false
	}
	copyJob := *job
	copyJob.Items = append([]accountModelRefreshItem(nil), job.Items...)
	return &copyJob, true
}

func (s *accountModelRefreshJobStore) activeSnapshot() (*accountModelRefreshJob, bool) {
	s.mu.RLock()
	active := s.active
	s.mu.RUnlock()
	if active == "" {
		return nil, false
	}
	return s.snapshot(active)
}

func (s *accountModelRefreshJobStore) create(accounts []service.Account) *accountModelRefreshJob {
	now := time.Now().UTC()
	job := &accountModelRefreshJob{
		ID:        fmt.Sprintf("models-%d", now.UnixNano()),
		Status:    "running",
		Total:     len(accounts),
		StartedAt: now,
		Items:     make([]accountModelRefreshItem, 0, len(accounts)),
	}
	for i := range accounts {
		account := &accounts[i]
		job.Items = append(job.Items, accountModelRefreshItem{
			AccountID:   account.ID,
			AccountName: account.Name,
			UpstreamURL: accountModelRefreshUpstreamURL(account),
			Status:      accountModelRefreshStatusPending,
		})
	}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.active = job.ID
	s.mu.Unlock()
	copyJob, _ := s.snapshot(job.ID)
	return copyJob
}

func (s *accountModelRefreshJobStore) setItemStatus(jobID string, index int, status string, modelCount int, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil || index < 0 || index >= len(job.Items) {
		return
	}
	previous := job.Items[index].Status
	job.Items[index].Status = status
	job.Items[index].ModelCount = modelCount
	job.Items[index].Message = message
	if previous == accountModelRefreshStatusSuccess ||
		previous == accountModelRefreshStatusFailed ||
		previous == accountModelRefreshStatusSkipped {
		return
	}
	switch status {
	case accountModelRefreshStatusSuccess:
		job.Completed++
		job.Succeeded++
	case accountModelRefreshStatusFailed:
		job.Completed++
		job.Failed++
	case accountModelRefreshStatusSkipped:
		job.Completed++
		job.Skipped++
	}
}

func (s *accountModelRefreshJobStore) finish(jobID string) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil {
		return
	}
	job.Status = "completed"
	job.CompletedAt = &now
	if s.active == jobID {
		s.active = ""
	}
}

// StartModelRefreshJob starts a server-owned background job. The request may end
// and the browser may close the progress dialog without cancelling the refresh.
func (h *AccountHandler) StartModelRefreshJob(c *gin.Context) {
	if h.adminService == nil || h.modelRefreshJobs == nil || !h.canSyncUpstreamModels() {
		response.InternalError(c, "Account model refresh service is not configured")
		return
	}
	// Serialize check/list/create so simultaneous confirmations share one job.
	h.modelRefreshJobs.startMu.Lock()
	defer h.modelRefreshJobs.startMu.Unlock()
	if active, ok := h.modelRefreshJobs.activeSnapshot(); ok && active.Status == "running" {
		response.Success(c, active)
		return
	}

	accounts, err := h.listAllAccountsForModelRefresh(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	job := h.modelRefreshJobs.create(accounts)
	go h.runModelRefreshJob(job.ID, accounts)
	response.Success(c, job)
}

// GetModelRefreshJob returns the latest immutable snapshot for progress polling.
func (h *AccountHandler) GetModelRefreshJob(c *gin.Context) {
	job, ok := h.modelRefreshJobs.snapshot(strings.TrimSpace(c.Param("job_id")))
	if !ok {
		response.NotFound(c, "Account model refresh job not found")
		return
	}
	response.Success(c, job)
}

func (h *AccountHandler) listAllAccountsForModelRefresh(ctx context.Context) ([]service.Account, error) {
	accounts := make([]service.Account, 0)
	for page := 1; ; page++ {
		batch, total, err := h.adminService.ListAccounts(ctx, page, accountModelRefreshPageSize, "", "", "", "", 0, "", "id", "asc")
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, batch...)
		if len(accounts) >= int(total) || len(batch) == 0 {
			break
		}
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].ID < accounts[j].ID })
	return accounts, nil
}

// runModelRefreshJob replaces each account's model whitelist with the model list
// the upstream actually reports. It is the batch form of the manual sequence
// "Edit -> Clear all models -> Sync upstream supported models -> Save".
func (h *AccountHandler) runModelRefreshJob(jobID string, accounts []service.Account) {
	defer h.modelRefreshJobs.finish(jobID)
	for index := range accounts {
		accountSummary := &accounts[index]
		h.modelRefreshJobs.setItemStatus(jobID, index, accountModelRefreshStatusRefreshing, 0, "")

		// Reload each account immediately before syncing. This mirrors the single-account
		// edit flow and avoids using a stale list snapshot for credentials, proxy, headers,
		// or other provider-specific settings.
		account, err := h.adminService.GetAccount(context.Background(), accountSummary.ID)
		if err != nil || account == nil {
			slog.Warn("account_model_refresh_load_failed", "account_id", accountSummary.ID, "error", err)
			h.modelRefreshJobs.setItemStatus(jobID, index, accountModelRefreshStatusFailed, 0, "failed to load account")
			continue
		}

		count, skipReason, err := h.refreshAccountModelsFromUpstream(account)
		switch {
		case skipReason != "":
			h.modelRefreshJobs.setItemStatus(jobID, index, accountModelRefreshStatusSkipped, 0, skipReason)
		case err != nil:
			slog.Warn("account_model_refresh_upstream_failed", "account_id", account.ID, "error", err)
			h.modelRefreshJobs.setItemStatus(jobID, index, accountModelRefreshStatusFailed, 0, safeAccountModelRefreshError(err))
		default:
			h.modelRefreshJobs.setItemStatus(jobID, index, accountModelRefreshStatusSuccess, count, "")
		}
	}
}

// refreshAccountModelsFromUpstream performs, for one account, the same steps an
// admin would repeat by hand: clear the whitelist, sync the upstream model list,
// then save.
//
// "Clear all models" in the editor empties the whitelist list only; custom
// aliases (request model != upstream model) live in the mapping tab and survive
// that button. This function reproduces that exactly:
//   - identity entries (model -> itself) are discarded and rebuilt from upstream;
//   - custom aliases are kept, and win over an identity entry with the same key,
//     matching how the editor merges both lists on save;
//   - every other credential field is written back unchanged.
//
// When the upstream call fails the account is left untouched, which is what
// happens when the editor is closed without saving.
func (h *AccountHandler) refreshAccountModelsFromUpstream(account *service.Account) (int, string, error) {
	if account.IsCredentialShadow() {
		return 0, "shadow account: models follow the parent account", nil
	}
	if account.IsOpenAIPassthroughEnabled() {
		return 0, "OpenAI passthrough enabled: model restriction is not used", nil
	}

	catalog, err := h.syncUpstreamModelCatalog(context.Background(), account)
	if err != nil {
		return 0, "", err
	}
	if catalog == nil {
		return 0, "", errors.New("upstream returned no model catalog")
	}

	upstreamModels := make([]string, 0, len(catalog.Models))
	for _, model := range catalog.Models {
		model = strings.TrimSpace(model)
		// The editor's whitelist writer skips wildcard entries; mirror that so a
		// synced list can never turn into a "model -> pattern" rewrite.
		if model == "" || strings.Contains(model, "*") {
			continue
		}
		upstreamModels = append(upstreamModels, model)
	}
	upstreamModels = dedupeAccountModelNames(upstreamModels)
	if len(upstreamModels) == 0 {
		return 0, "", errors.New("upstream returned no supported models")
	}

	mapping := make(map[string]any, len(upstreamModels))
	for key, value := range accountModelCustomAliases(account.Credentials["model_mapping"]) {
		mapping[key] = value
	}
	for _, model := range upstreamModels {
		if _, hasAlias := mapping[model]; hasAlias {
			// A custom alias for this exact request model wins, as it does in the editor.
			continue
		}
		mapping[model] = model
	}

	credentials := cloneAccountCredentialsForModelRefresh(account.Credentials)
	credentials["model_mapping"] = mapping
	if account.Platform == service.PlatformAntigravity {
		// The editor drops the retired whitelist field once it saves a mapping.
		delete(credentials, "model_whitelist")
	}

	if _, err := h.adminService.UpdateAccount(context.Background(), account.ID, &service.UpdateAccountInput{Credentials: credentials}); err != nil {
		return 0, "", err
	}
	return len(mapping), "", nil
}

func (h *AccountHandler) canSyncUpstreamModels() bool {
	return h.accountTestService != nil || h.modelCatalogSync != nil
}

// syncUpstreamModelCatalog fetches an account's live upstream model list and
// persists capability metadata, exactly like the editor's sync button.
func (h *AccountHandler) syncUpstreamModelCatalog(ctx context.Context, account *service.Account) (*service.UpstreamModelCatalog, error) {
	if h.modelCatalogSync != nil {
		return h.modelCatalogSync(ctx, account)
	}
	if h.accountTestService == nil {
		return nil, errors.New("account test service is not configured")
	}
	return h.accountTestService.SyncUpstreamModelCatalog(ctx, account)
}

func safeAccountModelRefreshError(err error) string {
	if err == nil {
		return ""
	}
	var syncErr *service.UpstreamModelSyncError
	if errors.As(err, &syncErr) {
		return syncErr.SafeMessage()
	}
	slog.Warn("account_model_refresh_error", "error", err)
	return "failed to refresh models from upstream"
}

// accountModelCustomAliases returns the entries of a stored model_mapping whose
// value differs from the key, i.e. the mappings the editor keeps in its mapping tab.
func accountModelCustomAliases(raw any) map[string]string {
	aliases := make(map[string]string)
	add := func(key, value string) {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || key == value || strings.Contains(value, "*") {
			return
		}
		aliases[key] = value
	}
	switch mapping := raw.(type) {
	case map[string]any:
		for key, value := range mapping {
			if text, ok := value.(string); ok {
				add(key, text)
			}
		}
	case map[string]string:
		for key, value := range mapping {
			add(key, value)
		}
	}
	return aliases
}

func dedupeAccountModelNames(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	result := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		key := strings.ToLower(model)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, model)
	}
	return result
}

func cloneAccountCredentialsForModelRefresh(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source)+1)
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func accountModelRefreshUpstreamURL(account *service.Account) string {
	if account == nil {
		return ""
	}
	if value, ok := account.Credentials["base_url"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	switch account.Platform {
	case service.PlatformOpenAI:
		return "https://api.openai.com"
	case service.PlatformGemini:
		return "https://generativelanguage.googleapis.com"
	case service.PlatformGrok:
		return "https://api.x.ai"
	case service.PlatformAnthropic:
		return "https://api.anthropic.com"
	default:
		return account.Platform
	}
}
