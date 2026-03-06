package contracts

import (
	"fmt"
	"sort"
	"strings"
)

var doctorSeverityRank = map[string]int{
	"critical": 0,
	"high":     1,
	"medium":   2,
	"low":      3,
	"info":     4,
	"":         1,
}

// FormatDoctorReport renders a sysadmin-focused health report from contract results.
func FormatDoctorReport(result RunResult) string {
	var findings []CheckResult
	for _, cr := range result.Results {
		if cr.Status == "fail" || cr.Status == "warn" || cr.Status == "unknown" {
			findings = append(findings, cr)
		}
	}

	sort.SliceStable(findings, func(i, j int) bool {
		return doctorSeverityRank[findings[i].Severity] < doctorSeverityRank[findings[j].Severity]
	})

	var b strings.Builder
	fmt.Fprintf(&b, "# System Doctor\n\n")
	fmt.Fprintf(&b, "evaluation_id: %s\n", result.EvaluationID)
	if result.RunID != "" {
		fmt.Fprintf(&b, "run_id: %s\n", result.RunID)
	}
	if result.Actor != "" {
		fmt.Fprintf(&b, "actor: %s\n", result.Actor)
	}
	fmt.Fprintf(&b, "summary: passed=%d failed=%d warned=%d unknown=%d skipped=%d\n\n",
		result.Passed, result.Failed, result.Warned, result.Unknown, result.Skipped)

	if len(findings) == 0 {
		b.WriteString("No issues found.\n")
		return b.String()
	}

	for i, f := range findings {
		what := f.What
		if what == "" {
			what = fmt.Sprintf("%s/%s failed", f.ContractID, f.CheckName)
		}
		fmt.Fprintf(&b, "## %d. [%s] %s\n", i+1, f.Severity, what)
		fmt.Fprintf(&b, "contract: %s\n", f.ContractID)
		fmt.Fprintf(&b, "check: %s\n", f.CheckName)
		fmt.Fprintf(&b, "status: %s\n", f.Status)
		if f.Owner != "" {
			fmt.Fprintf(&b, "owner: %s\n", f.Owner)
		}
		if f.Category != "" {
			fmt.Fprintf(&b, "category: %s\n", f.Category)
		}
		if len(f.Affects) > 0 {
			fmt.Fprintf(&b, "affects: %s\n", strings.Join(f.Affects, ", "))
		}
		if f.Verify != "" {
			fmt.Fprintf(&b, "verify: %s\n", f.Verify)
		}
		if f.Evidence != "" {
			fmt.Fprintf(&b, "evidence:\n")
			for _, line := range strings.Split(strings.TrimSpace(f.Evidence), "\n") {
				fmt.Fprintf(&b, "  %s\n", line)
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}
