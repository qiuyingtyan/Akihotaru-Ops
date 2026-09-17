package ai

import (
	"context"
	"io"
	"os/exec"
	"time"
)

const shellMaxOutputBytes = 4 << 20 // 4MB cap on captured command output

// ShellRequest describes one guarded shell execution.
type ShellRequest struct {
	Command string
	Timeout time.Duration
}

// runGuarded executes an approved shell command with a shape-based timeout.
func runGuarded(cmd string) (string, error) {
	return RunShell(ShellRequest{Command: cmd, Timeout: shellExecTimeout(cmd)})
}

// RunShell executes a command with combined-output capped at 4MB.
func RunShell(r ShellRequest) (string, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-lc", r.Command)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return "", err
	}
	data, readErr := io.ReadAll(io.LimitReader(pipe, shellMaxOutputBytes))
	waitErr := cmd.Wait()
	if readErr != nil {
		return string(data), readErr
	}
	return string(data), waitErr
}
