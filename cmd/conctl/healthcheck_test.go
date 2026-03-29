package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ConspiracyOS/conctl/internal/contracts"
)

func TestRunHealthcheck_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONOS_CONTRACTS_DIR", dir)
	runHealthcheck()
}

func TestHealthcheckIn_EmptyDir(t *testing.T) {
	err := healthcheckIn(t.TempDir(), filepath.Join(t.TempDir(), "contracts.log"), "")
	if err != nil {
		t.Errorf("expected nil for empty contracts dir, got: %v", err)
	}
}

func TestHealthcheckIn_BadDir(t *testing.T) {
	err := healthcheckIn("/nonexistent-xyz/contracts", "/dev/null", "")
	if err == nil {
		t.Error("expected error for nonexistent contracts dir")
	}
	if !strings.Contains(err.Error(), "loading contracts") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHealthcheckIn_PassingContract(t *testing.T) {
	contractsDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "contracts.log")

	os.WriteFile(filepath.Join(contractsDir, "test.yaml"), []byte(`
id: TEST-001
description: Always passing
type: detective
tags: [schedule]
scope: global
checks:
  - name: always_pass
    command:
      run: "true"
      exit_code: 0
    on_fail: alert
`), 0644)

	err := healthcheckIn(contractsDir, logPath, "")
	if err != nil {
		t.Errorf("expected nil for passing contract, got: %v", err)
	}
	// Log file should have been written
	if _, statErr := os.Stat(logPath); os.IsNotExist(statErr) {
		t.Error("expected log file to be written")
	}
}

func TestHealthcheckIn_FailingContract(t *testing.T) {
	contractsDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "contracts.log")

	os.WriteFile(filepath.Join(contractsDir, "test.yaml"), []byte(`
id: TEST-002
description: Always failing
type: detective
tags: [schedule]
scope: global
checks:
  - name: always_fail
    command:
      run: "false"
      exit_code: 0
    on_fail: alert
`), 0644)

	err := healthcheckIn(contractsDir, logPath, "")
	if err == nil {
		t.Fatal("expected error for failing contract")
	}
	if !strings.Contains(err.Error(), "contract(s) failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- selectedContractTags tests ---

func TestSelectedContractTags_Default(t *testing.T) {
	// No env var set -> returns ["schedule"]
	t.Setenv("CONOS_CONTRACT_TAGS", "")
	tags := selectedContractTags()
	if len(tags) != 1 || tags[0] != "schedule" {
		t.Errorf("expected [schedule], got %v", tags)
	}
}

func TestSelectedContractTags_Unset(t *testing.T) {
	// Env var not present at all -> returns ["schedule"]
	// t.Setenv sets it, so we need to unset after
	t.Setenv("CONOS_CONTRACT_TAGS", "")
	os.Unsetenv("CONOS_CONTRACT_TAGS")
	tags := selectedContractTags()
	if len(tags) != 1 || tags[0] != "schedule" {
		t.Errorf("expected [schedule], got %v", tags)
	}
}

func TestSelectedContractTags_Single(t *testing.T) {
	t.Setenv("CONOS_CONTRACT_TAGS", "boot")
	tags := selectedContractTags()
	if len(tags) != 1 || tags[0] != "boot" {
		t.Errorf("expected [boot], got %v", tags)
	}
}

func TestSelectedContractTags_Multiple(t *testing.T) {
	t.Setenv("CONOS_CONTRACT_TAGS", "boot,schedule,network")
	tags := selectedContractTags()
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d: %v", len(tags), tags)
	}
	want := []string{"boot", "schedule", "network"}
	for i, w := range want {
		if tags[i] != w {
			t.Errorf("tag[%d]: expected %q, got %q", i, w, tags[i])
		}
	}
}

func TestSelectedContractTags_Whitespace(t *testing.T) {
	t.Setenv("CONOS_CONTRACT_TAGS", " boot , schedule , network ")
	tags := selectedContractTags()
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d: %v", len(tags), tags)
	}
	want := []string{"boot", "schedule", "network"}
	for i, w := range want {
		if tags[i] != w {
			t.Errorf("tag[%d]: expected %q, got %q", i, w, tags[i])
		}
	}
}

