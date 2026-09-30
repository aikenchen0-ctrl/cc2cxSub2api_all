package admin

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modelcatalog"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const accountModelRefreshPageSize = 500

type accountModelRefreshItem struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	UpstreamURL string `json:"upstream_url"`
	Status      string `json:"status"`
	ModelCount  int    `json:"model_count,omitempty"`
}

type accountModelRefreshJob struct {
	ID          string                    `json:"id"`
	Status      string                    `json:"status"`
	Total       int                       `json:"total"`
	Completed   int                       `json:"completed"`
	Succeeded   int                       `json:"succeeded"`
	Failed      int                       `json:"failed"`
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
			Status:      "pending",
		})
	}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.active = job.ID
	s.mu.Unlock()
	copyJob, _ := s.snapshot(job.ID)
	return copyJob
}

func (s *accountModelRefreshJobStore) setItemStatus(jobID string, index int, status string, modelCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil || index < 0 || index >= len(job.Items) {
		return
	}
	previous := job.Items[index].Status
	job.Items[index].Status = status
	job.Items[index].ModelCount = modelCount
	if previous == "success" || previous == "failed" {
		return
	}
	if status == "success" || status == "failed" {
		job.Completed++
		if status == "success" {
			job.Succeeded++
		} else {
			job.Failed++
		}
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
	if h.adminService == nil || h.modelRefreshJobs == nil {
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

func (h *AccountHandler) runModelRefreshJob(jobID string, accounts []service.Account) {
	defer h.modelRefreshJobs.finish(jobID)
	for index := range accounts {
		accountSummary := &accounts[index]
		h.modelRefreshJobs.setItemStatus(jobID, index, "refreshing", 0)

		// Reload each account immediately before syncing. This mirrors the single-account
		// edit flow and avoids using a stale list snapshot for credentials, proxy, headers,
		// or other provider-specific settings.
		account, err := h.adminService.GetAccount(context.Background(), accountSummary.ID)
		if err != nil || account == nil {
			slog.Warn("account_model_refresh_load_failed", "account_id", accountSummary.ID, "error", err)
			h.modelRefreshJobs.setItemStatus(jobID, index, "failed", 0)
			continue
		}

		// Match Edit -> fillRelated -> Update: add the shared built-in catalog,
		// preserving custom aliases and every unrelated credential field.
		credentials := cloneAccountCredentialsForModelRefresh(account.Credentials)
		mapping := make(map[string]any)
		switch existing := credentials["model_mapping"].(type) {
		case map[string]any:
			for key, value := range existing {
				mapping[key] = value
			}
		case map[string]string:
			for key, value := range existing {
				mapping[key] = value
			}
		}
		// Preserve legacy Antigravity whitelist entries when migrating to mappings.
		if account.Platform == "antigravity" && credentials["model_mapping"] == nil {
			if legacy, ok := credentials["model_whitelist"].([]any); ok {
				for _, value := range legacy {
					if model, ok := value.(string); ok && strings.TrimSpace(model) != "" {
						model = strings.TrimSpace(model)
						mapping[model] = model
					}
				}
			}
		}
		if account.Platform == "antigravity" {
			delete(credentials, "model_whitelist")
		}
		for _, model := range modelcatalog.ForPlatform(account.Platform) {
			if _, exists := mapping[model]; !exists {
				mapping[model] = model
			}
		}
		if len(mapping) == 0 {
			h.modelRefreshJobs.setItemStatus(jobID, index, "failed", 0)
			continue
		}
		credentials["model_mapping"] = mapping
		if _, err := h.adminService.UpdateAccount(context.Background(), account.ID, &service.UpdateAccountInput{Credentials: credentials}); err != nil {
			slog.Warn("account_model_refresh_save_failed", "account_id", account.ID, "error", err)
			h.modelRefreshJobs.setItemStatus(jobID, index, "failed", 0)
			continue
		}
		h.modelRefreshJobs.setItemStatus(jobID, index, "success", len(mapping))
	}
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
