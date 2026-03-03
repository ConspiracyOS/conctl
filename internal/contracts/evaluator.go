package contracts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
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

// Evaluate runs all detective checks. Preventive contracts are skipped.
func Evaluate(ctx context.Context, contracts []Contract, contractsDir string, executor CommandExecutor) RunResult {
	result := RunResult{
		Timestamp: time.Now(),
	}

	for _, c := range contracts {
		if c.Type == "preventive" {
			result.Skipped++
			continue
		}

		for _, ch := range c.Checks {
			cr := runCheck(ctx, c.ID, ch, contractsDir, executor)
			result.Results = append(result.Results, cr)
			if cr.Passed {
				result.Passed++
			} else {
				result.Failed++
			}
		}
	}

	return result
}

// runCheck executes a single check (command or script).
func runCheck(ctx context.Context, contractID string, ch Check, contractsDir string, executor CommandExecutor) CheckResult {
	start := time.Now()

	cr := CheckResult{
		ContractID: contractID,
		CheckName:  ch.Name,
	}

	var command string
	var checkCtx context.Context
	var cancel context.CancelFunc

	if ch.Command != nil {
		if ch.Command.Test != "" {
			// Old format: combine run + test via $RESULT variable
			command = fmt.Sprintf("RESULT=$(%s); %s", ch.Command.Run, ch.Command.Test)
		} else {
			// New format: run command is self-contained, exit code signals pass/fail
			command = ch.Command.Run
		}
		checkCtx = ctx
	} else if ch.Script != nil {
		// Script: resolve path, apply timeout
		scriptPath := ch.Script.Path
		if !filepath.IsAbs(scriptPath) {
			scriptPath = filepath.Join(contractsDir, scriptPath)
		}
		command = "sh " + scriptPath

		if ch.Script.Timeout != "" {
			if d, err := time.ParseDuration(ch.Script.Timeout); err == nil {
				checkCtx, cancel = context.WithTimeout(ctx, d)
				defer cancel()
			} else {
				checkCtx = ctx
			}
		} else {
			checkCtx = ctx
		}
	}

	stdout, exitCode, err := executor.Execute(checkCtx, command)
	cr.Duration = time.Since(start)
	cr.Output = stdout

	if err != nil {
		cr.Passed = false
		cr.Error = err
	} else {
		cr.Passed = exitCode == 0
	}

	return cr
}

// WriteLog writes check results to the given writer in a greppable format.
func WriteLog(result RunResult, w io.Writer) {
	ts := result.Timestamp.Format(time.RFC3339)
	if ts == "0001-01-01T00:00:00Z" {
		ts = time.Now().Format(time.RFC3339)
	}

	for _, cr := range result.Results {
		status := "PASS"
		if !cr.Passed {
			status = "FAIL"
		}
		line := fmt.Sprintf("%s [healthcheck] %s %s %s (%dms)\n",
			ts, cr.ContractID, status, cr.CheckName, cr.Duration.Milliseconds())
		fmt.Fprint(w, line)
	}

	fmt.Fprintf(w, "%s [healthcheck] summary: %d passed, %d failed, %d skipped\n",
		ts, result.Passed, result.Failed, result.Skipped)
}