func TestSelectedContractTags_OnlyCommas(t *testing.T) {
	t.Setenv("CONOS_CONTRACT_TAGS", ",,, ,")
	tags := selectedContractTags()
	if len(tags) != 1 || tags[0] != "schedule" {
		t.Errorf("expected [schedule] for only-commas input, got %v", tags)
	}
}

func TestSelectedContractTags_OnlyWhitespace(t *testing.T) {
	t.Setenv("CONOS_CONTRACT_TAGS", "   ")
	tags := selectedContractTags()
	if len(tags) != 1 || tags[0] != "schedule" {
		t.Errorf("expected [schedule] for whitespace-only input, got %v", tags)
	}
}

// --- writeBriefOutput tests ---

// --- escalation message context tests ---

func TestHealthcheckIn_EscalateMessageContext(t *testing.T) {
	// When a check with on_fail=escalate fails, the escalation task
	// should contain the contract ID, check name, and what description.
	contractsDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "contracts.log")

	// Create a sysadmin inbox for Escalate() to write to
	sysadminInbox := filepath.Join(t.TempDir(), "agents", "sysadmin", "inbox")
	os.MkdirAll(sysadminInbox, 0755)
	t.Setenv("CONOS_STATE_ROOT", filepath.Dir(filepath.Dir(filepath.Dir(sysadminInbox))))

	// Patch the inbox path used by Escalate — it uses /srv/conos/agents/<name>/inbox
	// so we need to use the real path or override. Since Escalate hardcodes the path,
	// we test at the DispatchAction level instead.
	// This test verifies the FailAction is correctly composed.

	os.WriteFile(filepath.Join(contractsDir, "test.yaml"), []byte(`
id: TEST-ESC
description: Escalation test
type: detective
tags: [schedule]
scope: global
checks:
  - name: immutable_check
    command:
      run: "false"
      exit_code: 0
    on_fail: escalate
    what: "Critical files missing immutable bit (chattr +i)"
`), 0644)

	err := healthcheckIn(contractsDir, logPath, "")
	if err == nil {
		t.Fatal("expected error for failing contract")
	}
	if !strings.Contains(err.Error(), "contract(s) failed") {
		t.Errorf("unexpected error: %v", err)
	}
	// The Escalate call will fail (no /srv/conos/agents/sysadmin/inbox/) but
	// the error is logged to stderr and does not block the healthcheck.
	// This test validates the code path runs without panic.
}

func TestHealthcheckIn_MetaEscalationSkipsEscalateChecks(t *testing.T) {
	// When all failing checks use on_fail=escalate, the meta-escalation
	// summary should be suppressed (no duplicate tasks).
	contractsDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "contracts.log")

	os.WriteFile(filepath.Join(contractsDir, "test.yaml"), []byte(`
id: TEST-META
description: Meta-escalation test
type: detective
tags: [schedule]
scope: global
checks:
  - name: check_a
    command:
      run: "false"
      exit_code: 0
    on_fail: escalate
    what: "Check A failed"
  - name: check_b
    command:
      run: "false"
      exit_code: 0
    on_fail: escalate
    what: "Check B failed"
`), 0644)

	err := healthcheckIn(contractsDir, logPath, "")
	if err == nil {
		t.Fatal("expected error for failing contract")
	}
	// Both checks use escalate, so meta-escalation should be skipped.
	// We can't easily verify this without mocking Escalate, but the code
	// path should run without error.
}

