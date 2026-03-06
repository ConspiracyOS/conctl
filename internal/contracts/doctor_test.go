package contracts

import (
	"strings"
	"testing"
	"time"
)

func TestFormatDoctorReport_IncludesFindings(t *testing.T) {
	report := FormatDoctorReport(RunResult{
		Timestamp:    time.Unix(0, 0),
		EvaluationID: "eval-1",
		RunID:        "run-1",
		Actor:        "conctl:doctor",
		Passed:       1,
		Failed:       1,
		Results: []CheckResult{
			{ContractID: "C-1", CheckName: "ok", Status: "pass"},
			{ContractID: "C-2", CheckName: "disk", Status: "fail", Severity: "high", What: "Disk too full", Verify: "df -h /", Evidence: "95%"},
		},
	})

	if !strings.Contains(report, "System Doctor") {
		t.Fatalf("missing title: %s", report)
	}
	if !strings.Contains(report, "Disk too full") {
		t.Fatalf("missing finding: %s", report)
	}
	if !strings.Contains(report, "verify: df -h /") {
		t.Fatalf("missing verify command: %s", report)
	}
}
