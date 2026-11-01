package lock

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LocalLocker implements Locker using a JSON file per cluster on local disk. It is
// suitable for a single operator workstation or a single CI runner; use the DynamoDB
// backend for safety across multiple concurrent CI runners.
type LocalLocker struct {
	mu  sync.Mutex
	dir string
}

// NewLocalLocker returns a LocalLocker that stores lock files under dir.
func NewLocalLocker(dir string) *LocalLocker {
	return &LocalLocker{dir: dir}
}

func (l *LocalLocker) path(cluster string) string {
	return filepath.Join(l.dir, fmt.Sprintf("%s.lock.json", cluster))
}

func (l *LocalLocker) readLock(cluster string) (*Lock, error) {
	data, err := os.ReadFile(l.path(cluster))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading lock file: %w", err)
	}
	var lk Lock
	if err := json.Unmarshal(data, &lk); err != nil {
		return nil, fmt.Errorf("parsing lock file: %w", err)
	}
	return &lk, nil
}

func (l *LocalLocker) writeLock(lk *Lock) error {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return fmt.Errorf("creating lock directory: %w", err)
	}
	data, err := json.MarshalIndent(lk, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling lock: %w", err)
	}
	return os.WriteFile(l.path(lk.Cluster), data, 0o640)
}

func (l *LocalLocker) Acquire(ctx context.Context, cluster, runID, owner string, ttl time.Duration) (*Lock, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	existing, err := l.readLock(cluster)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if existing != nil && !existing.Expired(now) && existing.RunID != runID {
		return nil, fmt.Errorf("%w: held by runID=%s owner=%s since %s, expires %s",
			ErrHeld, existing.RunID, existing.Owner, existing.AcquiredAt.Format(time.RFC3339), existing.ExpiresAt.Format(time.RFC3339))
	}

	lk := &Lock{
		Cluster:    cluster,
		RunID:      runID,
		Owner:      owner,
		AcquiredAt: now,
		ExpiresAt:  now.Add(ttl),
	}
	// Preserve the original acquisition time when the same run (e.g. `resume`) refreshes
	// its own lock, so lock age reflects the whole run rather than resetting per resume.
	if existing != nil && existing.RunID == runID {
		lk.AcquiredAt = existing.AcquiredAt
	}
	if err := l.writeLock(lk); err != nil {
		return nil, err
	}
	return lk, nil
}

func (l *LocalLocker) Release(ctx context.Context, cluster, runID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	existing, err := l.readLock(cluster)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrNotHeld
	}
	if existing.RunID != runID {
		return fmt.Errorf("lock for cluster %q is held by a different run (%s), refusing to release", cluster, existing.RunID)
	}
	return os.Remove(l.path(cluster))
}

func (l *LocalLocker) ForceRelease(ctx context.Context, cluster string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	existing, err := l.readLock(cluster)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrNotHeld
	}
	return os.Remove(l.path(cluster))
}

func (l *LocalLocker) Status(ctx context.Context, cluster string) (*Lock, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.readLock(cluster)
}
