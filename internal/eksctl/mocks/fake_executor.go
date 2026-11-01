// Package mocks provides a hand-written fake Executor for tests, avoiding real
// subprocess execution against a live eksctl binary.
package mocks

import (
	"context"
	"fmt"
	"sync"

	"eks-upgrader/internal/eksctl"
)

// FakeExecutor is a scriptable, call-recording implementation of eksctl.Executor.
type FakeExecutor struct {
	mu sync.Mutex

	// Responses maps a joined-args key (see Key) to a canned result/error. If no entry
	// matches, DefaultResult/DefaultErr are used.
	Responses     map[string]*eksctl.CommandResult
	ResponseErrs  map[string]error
	DefaultResult *eksctl.CommandResult
	DefaultErr    error

	VersionString string
	VersionErr    error

	Calls [][]string
}

func NewFakeExecutor() *FakeExecutor {
	return &FakeExecutor{
		Responses:     make(map[string]*eksctl.CommandResult),
		ResponseErrs:  make(map[string]error),
		DefaultResult: &eksctl.CommandResult{ExitCode: 0, Stdout: "ok"},
		VersionString: "0.200.0",
	}
}

// Key joins args with spaces to use as a lookup key for scripted responses.
func Key(args []string) string {
	key := ""
	for i, a := range args {
		if i > 0 {
			key += " "
		}
		key += a
	}
	return key
}

func (f *FakeExecutor) Run(ctx context.Context, args []string) (*eksctl.CommandResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, append([]string(nil), args...))

	key := Key(args)
	if err, ok := f.ResponseErrs[key]; ok {
		return f.Responses[key], err
	}
	if res, ok := f.Responses[key]; ok {
		return res, nil
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("context error: %w", ctx.Err())
	}
	return f.DefaultResult, f.DefaultErr
}

func (f *FakeExecutor) Version(ctx context.Context) (string, error) {
	return f.VersionString, f.VersionErr
}
