package contracts

import (
	"testing"

	sharedcontracts "github.com/ConspiracyOS/contracts/pkg/contracts"
)

func TestFromSharedAudit(t *testing.T) {
	r := FromSharedAudit(sharedcontracts.AuditResult{
		Passed: 1,
		Failed: 1,
		Warned: 1,
		Results: []sharedcontracts.CheckResult{
			{ContractID: "C1", CheckName: "ok", Status: "pass"},
			{ContractID: "C1", CheckName: "warn", Status: "warn", OnFail: sharedcontracts.OnFailWarn},
			{ContractID: "C2", CheckName: "fail", Status: "fail", OnFail: sharedcontracts.OnFailFail},
		},
	}, EvalOptions{RunID: "run-1", Actor: "sysadmin"})

	if r.RunID != "run-1" {
		t.Fatalf("run_id = %q", r.RunID)
	}
	if r.Failed != 2 {
		t.Fatalf("failed = %d, want 2 (fail + warn compatibility)", r.Failed)
	}
	if len(r.Results) != 3 {
		t.Fatalf("results = %d", len(r.Results))
	}
	if r.Results[1].Status != "warn" {
		t.Fatalf("status mismatch: %+v", r.Results[1])
	}
}
