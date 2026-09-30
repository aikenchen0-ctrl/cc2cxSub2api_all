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

	"github.com/Wei-Shaw/sub2api/internal/modelcatalog"
	"github.com/Wei-Shaw/sub2api/internal/modeltrace"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	upstreamAuditTargetOutputs = 3
	upstreamAuditMaxAttempts   = 6
	upstreamAuditProbeTimeout  = 4 * time.Minute
)

type upstreamAuditItem struct {
	AccountID           int64                   `json:"account_id"`
	AccountName         string                  `json:"account_name"`
	Platform            string                  `json:"platform"`
	DeclaredModel       string                  `json:"declared_model"`
	Status              string                  `json:"status"`
	Prediction          string                  `json:"prediction,omitempty"`
	PredictionName      string                  `json:"prediction_name,omitempty"`
	Probability         float64                 `json:"probability,omitempty"`
	DeclaredProbability float64                 `json:"declared_probability,omitempty"`
	DeclaredSimilarity  float64                 `json:"declared_similarity,omitempty"`
	FamilyPrediction    string                  `json:"family_prediction,omitempty"`
	FamilyProbability   float64                 `json:"family_probability,omitempty"`
	Compatible          *bool                   `json:"compatible,omitempty"`
	UsedOutputs         int                     `json:"used_outputs,omitempty"`
	Attempts            int                     `json:"attempts,omitempty"`
	LatencyMs           int64                   `json:"latency_ms,omitempty"`
	Error               string                  `json:"error,omitempty"`
	Diagnostics         []modeltrace.Diagnostic `json:"diagnostics,omitempty"`
	StartedAt           *time.Time              `json:"started_at,omitempty"`
	CompletedAt         *time.Time              `json:"completed_at,omitempty"`
}

type upstreamAuditAccountError struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	Error       string `json:"error"`
}

type upstreamAuditJob struct {
	ID              string                      `json:"id"`
	Status          string                      `json:"status"`
	TotalAccounts   int                         `json:"total_accounts"`
	ScannedAccounts int                         `json:"scanned_accounts"`
	MatchedModels   int                         `json:"matched_models"`
	Completed       int                         `json:"completed"`
	Verified        int                         `json:"verified"`
	Mismatched      int                         `json:"mismatched"`
	Failed          int                         `json:"failed"`
	StartedAt       time.Time                   `json:"started_at"`
	CompletedAt     *time.Time                  `json:"completed_at,omitempty"`
	Items           []upstreamAuditItem         `json:"items"`
	AccountErrors   []upstreamAuditAccountError `json:"account_errors,omitempty"`
}

type upstreamAuditJobStore struct {
	mu      sync.RWMutex
	startMu sync.Mutex
	jobs    map[string]*upstreamAuditJob
	active  string
	latest  string
}

func newUpstreamAuditJobStore() *upstreamAuditJobStore {
	return &upstreamAuditJobStore{jobs: make(map[string]*upstreamAuditJob)}
}

func (s *upstreamAuditJobStore) create(accountCount int) *upstreamAuditJob {
	now := time.Now().UTC()
	job := &upstreamAuditJob{ID: fmt.Sprintf("upstream-audit-%d", now.UnixNano()), Status: "scanning", TotalAccounts: accountCount, StartedAt: now, Items: []upstreamAuditItem{}}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.active = job.ID
	s.latest = job.ID
	s.mu.Unlock()
	return s.snapshot(job.ID)
}

func (s *upstreamAuditJobStore) snapshot(id string) *upstreamAuditJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job := s.jobs[id]
	if job == nil {
		return nil
	}
	copyJob := *job
	copyJob.Items = append([]upstreamAuditItem(nil), job.Items...)
	for i := range copyJob.Items {
		copyJob.Items[i].Diagnostics = append([]modeltrace.Diagnostic(nil), job.Items[i].Diagnostics...)
	}
	copyJob.AccountErrors = append([]upstreamAuditAccountError(nil), job.AccountErrors...)
	return &copyJob
}

func (s *upstreamAuditJobStore) activeSnapshot() *upstreamAuditJob {
	s.mu.RLock()
	id := s.active
	s.mu.RUnlock()
	if id == "" {
		return nil
	}
	return s.snapshot(id)
}
func (s *upstreamAuditJobStore) latestSnapshot() *upstreamAuditJob {
	s.mu.RLock()
	id := s.latest
	s.mu.RUnlock()
	if id == "" {
		return nil
	}
	return s.snapshot(id)
}

