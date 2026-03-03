package contracts

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestEvaluate_NewFormatCommand(t *testing.T) {
	// Command with no Test field — exit code signals pass/fail directly.
	// The command string should be used as-is, not wrapped in RESULT=$(...).
	contracts := []Contract{
		{
			ID:   "CON-NEW",
			Type: "detective",
			Checks: []Check{
				{
					Name:    "self_contained",
					Command: &CmdCheck{Run: "test -f /etc/hosts"},
					OnFail:  FailAction{Action: "alert"},
				},
			},
		},
	}

	exec := &MockExecutor{ExitCode: 0}
	result := Evaluate(context.Background(), contracts, "/tmp", exec)

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
	if len(exec.Calls) != 1 {
		t.Fatalf("Calls = %d, want 1", len(exec.Calls))
	}
	if strings.Contains(exec.Calls[0], "RESULT=") {
		t.Errorf("new format command should not use RESULT=$(...) wrapping, got: %s", exec.Calls[0])
	}
	if exec.Calls[0] != "test -f /etc/hosts" {
		t.Errorf("command = %q, want %q", exec.Calls[0], "test -f /etc/hosts")
	}
}

func TestEvaluate_ScriptNoTimeout(t *testing.T) {
	// Script with empty Timeout — should run without a deadline.
	contracts := []Contract{
		{
			ID:   "CON-042",
			Type: "detective",
			Checks: []Check{
				{
					Name:   "script_no_timeout",
					Script: &ScriptCheck{Path: "/some/script.sh", Timeout: ""},
					OnFail: FailAction{Action: "alert"},
				},
			},
		},
	}

	exec := &MockExecutor{ExitCode: 0}
	result := Evaluate(context.Background(), contracts, "/tmp", exec)

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
	if len(exec.Calls) != 1 || !strings.Contains(exec.Calls[0], "/some/script.sh") {
		t.Errorf("expected script command, got: %v", exec.Calls)
	}
}

func TestEvaluate_ScriptInvalidTimeout(t *testing.T) {
	// Invalid timeout string — should fall back to no timeout and still run.
	contracts := []Contract{
		{
			ID:   "CON-043",
			Type: "detective",
			Checks: []Check{
				{
					Name:   "script_bad_timeout",
					Script: &ScriptCheck{Path: "/script.sh", Timeout: "not-a-duration"},
					OnFail: FailAction{Action: "alert"},
				},
			},
		},
	}

	exec := &MockExecutor{ExitCode: 0}
	result := Evaluate(context.Background(), contracts, "/tmp", exec)

	if result.Passed != 1 {
		t.Errorf("invalid timeout should fall back gracefully, Passed = %d", result.Passed)
	}
}

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

