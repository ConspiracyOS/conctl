package contracts

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRecordSpendAndCheckBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spend.jsonl")
	day := time.Date(2026, 3, 6, 10, 0, 0, 0, time.UTC)

	exceeded, total, err := RecordSpendAndCheckBudget(path, SpendEvent{
		Timestamp: day,
		Agent:     "analyst",
		CostUSD:   1.25,
	}, 2.0)
	if err != nil {
		t.Fatalf("record spend #1: %v", err)
	}
	if exceeded {
		t.Fatalf("unexpected exceed after first event")
	}
	if total != 1.25 {
		t.Fatalf("total = %f", total)
	}

	exceeded, total, err = RecordSpendAndCheckBudget(path, SpendEvent{
		Timestamp: day.Add(1 * time.Hour),
		Agent:     "analyst",
		CostUSD:   1.00,
	}, 2.0)
	if err != nil {
		t.Fatalf("record spend #2: %v", err)
	}
	if !exceeded {
		t.Fatalf("expected budget exceed")
	}
	if total != 2.25 {
		t.Fatalf("total = %f", total)
	}
}
