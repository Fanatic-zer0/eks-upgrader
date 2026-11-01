package lock

import (
	"context"
	"errors"
	"time"
)

// ErrHeld is returned by Acquire when another valid (non-expired) lock is already held.
var ErrHeld = errors.New("lock is already held by another run")

// ErrNotHeld is returned by Release/ForceRelease when no lock exists for the cluster.
var ErrNotHeld = errors.New("no lock is currently held for this cluster")

// Lock represents a held distributed lock for a single cluster.
type Lock struct {
	Cluster    string    `json:"cluster"`
	RunID      string    `json:"runID"`
	Owner      string    `json:"owner"`
	AcquiredAt time.Time `json:"acquiredAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// Expired reports whether the lock's TTL has elapsed.
func (l *Lock) Expired(now time.Time) bool {
	return now.After(l.ExpiresAt)
}

// Locker prevents concurrent upgrades against the same cluster. Implementations must
// be safe to call from CI, a Kubernetes Job, or an operator workstation.
type Locker interface {
	// Acquire takes the lock for cluster, failing with ErrHeld if another non-expired
	// lock already exists.
	Acquire(ctx context.Context, cluster, runID, owner string, ttl time.Duration) (*Lock, error)
	// Release releases the lock only if it is currently held by runID.
	Release(ctx context.Context, cluster, runID string) error
	// ForceRelease unconditionally releases the lock regardless of owner; callers must
	// have already confirmed no run process is actually active.
	ForceRelease(ctx context.Context, cluster string) error
	// Status returns the current lock for cluster, or nil if none is held.
	Status(ctx context.Context, cluster string) (*Lock, error)
}