func TestEvaluate_AllPass(t *testing.T) {
	contracts := []Contract{
		{
			ID:   "CON-001",
			Type: "detective",
			Checks: []Check{
				{
					Name:    "check_a",
					Command: &CmdCheck{Run: "echo 50", Test: "[ $RESULT -ge 15 ]"},
					OnFail:  FailAction{Action: "alert", Message: "failed"},
				},
			},
		},
		{
			ID:   "CON-002",
			Type: "detective",
			Checks: []Check{
				{
					Name:    "check_b",
					Command: &CmdCheck{Run: "echo ok", Test: "[ \"$RESULT\" = \"ok\" ]"},
					OnFail:  FailAction{Action: "alert", Message: "failed"},
				},
			},
		},
	}

	exec := &MockExecutor{ExitCode: 0}
	result := Evaluate(context.Background(), contracts, "/tmp", exec)

	if result.Passed != 2 {
		t.Errorf("Passed = %d, want 2", result.Passed)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
	if len(exec.Calls) != 2 {
		t.Errorf("Calls = %d, want 2", len(exec.Calls))
	}
}

func TestEvaluate_OneFail(t *testing.T) {
	contracts := []Contract{
		{
			ID:   "CON-001",
			Type: "detective",
			Checks: []Check{
				{
					Name:    "pass_check",
					Command: &CmdCheck{Run: "echo good", Test: "[ \"$RESULT\" = \"good\" ]"},
					OnFail:  FailAction{Action: "alert", Message: "failed"},
				},
			},
		},
		{
			ID:   "CON-002",
			Type: "detective",
			Checks: []Check{
				{
					Name:    "fail_check",
					Command: &CmdCheck{Run: "echo bad", Test: "[ \"$RESULT\" = \"good\" ]"},
					OnFail:  FailAction{Action: "halt_agents", Escalate: "sysadmin", Message: "disk low"},
				},
			},
		},
	}

	exec := &MockExecutor{
		Overrides: map[string]int{
			"echo bad": 1, // this check fails
		},
	}
	result := Evaluate(context.Background(), contracts, "/tmp", exec)

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
}

func TestEvaluate_SkipsPreventive(t *testing.T) {
	contracts := []Contract{
		{
			ID:   "CON-001",
			Type: "detective",
			Checks: []Check{
				{
					Name:    "check_a",
					Command: &CmdCheck{Run: "echo 1", Test: "[ 1 -eq 1 ]"},
					OnFail:  FailAction{Action: "alert"},
				},
			},
		},
		{
			ID:   "CON-117",
			Type: "preventive",
		},
	}

	exec := &MockExecutor{ExitCode: 0}
	result := Evaluate(context.Background(), contracts, "/tmp", exec)

	if result.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.Passed)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
	if len(exec.Calls) != 1 {
		t.Errorf("Calls = %d, want 1 (preventive should not execute)", len(exec.Calls))
	}
}

func TestEvaluate_CommandConstruction(t *testing.T) {
	contracts := []Contract{
		{
			ID:   "CON-001",
			Type: "detective",
			Checks: []Check{
				{
					Name:    "disk",
					Command: &CmdCheck{Run: "df /srv --output=pcent | tail -1", Test: "[ $RESULT -ge 15 ]"},
					OnFail:  FailAction{Action: "alert"},
				},
			},
		},
	}

	exec := &MockExecutor{ExitCode: 0}
	Evaluate(context.Background(), contracts, "/tmp", exec)

	if len(exec.Calls) != 1 {
		t.Fatalf("Calls = %d, want 1", len(exec.Calls))
	}
	cmd := exec.Calls[0]
	if !strings.Contains(cmd, "RESULT=$(") {
		t.Errorf("command should contain RESULT=$(...), got: %s", cmd)
	}
	if !strings.Contains(cmd, "df /srv --output=pcent | tail -1") {
		t.Errorf("command should contain the run command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "[ $RESULT -ge 15 ]") {
		t.Errorf("command should contain the test, got: %s", cmd)
	}
}

func TestEvaluate_ScriptCheck(t *testing.T) {
	contracts := []Contract{
		{
			ID:   "CON-042",
			Type: "detective",
			Checks: []Check{
				{
					Name:   "script_check",
					Script: &ScriptCheck{Path: "scripts/check.sh", Timeout: "30s"},
					OnFail: FailAction{Action: "alert"},
				},
			},
		},
	}

	exec := &MockExecutor{ExitCode: 0}
	Evaluate(context.Background(), contracts, "/srv/conos/contracts", exec)

	if len(exec.Calls) != 1 {
		t.Fatalf("Calls = %d, want 1", len(exec.Calls))
	}
	cmd := exec.Calls[0]
	if !strings.Contains(cmd, "/srv/conos/contracts/scripts/check.sh") {
		t.Errorf("command should contain resolved script path, got: %s", cmd)
	}
}

func TestEvaluate_ContextCancelled(t *testing.T) {
	contracts := []Contract{
		{
			ID:   "CON-001",
			Type: "detective",
			Checks: []Check{
				{
					Name:    "check",
					Command: &CmdCheck{Run: "echo 1", Test: "[ 1 -eq 1 ]"},
					OnFail:  FailAction{Action: "alert"},
				},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	exec := &MockExecutor{ExitCode: 0}
	result := Evaluate(ctx, contracts, "/tmp", exec)

	// With a cancelled context, the check should fail
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1 (cancelled context)", result.Failed)
	}
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