func (s *upstreamAuditJobStore) addAccountError(id string, item upstreamAuditAccountError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job := s.jobs[id]; job != nil {
		job.AccountErrors = append(job.AccountErrors, item)
		job.ScannedAccounts++
	}
}
func (s *upstreamAuditJobStore) markAccountScanned(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job := s.jobs[id]; job != nil {
		job.ScannedAccounts++
	}
}
func (s *upstreamAuditJobStore) addItem(id string, item upstreamAuditItem) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[id]
	if job == nil {
		return -1
	}
	job.Items = append(job.Items, item)
	job.MatchedModels++
	return len(job.Items) - 1
}
func (s *upstreamAuditJobStore) updateItem(id string, index int, item upstreamAuditItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[id]
	if job == nil || index < 0 || index >= len(job.Items) {
		return
	}
	previous := job.Items[index].Status
	job.Items[index] = item
	if previous != "verified" && previous != "mismatch" && previous != "failed" {
		switch item.Status {
		case "verified":
			job.Completed++
			job.Verified++
		case "mismatch":
			job.Completed++
			job.Mismatched++
		case "failed":
			job.Completed++
			job.Failed++
		}
	}
}
func (s *upstreamAuditJobStore) finish(id string) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if job := s.jobs[id]; job != nil {
		job.Status = "completed"
		job.CompletedAt = &now
	}
	if s.active == id {
		s.active = ""
	}
}

// GetUpstreamAuditOverview returns the embedded library and latest job snapshot.
func (h *AccountHandler) GetUpstreamAuditOverview(c *gin.Context) {
	response.Success(c, gin.H{"supported_models": modeltrace.SupportedModels(), "bank_sha256": "1c2cb74d372f9f0f30d0dabbb7b7a838660d2f769a88d0c8489e4c662e088c21", "latest_job": h.upstreamAuditJobs.latestSnapshot()})
}

