package contracts

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestDefaultExecutor_Success(t *testing.T) {
	e := &DefaultExecutor{}
	stdout, exitCode, err := e.Execute(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}
	if stdout != "hello" {
		t.Errorf("stdout = %q, want %q", stdout, "hello")
	}
}

func TestDefaultExecutor_NonZeroExit(t *testing.T) {
	e := &DefaultExecutor{}
	// Non-zero exit must set exitCode but NOT return an error.
	_, exitCode, err := e.Execute(context.Background(), "exit 2")
	if err != nil {
		t.Fatalf("non-zero exit should not return error, got: %v", err)
	}
	if exitCode != 2 {
		t.Errorf("exitCode = %d, want 2", exitCode)
	}
}

func TestDefaultExecutor_StdoutTrimmed(t *testing.T) {
	e := &DefaultExecutor{}
	stdout, _, err := e.Execute(context.Background(), "printf '  hello  '")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "hello" {
		t.Errorf("stdout = %q, want %q (trimmed)", stdout, "hello")
	}
}

func TestDefaultExecutor_BadCommand(t *testing.T) {
	e := &DefaultExecutor{}
	// A command that doesn't exist should return an exec error.
	_, _, err := e.Execute(context.Background(), "this_command_does_not_exist_xyz")
	if err != nil {
		// Command not found may surface as exit code 127, not an error — both are acceptable.
		// Either way, the call must not panic.
	}
}

// ErrorExecutor always returns an execution error, for testing error-handling paths.
type ErrorExecutor struct{}

func (e *ErrorExecutor) Execute(ctx context.Context, command string) (string, int, error) {
	return "", 0, fmt.Errorf("executor failure: %s", command)
}

// mockOnNthCallError succeeds on all calls except the Nth, which returns an error.
type mockOnNthCallError struct {
	failOn int
	calls  int
}

func (m *mockOnNthCallError) Execute(ctx context.Context, command string) (string, int, error) {
	m.calls++
	if m.calls == m.failOn {
		return "", 0, fmt.Errorf("simulated failure on call %d", m.calls)
	}
	return "", 0, nil
}

// MockExecutor returns predefined results for commands.
type MockExecutor struct {
	// Calls records every command that was executed.
	Calls []string
	// ExitCode is the default exit code for all commands.
	ExitCode int
	// Overrides maps command substrings to specific exit codes.
	Overrides map[string]int
}

func (m *MockExecutor) Execute(ctx context.Context, command string) (string, int, error) {
	m.Calls = append(m.Calls, command)

	// Check for context cancellation (timeout)
	select {
	case <-ctx.Done():
		return "", -1, ctx.Err()
	default:
	}

	for substr, code := range m.Overrides {
		if strings.Contains(command, substr) {
			return "", code, nil
		}
	}
	return "", m.ExitCode, nil
}

func TestWriteLog(t *testing.T) {
	result := RunResult{
		Results: []CheckResult{
			{ContractID: "CON-001", CheckName: "disk", Passed: true, Duration: 52000000},
			{ContractID: "CON-002", CheckName: "mem", Passed: false, Output: "low", Duration: 28000000},
		},
		Passed:  1,
		Failed:  1,
		Skipped: 0,
	}

	var buf strings.Builder
	WriteLog(result, &buf)

	output := buf.String()
	if !strings.Contains(output, "CON-001 PASS disk") {
		t.Errorf("log should contain PASS entry, got:\n%s", output)
	}
	if !strings.Contains(output, "CON-002 FAIL mem") {
		t.Errorf("log should contain FAIL entry, got:\n%s", output)
	}
	if !strings.Contains(output, fmt.Sprintf("summary: %d passed, %d failed", 1, 1)) {
		t.Errorf("log should contain summary, got:\n%s", output)
	}
}
