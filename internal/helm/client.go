// Package helm wraps the helm CLI as a subprocess for upgrading the Karpenter
// controller chart. A CLI wrapper (rather than embedding the Helm Go SDK) keeps this
// application's dependency surface small while still being fully mockable behind the
// Upgrader interface.
package helm

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// UpgradeOptions configures a single `helm upgrade --install` invocation.
type UpgradeOptions struct {
	ReleaseName     string
	Namespace       string
	ChartRepository string
	ChartName       string
	ChartVersion    string
	Wait            bool
	Timeout         time.Duration
	Values          map[string]string // rendered as --set key=value
}

// Result captures the outcome of a single helm invocation.
type Result struct {
	Args     []string
	Stdout   string
	Stderr   string
	ExitCode int
}

// Upgrader installs or upgrades a Helm release. The sole production implementation
// shells out to the helm CLI; tests use a fake.
type Upgrader interface {
	Upgrade(ctx context.Context, opts UpgradeOptions) (*Result, error)
}

// CLIClient shells out to a real helm binary.
type CLIClient struct {
	BinaryPath string // defaults to "helm"
}

func (c *CLIClient) binary() string {
	if c.BinaryPath == "" {
		return "helm"
	}
	return c.BinaryPath
}

// BuildUpgradeArgs constructs the helm CLI argument list for opts. Exported separately
// from Upgrade so the exact command can be printed in --dry-run output.
func BuildUpgradeArgs(opts UpgradeOptions) []string {
	args := []string{
		"upgrade", opts.ReleaseName, opts.ChartName,
		"--install",
		"--repo", opts.ChartRepository,
		"--namespace", opts.Namespace,
		"--create-namespace",
	}
	if opts.ChartVersion != "" {
		args = append(args, "--version", opts.ChartVersion)
	}
	if opts.Wait {
		args = append(args, "--wait")
	}
	if opts.Timeout > 0 {
		args = append(args, "--timeout", opts.Timeout.String())
	}
	for k, v := range opts.Values {
		args = append(args, "--set", k+"="+v)
	}
	return args
}

func (c *CLIClient) Upgrade(ctx context.Context, opts UpgradeOptions) (*Result, error) {
	args := BuildUpgradeArgs(opts)
	cmd := exec.CommandContext(ctx, c.binary(), args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := &Result{Args: append([]string{c.binary()}, args...), Stdout: stdout.String(), Stderr: stderr.String()}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		return result, fmt.Errorf("helm upgrade exited with code %s: %s", strconv.Itoa(result.ExitCode), stderr.String())
	}
	if err != nil {
		return result, fmt.Errorf("executing helm upgrade: %w", err)
	}
	return result, nil
}
