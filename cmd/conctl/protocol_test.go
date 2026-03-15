package main

import (
	"testing"

	sharedcontracts "github.com/ConspiracyOS/contracts/pkg/contracts"
)

func TestRecount(t *testing.T) {
	tests := []struct {
		name    string
		input   sharedcontracts.AuditResult
		wantP   int // Passed
		wantF   int // Failed
		wantW   int // Warned
		wantE   int // Exempt
		wantS   int // Skipped
		wantH   int // Halted
	}{
		{
			name: "mixed statuses",
			input: sharedcontracts.AuditResult{
				Results: []sharedcontracts.CheckResult{
					{Status: "pass", CheckName: "check-1"},
					{Status: "fail", CheckName: "check-2"},
					{Status: "warn", CheckName: "check-3"},
					{Status: "exempt", CheckName: "check-4"},
					{Status: "skip", CheckName: "check-5"},
					{Status: "halt", CheckName: "check-6"},
				},
			},
			wantP: 1, wantF: 1, wantW: 1, wantE: 1, wantS: 1, wantH: 1,
		},
		{
			name: "all pass",
			input: sharedcontracts.AuditResult{
				Results: []sharedcontracts.CheckResult{
					{Status: "pass", CheckName: "a"},
					{Status: "pass", CheckName: "b"},
					{Status: "pass", CheckName: "c"},
				},
			},
			wantP: 3, wantF: 0, wantW: 0, wantE: 0, wantS: 0, wantH: 0,
		},
		{
			name: "all fail",
			input: sharedcontracts.AuditResult{
				Results: []sharedcontracts.CheckResult{
					{Status: "fail", CheckName: "a"},
					{Status: "fail", CheckName: "b"},
				},
			},
			wantP: 0, wantF: 2, wantW: 0, wantE: 0, wantS: 0, wantH: 0,
		},
		{
			name: "all warn",
			input: sharedcontracts.AuditResult{
				Results: []sharedcontracts.CheckResult{
					{Status: "warn", CheckName: "a"},
					{Status: "warn", CheckName: "b"},
				},
			},
			wantP: 0, wantF: 0, wantW: 2, wantE: 0, wantS: 0, wantH: 0,
		},
		{
			name: "all exempt",
			input: sharedcontracts.AuditResult{
				Results: []sharedcontracts.CheckResult{
					{Status: "exempt", CheckName: "a"},
					{Status: "exempt", CheckName: "b"},
				},
			},
			wantP: 0, wantF: 0, wantW: 0, wantE: 2, wantS: 0, wantH: 0,
		},
		{
			name: "all skip",
			input: sharedcontracts.AuditResult{
				Results: []sharedcontracts.CheckResult{
					{Status: "skip", CheckName: "a"},
					{Status: "skip", CheckName: "b"},
				},
			},
			wantP: 0, wantF: 0, wantW: 0, wantE: 0, wantS: 2, wantH: 0,
		},
		{
			name: "all halt",
			input: sharedcontracts.AuditResult{
				Results: []sharedcontracts.CheckResult{
					{Status: "halt", CheckName: "a"},
					{Status: "halt", CheckName: "b"},
					{Status: "halt", CheckName: "c"},
				},
			},
			wantP: 0, wantF: 0, wantW: 0, wantE: 0, wantS: 0, wantH: 3,
		},
		{
			name:  "empty results",
			input: sharedcontracts.AuditResult{Results: nil},
			wantP: 0, wantF: 0, wantW: 0, wantE: 0, wantS: 0, wantH: 0,
		},
		{
			name: "single result pass",
			input: sharedcontracts.AuditResult{
				Results: []sharedcontracts.CheckResult{
					{Status: "pass", CheckName: "only-check"},
				},
			},
			wantP: 1, wantF: 0, wantW: 0, wantE: 0, wantS: 0, wantH: 0,
		},
		{
			name: "single result halt",
			input: sharedcontracts.AuditResult{
				Results: []sharedcontracts.CheckResult{
					{Status: "halt", CheckName: "only-check"},
				},
			},
			wantP: 0, wantF: 0, wantW: 0, wantE: 0, wantS: 0, wantH: 1,
		},
		{
			name: "stale counts are replaced",
			input: sharedcontracts.AuditResult{
				Results: []sharedcontracts.CheckResult{
					{Status: "pass", CheckName: "a"},
					{Status: "exempt", CheckName: "b"},
				},
				// Pre-populate with wrong counts to verify they get replaced.
				Passed: 99, Failed: 99, Warned: 99, Exempt: 99, Skipped: 99, Halted: 99,
			},
			wantP: 1, wantF: 0, wantW: 0, wantE: 1, wantS: 0, wantH: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := recount(tt.input)

			if got.Passed != tt.wantP {
				t.Errorf("Passed = %d, want %d", got.Passed, tt.wantP)
			}
			if got.Failed != tt.wantF {
				t.Errorf("Failed = %d, want %d", got.Failed, tt.wantF)
			}
			if got.Warned != tt.wantW {
				t.Errorf("Warned = %d, want %d", got.Warned, tt.wantW)
			}
			if got.Exempt != tt.wantE {
				t.Errorf("Exempt = %d, want %d", got.Exempt, tt.wantE)
			}
			if got.Skipped != tt.wantS {
				t.Errorf("Skipped = %d, want %d", got.Skipped, tt.wantS)
			}
			if got.Halted != tt.wantH {
				t.Errorf("Halted = %d, want %d", got.Halted, tt.wantH)
			}

			// Verify Results slice is preserved unchanged.
			if len(got.Results) != len(tt.input.Results) {
				t.Errorf("Results length = %d, want %d", len(got.Results), len(tt.input.Results))
			}
		})
	}
}
