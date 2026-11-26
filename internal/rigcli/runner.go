// Package rigcli runs the external rig binary as a subprocess.
package rigcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ErrRigNotFound is returned when the binary is not on PATH.
var ErrRigNotFound = errors.New("rig binary not found on PATH")

// Output is what a finished command produced. A non-zero exit is not an error.
type Output struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Runner runs one command given as an argument vector.
type Runner interface {
	Run(ctx context.Context, argv []string) (Output, error)
}

// ExecRunner runs argv[0] (found on PATH) directly, without a shell.
type ExecRunner struct {
	Timeout time.Duration
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, argv []string) (Output, error) {
	if len(argv) == 0 {
		return Output{}, errors.New("rigcli: empty argv")
	}
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return Output{}, fmt.Errorf("%w: %s: %v", ErrRigNotFound, argv[0], err)
	}
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, path, argv[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second
	runErr := cmd.Run()
	out := Output{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return out, fmt.Errorf("rigcli: %s: %w", strings.Join(argv, " "), ctxErr)
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		out.ExitCode = exitErr.ExitCode()
		return out, nil
	}
	if runErr != nil {
		return out, fmt.Errorf("rigcli: %s: %w", strings.Join(argv, " "), runErr)
	}
	return out, nil
}
