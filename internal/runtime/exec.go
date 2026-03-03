package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// maxOutputSize limits stdout capture to prevent OOM from runaway CLI output.
const maxOutputSize = 1 << 20 // 1MB

// Exec runs an agent using an external CLI binary.
// The prompt is passed via stdin. The response is read from stdout.
type Exec struct {
	Cmd       string
	Args      []string
	Workspace string
}

// Invoke runs the configured CLI, passing prompt via stdin and capturing stdout.
// sessionKey is accepted for interface compatibility but not forwarded — external
// CLIs manage their own session state.
func (e *Exec) Invoke(ctx context.Context, prompt, sessionKey string) (string, error) {
	cmd := exec.CommandContext(ctx, e.Cmd, e.Args...)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Dir = e.Workspace
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Kill entire process group on context cancellation (child processes survive
	// a regular SIGKILL to the parent).
	if ctx.Err() != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("exec runtime %s: %w\nstderr: %s", e.Cmd, err, stderr.String())
		}
		return "", fmt.Errorf("exec runtime %s: %w", e.Cmd, err)
	}

	output := stdout.Bytes()
	if len(output) > maxOutputSize {
		output = output[:maxOutputSize]
	}
	return string(output), nil
}
