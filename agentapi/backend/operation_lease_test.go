package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openSharedLeaseStores(t *testing.T) (*Store, *Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agentapi.db")
	first, err := OpenStore(path, "shared-lease-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenStore(path, "shared-lease-test-secret")
	if err != nil {
		_ = first.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = second.Close()
		_ = first.Close()
	})
	return first, second
}

func TestOperationLeaseCoordinatesIndependentStoreInstances(t *testing.T) {
	first, second := openSharedLeaseStores(t)
	const key = "settlement:user:42"
	if err := first.AcquireOperationLease(context.Background(), key, "owner-a", 5*time.Second); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	waitCtx, cancelWait := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancelWait()
	err := second.AcquireOperationLease(waitCtx, key, "owner-b", 5*time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second store bypassed active lease: %v", err)
	}

	if err := first.ReleaseOperationLease(context.Background(), key, "owner-a"); err != nil {
		t.Fatalf("release first lease: %v", err)
	}
	if err := second.AcquireOperationLease(context.Background(), key, "owner-b", 5*time.Second); err != nil {
		t.Fatalf("second acquire after release: %v", err)
	}
	if err := second.ReleaseOperationLease(context.Background(), key, "owner-b"); err != nil {
		t.Fatalf("release second lease: %v", err)
	}
}

func TestOperationLeaseExpiryAndOwnerCheckedRelease(t *testing.T) {
	first, second := openSharedLeaseStores(t)
	const key = "settlement:user:84"
	now := time.Unix(1_800_000_000, 0).UTC()
	first.clock = func() time.Time { return now }
	second.clock = func() time.Time { return now }

	if err := first.AcquireOperationLease(context.Background(), key, "stale-owner", time.Second); err != nil {
		t.Fatalf("initial acquire: %v", err)
	}
	now = now.Add(2 * time.Second)
	if err := second.AcquireOperationLease(context.Background(), key, "successor", time.Second); err != nil {
		t.Fatalf("take over expired lease: %v", err)
	}

	// A delayed cleanup from the old process must not delete its successor.
	if err := first.ReleaseOperationLease(context.Background(), key, "stale-owner"); err != nil {
		t.Fatalf("release stale owner: %v", err)
	}
	waitCtx, cancelWait := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancelWait()
	err := first.AcquireOperationLease(waitCtx, key, "third-owner", time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stale owner release removed successor lease: %v", err)
	}

	if err := second.ReleaseOperationLease(context.Background(), key, "successor"); err != nil {
		t.Fatalf("release successor: %v", err)
	}
}

func TestOperationLeaseRenewalRequiresCurrentOwner(t *testing.T) {
	first, second := openSharedLeaseStores(t)
	const key = "settlement:user:126"
	if err := first.AcquireOperationLease(context.Background(), key, "owner-a", time.Second); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	owned, err := second.RenewOperationLease(context.Background(), key, "owner-b", time.Second)
	if err != nil {
		t.Fatalf("renew wrong owner: %v", err)
	}
	if owned {
		t.Fatal("non-owner renewed operation lease")
	}
	owned, err = first.RenewOperationLease(context.Background(), key, "owner-a", time.Second)
	if err != nil {
		t.Fatalf("renew owner: %v", err)
	}
	if !owned {
		t.Fatal("current owner could not renew operation lease")
	}
}
