package repository

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"infinite-canvas/backend/internal/model"
)

func TestSSOTicketSurvivesReopenAndConcurrentConsumption(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "replay.db") + "?_busy_timeout=5000&_journal_mode=WAL"
	open := func() *gorm.DB {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		return db
	}
	first := open()
	if err := first.AutoMigrate(&model.SSOTicket{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := New(first).ConsumeSSOTicket("persisted-digest", now.Add(time.Minute), now); err != nil {
		t.Fatal(err)
	}
	pool, err := first.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err = pool.Close(); err != nil {
		t.Fatal(err)
	}
	second := open()
	pool, err = second.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	repo := New(second)
	if err := repo.ConsumeSSOTicket("persisted-digest", now.Add(time.Minute), now); err == nil {
		t.Fatal("reopened database accepted replay")
	}
	var accepted atomic.Int32
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			if repo.ConsumeSSOTicket("concurrent-digest", now.Add(time.Minute), now) == nil {
				accepted.Add(1)
			}
		}()
	}
	group.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("concurrent successes = %d", accepted.Load())
	}
	if err := repo.ConsumeSSOTicket("new-digest", now.Add(3*time.Minute), now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := second.Model(&model.SSOTicket{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("expired records were not removed")
	}
}
