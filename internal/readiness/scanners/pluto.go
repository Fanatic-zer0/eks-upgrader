package scanners

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"eks-upgrader/internal/readiness"
)

// ExternalToolRunner abstracts subprocess execution so tests can substitute a fake
// without a real pluto/kubent binary installed.
type ExternalToolRunner interface {
	Run(ctx context.Context, binary string, args []string) (stdout []byte, err error)
}

// SubprocessRunner shells out to a real binary on $PATH.
type SubprocessRunner struct{}

func (SubprocessRunner) Run(ctx context.Context, binary string, args []string) ([]byte, error) {
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("%w: %s not found on PATH", ErrToolNotInstalled, binary)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Both pluto and kubent conventionally print JSON findings to stdout and may exit
	// non-zero purely because findings were detected (not a tool failure), so stdout is
	// trusted and a non-zero exit is not itself treated as an error here.
	_ = cmd.Run()
	return stdout.Bytes(), nil
}

// ErrToolNotInstalled is returned (and should be treated as a skip, not a failure) when
// the external scanner binary isn't available in the environment.
var ErrToolNotInstalled = fmt.Errorf("external scanner binary not installed")

// plutoFinding is a best-effort subset of pluto's `-o json` output schema; verify
// against the installed pluto version, as the schema has changed across releases.
type plutoFinding struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Kind           string `json:"kind"`
	APIVersion     string `json:"api-version"`
	ReplacementAPI string `json:"replacement-api"`
	RemovedIn      string `json:"removed-in"`
	Deprecated     bool   `json:"deprecated"`
	Removed        bool   `json:"removed"`
}

type plutoOutput struct {
	Items []plutoFinding `json:"items"`
}

// ScanWithPluto runs `pluto detect-files <path> -o json --target-versions k8s=v<target>`
// against manifestPaths. If pluto is not installed, it returns (nil, nil) so the
// caller can treat this signal as skipped rather than failing readiness outright.
func ScanWithPluto(ctx context.Context, runner ExternalToolRunner, manifestPaths []string, targetK8sVersion string) ([]readiness.DeprecatedAPIFinding, error) {
	var findings []readiness.DeprecatedAPIFinding
	for _, path := range manifestPaths {
		args := []string{"detect-files", path, "-o", "json", "--target-versions", "k8s=v" + targetK8sVersion}
		out, err := runner.Run(ctx, "pluto", args)
		if err != nil {
			if errors.Is(err, ErrToolNotInstalled) {
				return nil, nil
			}
			return nil, fmt.Errorf("running pluto against %q: %w", path, err)
		}
		if len(out) == 0 {
			continue
		}

		var parsed plutoOutput
		if err := json.Unmarshal(out, &parsed); err != nil {
			// Best-effort signal only; a parse failure degrades gracefully rather than
			// failing the whole readiness analysis.
			continue
		}
		for _, item := range parsed.Items {
			if !item.Deprecated && !item.Removed {
				continue
			}
			f := readiness.DeprecatedAPIFinding{
				Source: "pluto", ResourceKind: item.Kind, ResourceName: item.Name,
				APIVersion: item.APIVersion, Replacement: item.ReplacementAPI,
			}
			if item.Removed {
				f.RemovedInVersion = item.RemovedIn
			}
			findings = append(findings, f)
		}
	}
	return findings, nil
}
