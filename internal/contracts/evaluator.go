package contracts

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// CommandExecutor abstracts shell execution for testing.
type CommandExecutor interface {
	Execute(ctx context.Context, command string) (stdout string, exitCode int, err error)
}

// DefaultExecutor runs commands via sh -c.
type DefaultExecutor struct{}

func (e *DefaultExecutor) Execute(ctx context.Context, command string) (string, int, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			err = nil // non-zero exit is not an execution error
		}
	}
	return strings.TrimSpace(stdout.String()), exitCode, err
}

// EvalOptions carries runtime attribution metadata for a contract evaluation run.
type EvalOptions struct {
	RunID        string
	Actor        string
	EvaluationID string
}

// WriteLog writes check results to the given writer in a greppable format.
func WriteLog(result RunResult, w io.Writer) {
	ts := result.Timestamp.Format(time.RFC3339)
	if ts == "0001-01-01T00:00:00Z" {
		ts = time.Now().Format(time.RFC3339)
	}

	for _, cr := range result.Results {
		status := "PASS"
		if cr.Status == "warn" {
			status = "WARN"
		} else if !cr.Passed {
			status = "FAIL"
		}
		meta := ""
		if result.RunID != "" || result.Actor != "" {
			meta = fmt.Sprintf(" run_id=%s actor=%s", result.RunID, result.Actor)
		}
		line := fmt.Sprintf("%s [healthcheck] %s %s %s (%dms)%s\n",
			ts, cr.ContractID, status, cr.CheckName, cr.Duration.Milliseconds(), meta)
		fmt.Fprint(w, line)
	}

	fmt.Fprintf(w, "%s [healthcheck] summary: %d passed, %d failed, %d warned, %d unknown, %d skipped eval=%s\n",
		ts, result.Passed, result.Failed, result.Warned, result.Unknown, result.Skipped, result.EvaluationID)
}

func newEvaluationID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("eval-%d", time.Now().UnixNano())
	}
	return "eval-" + hex.EncodeToString(b[:])
}
