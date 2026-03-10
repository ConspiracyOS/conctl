package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