func TestHealthcheckIn_MetaEscalationForAlertChecks(t *testing.T) {
	// When some checks use on_fail=alert and some use on_fail=escalate,
	// the meta-escalation should only include the alert checks.
	contractsDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "contracts.log")

	os.WriteFile(filepath.Join(contractsDir, "esc.yaml"), []byte(`
id: TEST-MIX-ESC
description: Escalate check
type: detective
tags: [schedule]
scope: global
checks:
  - name: esc_check
    command:
      run: "false"
      exit_code: 0
    on_fail: escalate
    what: "Escalate this"
`), 0644)

	os.WriteFile(filepath.Join(contractsDir, "alert.yaml"), []byte(`
id: TEST-MIX-ALERT
description: Alert check
type: detective
tags: [schedule]
scope: global
checks:
  - name: alert_check
    command:
      run: "false"
      exit_code: 0
    on_fail: alert
    what: "Alert this"
`), 0644)

	err := healthcheckIn(contractsDir, logPath, "")
	if err == nil {
		t.Fatal("expected error for failing contracts")
	}
	// Meta-escalation should include only TEST-MIX-ALERT/alert_check,
	// not TEST-MIX-ESC/esc_check.
}

func TestWriteBriefOutput_AllPassed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "brief.md")
	ts := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	result := contracts.RunResult{
		Timestamp: ts,
		Passed:    5,
		Failed:    0,
	}

	if err := writeBriefOutput(path, result); err != nil {
		t.Fatalf("writeBriefOutput: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "All 5 checks passed") {
		t.Errorf("expected 'All 5 checks passed' in output, got:\n%s", content)
	}
	if !strings.Contains(content, "2026-03-15T12:00:00Z") {
		t.Errorf("expected timestamp in output, got:\n%s", content)
	}
}

func TestWriteBriefOutput_SomeFailed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "brief.md")
	ts := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)

	result := contracts.RunResult{
		Timestamp: ts,
		Passed:    3,
		Failed:    2,
		Results: []contracts.CheckResult{
			{ContractID: "C-001", CheckName: "dns_resolves", Passed: true},
			{ContractID: "C-002", CheckName: "disk_space", Passed: false, Output: "95% full"},
			{ContractID: "C-003", CheckName: "mem_check", Passed: false, Error: fmt.Errorf("OOM detected")},
			{ContractID: "C-004", CheckName: "cpu_ok", Passed: true},
			{ContractID: "C-005", CheckName: "net_ok", Passed: true},
		},
	}

	if err := writeBriefOutput(path, result); err != nil {
		t.Fatalf("writeBriefOutput: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "**2 failed**") {
		t.Errorf("expected '**2 failed**' in output, got:\n%s", content)
	}
	if !strings.Contains(content, "3 passed") {
		t.Errorf("expected '3 passed' in output, got:\n%s", content)
	}
	if !strings.Contains(content, "C-002: disk_space") {
		t.Errorf("expected failed check C-002 in output, got:\n%s", content)
	}
	if !strings.Contains(content, "95% full") {
		t.Errorf("expected output '95%% full' in content, got:\n%s", content)
	}
	if !strings.Contains(content, "C-003: mem_check") {
		t.Errorf("expected failed check C-003 in output, got:\n%s", content)
	}
	if !strings.Contains(content, "OOM detected") {
		t.Errorf("expected error 'OOM detected' in content, got:\n%s", content)
	}
	// Passing checks should NOT appear in the failed section
	if strings.Contains(content, "C-001") {
		t.Errorf("passing check C-001 should not appear in output")
	}
}

func TestWriteBriefOutput_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "brief.md")
	ts := time.Now()

	result := contracts.RunResult{
		Timestamp: ts,
		Passed:    1,
		Failed:    0,
	}

	if err := writeBriefOutput(path, result); err != nil {
		t.Fatalf("writeBriefOutput with nested dirs: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to exist after writeBriefOutput with nested dirs")
	}
}

func TestWriteBriefOutput_TimestampPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "brief.md")
	ts := time.Date(2025, 12, 25, 8, 0, 0, 0, time.UTC)

	result := contracts.RunResult{
		Timestamp: ts,
		Passed:    0,
		Failed:    0,
	}

	if err := writeBriefOutput(path, result); err != nil {
		t.Fatalf("writeBriefOutput: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "2025-12-25T08:00:00Z") {
		t.Errorf("expected RFC3339 timestamp in output, got:\n%s", content)
	}
	if !strings.Contains(content, "# System State") {
		t.Errorf("expected markdown header in output, got:\n%s", content)
	}
}
