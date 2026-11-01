package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger emits immutable audit events. Implementations must be append-only and safe
// for concurrent use.
type Logger interface {
	Emit(evt Event) error
	Close() error
}

// FileLogger appends newline-delimited JSON audit events to a local file.
type FileLogger struct {
	mu   sync.Mutex
	file *os.File
}

// NewFileLogger opens (creating if necessary) an append-only audit log file.
func NewFileLogger(path string) (*FileLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating audit log directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, fmt.Errorf("opening audit log %q: %w", path, err)
	}
	return &FileLogger{file: f}, nil
}

// Emit writes a single audit event as one JSON line, fsyncing to guarantee durability.
func (l *FileLogger) Emit(evt Event) error {
	if evt.Time.IsZero() {
		evt.Time = time.Now().UTC()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	enc := json.NewEncoder(l.file)
	if err := enc.Encode(evt); err != nil {
		return fmt.Errorf("writing audit event: %w", err)
	}
	return l.file.Sync()
}

func (l *FileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// NopLogger discards all events; useful for dry-run or tests.
type NopLogger struct{}

func (NopLogger) Emit(Event) error { return nil }
func (NopLogger) Close() error     { return nil }
