package service

import (
	"log"
	"time"

	"github.com/tigerowo/infinite-canvas/repository"
)

// canvasImageTaskStaleAfter is deliberately longer than the largest channel
// timeout (600s) so a slow but healthy generation is not failed early.
const canvasImageTaskStaleAfter = 30 * time.Minute

func StartCanvasImageTaskRecovery() {
	go func() {
		RecoverStaleCanvasImageTasks()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			RecoverStaleCanvasImageTasks()
		}
	}()
}

// RecoverStaleCanvasImageTasks fails tasks whose owning goroutine disappeared
// with the previous process, preventing the workbench from polling forever.
func RecoverStaleCanvasImageTasks() {
	cutoff := time.Now().UTC().Add(-canvasImageTaskStaleAfter).Format(time.RFC3339)
	affected, err := repository.FailStaleCanvasImageTasks(cutoff, "图片任务超时未完成")
	if err != nil {
		log.Printf("recover stale canvas image tasks failed err=%v", err)
		return
	}
	if affected > 0 {
		log.Printf("recover stale canvas image tasks finished: tasks=%d", affected)
	}
}