// StartUpstreamAudit scans every account's live model catalog and audits every
// model present in the embedded ModelTrace bank.
func (h *AccountHandler) StartUpstreamAudit(c *gin.Context) {
	if h.adminService == nil || h.accountTestService == nil || h.upstreamAuditJobs == nil {
		response.InternalError(c, "Upstream audit service is not configured")
		return
	}
	h.upstreamAuditJobs.startMu.Lock()
	defer h.upstreamAuditJobs.startMu.Unlock()
	if active := h.upstreamAuditJobs.activeSnapshot(); active != nil {
		response.Success(c, active)
		return
	}
	accounts, err := h.listAllAccountsForModelRefresh(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	job := h.upstreamAuditJobs.create(len(accounts))
	go h.runUpstreamAudit(job.ID, accounts)
	response.Accepted(c, job)
}

func (h *AccountHandler) GetUpstreamAuditJob(c *gin.Context) {
	job := h.upstreamAuditJobs.snapshot(strings.TrimSpace(c.Param("job_id")))
	if job == nil {
		response.NotFound(c, "Upstream audit job not found")
		return
	}
	response.Success(c, job)
}

func (h *AccountHandler) runUpstreamAudit(jobID string, accounts []service.Account) {
	defer h.upstreamAuditJobs.finish(jobID)
	supported := make(map[string]struct{})
	for _, model := range modeltrace.SupportedModels() {
		supported[strings.ToLower(model)] = struct{}{}
	}
	for i := range accounts {
		account, err := h.adminService.GetAccount(context.Background(), accounts[i].ID)
		if err != nil || account == nil {
			h.upstreamAuditJobs.addAccountError(jobID, upstreamAuditAccountError{AccountID: accounts[i].ID, AccountName: accounts[i].Name, Error: "failed to load account"})
			continue
		}
		catalog, err := h.accountTestService.SyncUpstreamModelCatalog(context.Background(), account)
		models := make([]string, 0)
		if err == nil && catalog != nil {
			models = append(models, catalog.Models...)
			for requested := range account.GetModelMapping() {
				models = append(models, requested)
			}
		} else {
			models = upstreamAuditFallbackModels(account)
			if len(models) == 0 {
				h.upstreamAuditJobs.addAccountError(jobID, upstreamAuditAccountError{AccountID: account.ID, AccountName: account.Name, Error: safeUpstreamAuditError(err)})
				continue
			}
			slog.Info("upstream_audit_using_configured_models", "account_id", account.ID, "model_count", len(models), "reason", safeUpstreamAuditError(err))
		}
		models = dedupeUpstreamAuditModels(models)
		for _, model := range models {
			model = strings.TrimSpace(model)
			if _, ok := supported[strings.ToLower(model)]; !ok {
				continue
			}
			item := upstreamAuditItem{AccountID: account.ID, AccountName: account.Name, Platform: account.Platform, DeclaredModel: model, Status: "pending"}
			index := h.upstreamAuditJobs.addItem(jobID, item)
			if index >= 0 {
				h.auditUpstreamModel(jobID, index, item)
			}
		}
		h.upstreamAuditJobs.markAccountScanned(jobID)
	}
}

func (h *AccountHandler) auditUpstreamModel(jobID string, index int, item upstreamAuditItem) {
	started := time.Now().UTC()
	item.StartedAt = &started
	item.Status = "probing"
	h.upstreamAuditJobs.updateItem(jobID, index, item)
	challenges := modeltrace.GenerateChallenges(upstreamAuditMaxAttempts)
	outputs := make([]modeltrace.Output, 0, upstreamAuditTargetOutputs)
	var lastError string
	for _, challenge := range challenges {
		item.Attempts++
		ctx, cancel := context.WithTimeout(context.Background(), upstreamAuditProbeTimeout)
		result, err := h.accountTestService.RunPromptBackground(ctx, item.AccountID, item.DeclaredModel, challenge.Prompt)
		cancel()
		if err != nil {
			lastError = err.Error()
			continue
		}
		item.LatencyMs += result.LatencyMs
		if result.Status != "success" {
			lastError = result.ErrorMessage
			continue
		}
		output := modeltrace.Output{Text: result.ResponseText, ExpectedCount: challenge.ExpectedCount}
		minimum := maxAuditInt(80, int(float64(challenge.ExpectedCount)*0.55+0.999999))
		if len(modeltrace.ParseNumbers(output.Text)) < minimum {
			lastError = "model response did not contain enough valid integers"
			continue
		}
		outputs = append(outputs, output)
		if len(outputs) >= upstreamAuditTargetOutputs {
			break
		}
	}
	analysis, err := modeltrace.Analyze(outputs)
	completed := time.Now().UTC()
	item.CompletedAt = &completed
	if err != nil {
		item.Status = "failed"
		if lastError != "" {
			item.Error = lastError
		} else {
			item.Error = err.Error()
		}
		h.upstreamAuditJobs.updateItem(jobID, index, item)
		return
	}
	item.Prediction = analysis.Prediction
	item.PredictionName = analysis.PredictionName
	item.Probability = analysis.Probability
	item.FamilyPrediction = analysis.FamilyPrediction
	item.FamilyProbability = analysis.FamilyProbability
	item.UsedOutputs = analysis.UsedOutputs
	item.Diagnostics = analysis.Diagnostics
	compatible := strings.EqualFold(item.DeclaredModel, analysis.Prediction)
	item.Compatible = &compatible
	for _, result := range analysis.Results {
		if strings.EqualFold(result.Model, item.DeclaredModel) {
			item.DeclaredProbability = result.Probability
			item.DeclaredSimilarity = result.ProfileSimilarity
			break
		}
	}
	if compatible {
		item.Status = "verified"
	} else {
		item.Status = "mismatch"
	}
	h.upstreamAuditJobs.updateItem(jobID, index, item)
}

func safeUpstreamAuditError(err error) string {
	if err == nil {
		return ""
	}
	var syncErr *service.UpstreamModelSyncError
	if errors.As(err, &syncErr) {
		return syncErr.SafeMessage()
	}
	slog.Warn("upstream_audit_model_sync_failed", "error", err)
	return "failed to fetch upstream model list"
}

func upstreamAuditFallbackModels(account *service.Account) []string {
	if account == nil {
		return nil
	}
	seen := make(map[string]string)
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		key := strings.ToLower(model)
		if _, exists := seen[key]; !exists {
			seen[key] = model
		}
	}
	for requested := range account.GetModelMapping() {
		add(requested)
	}
	if snapshot := account.GetUpstreamModelMetadataSnapshot(); snapshot != nil {
		for model := range snapshot.Models {
			add(model)
		}
	}
	for _, model := range modelcatalog.ForPlatform(account.Platform) {
		add(model)
	}
	models := make([]string, 0, len(seen))
	for _, model := range seen {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}

func dedupeUpstreamAuditModels(models []string) []string {
	seen := make(map[string]string, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		key := strings.ToLower(model)
		if _, exists := seen[key]; !exists {
			seen[key] = model
		}
	}
	result := make([]string, 0, len(seen))
	for _, model := range seen {
		result = append(result, model)
	}
	sort.Strings(result)
	return result
}

func maxAuditInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
