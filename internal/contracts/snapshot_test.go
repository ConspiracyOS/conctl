package contracts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPersistSnapshotAndDiff_AppendsLedger(t *testing.T) {
	dir := t.TempDir()
	r1 := RunResult{
		Timestamp:    time.Now(),
		EvaluationID: "eval-1",
		RunID:        "run-1",
		Actor:        "conctl:test",
		Results: []CheckResult{
			{ContractID: "C-1", CheckName: "a", Status: "fail"},
		},
	}
	if err := PersistSnapshotAndDiff(dir, r1); err != nil {
		t.Fatalf("persist #1: %v", err)
	}

	r2 := RunResult{
		Timestamp:    time.Now(),
		EvaluationID: "eval-2",
		RunID:        "run-2",
		Actor:        "conctl:test",
		Results: []CheckResult{
			{ContractID: "C-2", CheckName: "b", Status: "fail"},
		},
	}
	if err := PersistSnapshotAndDiff(dir, r2); err != nil {
		t.Fatalf("persist #2: %v", err)
	}

	latest, err := os.ReadFile(filepath.Join(dir, "contracts-state.latest.json"))
	if err != nil {
		t.Fatalf("latest read: %v", err)
	}
	if !strings.Contains(string(latest), "C-2/b") {
		t.Fatalf("latest snapshot missing expected failing check: %s", string(latest))
	}

	ledger, err := os.ReadFile(filepath.Join(dir, "contracts-state.diff.jsonl"))
	if err != nil {
		t.Fatalf("ledger read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(ledger)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 ledger events, got %d", len(lines))
	}
}
