package lock

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLocalLocker_AcquireReleaseLifecycle(t *testing.T) {
	ctx := context.Background()
	l := NewLocalLocker(t.TempDir())

	lk, err := l.Acquire(ctx, "prod-cluster", "run-1", "ci", time.Hour)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if lk.RunID != "run-1" {
		t.Fatalf("unexpected runID: %s", lk.RunID)
	}

	if _, err := l.Acquire(ctx, "prod-cluster", "run-2", "ci", time.Hour); !errors.Is(err, ErrHeld) {
		t.Fatalf("expected ErrHeld for concurrent acquire, got %v", err)
	}

	if err := l.Release(ctx, "prod-cluster", "run-2"); err == nil {
		t.Fatal("expected error releasing lock owned by a different run")
	}

	if err := l.Release(ctx, "prod-cluster", "run-1"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	status, err := l.Status(ctx, "prod-cluster")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != nil {
		t.Fatalf("expected no lock after release, got %+v", status)
	}
}

func TestLocalLocker_ExpiredLockCanBeStolen(t *testing.T) {
	ctx := context.Background()
	l := NewLocalLocker(t.TempDir())

	if _, err := l.Acquire(ctx, "prod-cluster", "run-1", "ci", -time.Minute); err != nil {
		t.Fatalf("Acquire (pre-expired): %v", err)
	}

	lk, err := l.Acquire(ctx, "prod-cluster", "run-2", "ci", time.Hour)
	if err != nil {
		t.Fatalf("expected Acquire to succeed over an expired lock, got %v", err)
	}
	if lk.RunID != "run-2" {
		t.Fatalf("unexpected runID: %s", lk.RunID)
	}
}

func TestLocalLocker_SameRunCanReacquireItsOwnLock(t *testing.T) {
	ctx := context.Background()
	l := NewLocalLocker(t.TempDir())

	first, err := l.Acquire(ctx, "prod-cluster", "run-1", "ci", time.Hour)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Simulate `resume` refreshing the lock it already owns.
	second, err := l.Acquire(ctx, "prod-cluster", "run-1", "ci", 2*time.Hour)
	if err != nil {
		t.Fatalf("expected resume to reacquire its own lock, got %v", err)
	}
	if !second.AcquiredAt.Equal(first.AcquiredAt) {
		t.Fatalf("expected AcquiredAt to be preserved across resume, got %v want %v", second.AcquiredAt, first.AcquiredAt)
	}
}

func TestLocalLocker_ForceRelease(t *testing.T) {
	ctx := context.Background()
	l := NewLocalLocker(t.TempDir())

	if _, err := l.Acquire(ctx, "prod-cluster", "run-1", "ci", time.Hour); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := l.ForceRelease(ctx, "prod-cluster"); err != nil {
		t.Fatalf("ForceRelease: %v", err)
	}
	if err := l.ForceRelease(ctx, "prod-cluster"); !errors.Is(err, ErrNotHeld) {
		t.Fatalf("expected ErrNotHeld on second ForceRelease, got %v", err)
	}
}
