package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestUpstreamAuditJobStoreLifecycle(t *testing.T) {
	store := newUpstreamAuditJobStore()
	job := store.create(2)
	if job.Status != "scanning" || job.TotalAccounts != 2 {
		t.Fatalf("unexpected initial job: %#v", job)
	}

	item := upstreamAuditItem{AccountID: 7, AccountName: "upstream", DeclaredModel: "gpt-5.4", Status: "pending"}
	index := store.addItem(job.ID, item)
	item.Status = "probing"
	store.updateItem(job.ID, index, item)
	compatible := true
	item.Status = "verified"
	item.Compatible = &compatible
	store.updateItem(job.ID, index, item)
	store.updateItem(job.ID, index, item)
	store.markAccountScanned(job.ID)
	store.addAccountError(job.ID, upstreamAuditAccountError{AccountID: 8, AccountName: "broken", Error: "unavailable"})
	store.finish(job.ID)

	snapshot := store.snapshot(job.ID)
	if snapshot.Status != "completed" || snapshot.ScannedAccounts != 2 || snapshot.MatchedModels != 1 || snapshot.Completed != 1 || snapshot.Verified != 1 {
		t.Fatalf("unexpected completed job: %#v", snapshot)
	}
	if store.activeSnapshot() != nil || store.latestSnapshot().ID != job.ID {
		t.Fatal("active/latest job pointers were not updated")
	}
}

func TestGetUpstreamAuditOverviewReturnsEmbeddedLibrary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/admin/upstream-audit", handler.GetUpstreamAuditOverview)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/upstream-audit", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data struct {
			SupportedModels []string `json:"supported_models"`
			BankSHA256      string   `json:"bank_sha256"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.SupportedModels) != 16 || envelope.Data.BankSHA256 == "" {
		t.Fatalf("unexpected overview: %#v", envelope.Data)
	}
}

func TestStartUpstreamAuditRequiresConfiguredServices(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/admin/upstream-audit/start", handler.StartUpstreamAudit)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/admin/upstream-audit/start", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestUpstreamAuditFallbackModelsUsesMappingSnapshotAndCatalog(t *testing.T) {
	account := &service.Account{
		Platform: service.PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-5.4": "vendor-gpt"},
		},
		Extra: map[string]any{
			service.UpstreamModelMetadataExtraKey: service.UpstreamModelMetadataSnapshot{
				Models: map[string]service.UpstreamModelMetadata{"gpt-5.5": {ID: "gpt-5.5"}},
			},
		},
	}
	models := upstreamAuditFallbackModels(account)
	contains := func(target string) bool {
		for _, model := range models {
			if model == target {
				return true
			}
		}
		return false
	}
	if !contains("gpt-5.4") || !contains("gpt-5.5") {
		t.Fatalf("fallback models = %v", models)
	}
}

func TestDedupeUpstreamAuditModelsIsCaseInsensitive(t *testing.T) {
	models := dedupeUpstreamAuditModels([]string{"gpt-5.4", " GPT-5.4 ", "claude-opus-5"})
	if len(models) != 2 {
		t.Fatalf("models = %v", models)
	}
}
