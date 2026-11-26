package repository

import (
	"testing"
	"time"

	"github.com/tigerowo/infinite-canvas/config"
	"github.com/tigerowo/infinite-canvas/model"
)

func TestFailStaleCanvasImageTasksOnlyFailsExpiredUnfinishedTasks(t *testing.T) {
	config.Cfg = config.Config{StorageDriver: "sqlite", DatabaseDSN: "file:stale-canvas-image-task?mode=memory&cache=shared"}
	now := time.Now().UTC()
	old := now.Add(-45 * time.Minute).Format(time.RFC3339Nano)
	recent := now.Add(-2 * time.Minute).Format(time.RFC3339Nano)
	cutoff := now.Add(-30 * time.Minute).Format(time.RFC3339)

	tasks := []model.CanvasImageTask{
		{ID: "stale-processing", UserID: "user-1", Status: "processing", CreatedAt: old, UpdatedAt: old, StartedAt: old},
		{ID: "fresh-processing", UserID: "user-1", Status: "processing", CreatedAt: recent, UpdatedAt: recent, StartedAt: recent},
		{ID: "old-completed", UserID: "user-1", Status: "completed", CreatedAt: old, UpdatedAt: old, ImageURL: "https://example.invalid/a.png"},
	}
	for _, task := range tasks {
		if _, err := SaveCanvasImageTask(task); err != nil {
			t.Fatal(err)
		}
	}

	affected, err := FailStaleCanvasImageTasks(cutoff, "图片任务超时未完成")
	if err != nil {
		t.Fatal(err)
	}
	if affected != 1 {
		t.Fatalf("expected one stale task to be failed, got %d", affected)
	}

	stale, _, err := GetCanvasImageTaskByID("stale-processing")
	if err != nil {
		t.Fatal(err)
	}
	if stale.Status != "failed" || stale.Error == "" || stale.CompletedAt == "" {
		t.Fatalf("stale task was not closed: %+v", stale)
	}
	fresh, _, err := GetCanvasImageTaskByID("fresh-processing")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status != "processing" {
		t.Fatalf("fresh task must keep processing, got %q", fresh.Status)
	}
	completed, _, err := GetCanvasImageTaskByID("old-completed")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" {
		t.Fatalf("completed task must stay completed, got %q", completed.Status)
	}
}
