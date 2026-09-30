package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// A failed task may still acquire a delayed main-site billing record.
// Its task state must not close the independent usage replication queue.
func TestFailedTasksReplicateDelayedMainUsage(t *testing.T) {
	for _, kind := range []string{"image", "video"} {
		t.Run(kind, func(t *testing.T) {
			visible := false
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/sub2api/usage" {
					items := []any{}
					if visible {
						items = append(items, map[string]any{"id": 99, "request_id": "delayed", "actual_cost": 2.5, "total_cost": 3})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "failed-task", "task_id": "failed-task", "status": "failed"})
			}))
			defer upstream.Close()
			s := testServer(t, upstream)
			s.cfg.OwnerMainUserID = ""
			s.cfg.VideoTaskReconcileAge = time.Minute
			s.cfg.ImageTaskReconcileAge = time.Minute
			s.main = NewMainClient(s.cfg)
			old := time.Now().UTC().Add(-2 * time.Hour)
			s.store.clock = func() time.Time { return old }
			if _, _, err := s.store.PrepareDirectSettlement(s.cfg.AgentID, "42", "42", "delayed", "delayed", 100); err != nil {
				t.Fatal(err)
			}
			if kind == "image" {
				if _, err := s.store.RecordImageTask(s.cfg.AgentID, "42", "failed-task", "delayed", "processing"); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := s.store.RecordVideoTask(s.cfg.AgentID, "42", "failed-task", "delayed", "processing"); err != nil {
					t.Fatal(err)
				}
			}
			s.store.clock = time.Now
			if kind == "image" {
				s.reconcileStaleImageTasks(context.Background())
			} else {
				s.reconcileStaleVideoTasks(context.Background())
			}
			record, err := s.store.Settlement(s.cfg.AgentID, "delayed")
			if err != nil || record.Status != "pending" {
				t.Fatalf("missing usage must remain pending: %+v %v", record, err)
			}
			visible = true
			if _, err := s.reconcileSettlements(context.Background(), ""); err != nil {
				t.Fatal(err)
			}
			record, err = s.store.Settlement(s.cfg.AgentID, "delayed")
			if err != nil || record.Status != "confirmed" || record.ActualCents != 250 || record.UsageID != "99" {
				t.Fatalf("delayed fact not replicated: %+v %v", record, err)
			}
		})
	}
}

func TestPendingReconciliationRotationPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.db")
	s, err := OpenStore(path, "secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "second", "third"} {
		if _, _, err := s.PrepareDirectSettlement("a", "42", "42", id, id, 100); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkReconciliationAttempt("a", "first"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(path, "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, expected := range []string{"second", "third", "first"} {
		rows, err := s.PendingSettlements("a", 1)
		if err != nil || len(rows) != 1 || rows[0].RequestID != expected {
			t.Fatalf("queue expected %s got %+v err=%v", expected, rows, err)
		}
		if err := s.MarkReconciliationAttempt("a", expected); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReconciliationWaitsForActualCostInsteadOfStandardCost(t *testing.T) {
	actualVisible := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		item := map[string]any{"id": 81, "request_id": "discounted", "total_cost": 4.0}
		if actualVisible {
			item["actual_cost"] = 0
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{item}})
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	s.main = NewMainClient(s.cfg)
	if _, _, err := s.store.PrepareDirectSettlement(s.cfg.AgentID, "42", "42", "discounted", "discounted", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := s.reconcileSettlements(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	record, err := s.store.Settlement(s.cfg.AgentID, "discounted")
	if err != nil || record.Status != "pending" || record.ActualCents != 0 {
		t.Fatalf("standard cost was treated as an actual charge: %+v %v", record, err)
	}
	actualVisible = true
	if _, err := s.reconcileSettlements(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	record, err = s.store.Settlement(s.cfg.AgentID, "discounted")
	if err != nil || record.Status != "confirmed" || record.ActualCents != 0 {
		t.Fatalf("explicit zero charge was not confirmed: %+v %v", record, err)
	}
}

func TestReconciliationContinuesPastFailedUsageLookup(t *testing.T) {
	calls := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("request_id")
		calls = append(calls, id)
		if id == "first" {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{"id": 12, "request_id": id, "actual_cost": 2, "total_cost": 2}}})
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	s.main = NewMainClient(s.cfg)
	for _, id := range []string{"first", "second"} {
		if _, _, err := s.store.PrepareDirectSettlement(s.cfg.AgentID, "42", "42", id, id, 100); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.reconcileSettlements(context.Background(), "")
	if err != nil || len(rows) != 2 || len(calls) != 2 {
		t.Fatalf("batch stopped %+v calls=%v err=%v", rows, calls, err)
	}
	first, err := s.store.Settlement(s.cfg.AgentID, "first")
	if err != nil || first.Status != "pending" {
		t.Fatalf("failed lookup altered record %+v %v", first, err)
	}
	second, err := s.store.Settlement(s.cfg.AgentID, "second")
	if err != nil || second.Status != "confirmed" || second.ActualCents != 200 {
		t.Fatalf("second record not synced %+v %v", second, err)
	}
}

func TestReconciliationWithoutUsageDoesNotStarveNextBatch(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("request_id")
		items := []any{}
		if id == "second" {
			items = append(items, map[string]any{"id": 13, "request_id": id, "actual_cost": 0, "total_cost": 0})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	}))
	defer upstream.Close()
	s := testServer(t, upstream)
	s.cfg.SettlementReconcileBatch = 1
	s.main = NewMainClient(s.cfg)
	for _, id := range []string{"first", "second"} {
		if _, _, err := s.store.PrepareDirectSettlement(s.cfg.AgentID, "42", "42", id, id, 100); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := s.reconcileSettlements(context.Background(), ""); err != nil {
			t.Fatal(err)
		}
	}
	second, err := s.store.Settlement(s.cfg.AgentID, "second")
	if err != nil || second.Status != "confirmed" {
		t.Fatalf("starved record: %+v %v", second, err)
	}
}
