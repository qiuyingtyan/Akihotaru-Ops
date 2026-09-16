package collect

import (
	"context"
	"io"
	"os/exec"
	"time"
)

const maxOutputBytes = 4 << 20 // 4MB cap on captured command output

func run(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var pipe io.Reader
	if p, err := cmd.StdoutPipe(); err == nil {
		pipe = p
		cmd.Stderr = cmd.Stdout
	} else {
		pipe = nil
	}
	if pipe == nil {
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	limited := io.LimitReader(pipe, maxOutputBytes)
	data, readErr := io.ReadAll(limited)
	waitErr := cmd.Wait()
	if readErr != nil {
		return string(data), readErr
	}
	return string(data), waitErr
}
