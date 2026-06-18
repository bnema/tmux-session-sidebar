package process

import (
	"bytes"
	"context"
	"os/exec"

	"github.com/bnema/tmux-session-sidebar/internal/ports"
)

type Runner struct{}

func (Runner) Exec(ctx context.Context, name string, args []string) (ports.Result, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := ports.Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	return result, err
}
