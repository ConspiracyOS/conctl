package contracts

import (
	"time"

	sharedcontracts "github.com/ConspiracyOS/contracts/pkg/contracts"
)

// FromSharedAudit converts shared contracts-engine results into conctl runtime result format.
func FromSharedAudit(a sharedcontracts.AuditResult, opts EvalOptions) RunResult {
	if opts.EvaluationID == "" {
		opts.EvaluationID = newEvaluationID()
	}
	out := RunResult{
		Timestamp:    time.Now(),
		Passed:       a.Passed,
		Failed:       a.Failed + a.Warned, // conctl compatibility: alert/warn findings fail healthcheck run
		Warned:       a.Warned,
		Skipped:      a.Skipped,
		RunID:        opts.RunID,
		Actor:        opts.Actor,
		EvaluationID: opts.EvaluationID,
	}
	for _, r := range a.Results {
		cr := CheckResult{
			ContractID:   r.ContractID,
			CheckName:    r.CheckName,
			Status:       string(r.Status),
			Passed:       r.Status == "pass",
			Output:       r.Message,
			Evidence:     r.Evidence,
			OnFail:       string(r.OnFail),
			Severity:     r.Severity,
			Category:     r.Category,
			What:         r.What,
			Verify:       r.Verify,
			Affects:      r.Affects,
			RunID:        opts.RunID,
			Actor:        opts.Actor,
			EvaluationID: opts.EvaluationID,
		}
		if r.Status == "warn" {
			cr.Passed = false
		}
		out.Results = append(out.Results, cr)
	}
	return out
}
