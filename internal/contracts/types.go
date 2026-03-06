package contracts

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Contract represents a single YAML contract file.
type Contract struct {
	ID           string   `yaml:"id"`
	Description  string   `yaml:"description"`
	Type         string   `yaml:"type"`                // "detective" | "preventive" | "task" | "meta"
	Class        string   `yaml:"class,omitempty"`     // generic class: invariant | task | meta
	Trigger      string   `yaml:"trigger,omitempty"`   // new: "schedule" | "event"
	Frequency    string   `yaml:"frequency,omitempty"` // old: e.g. "60s" — kept for backward compat
	Scope        string   `yaml:"scope"`               // "global" | "system" (legacy) | "agent:<name>"
	Subject      string   `yaml:"subject,omitempty"`   // resource this contract protects/tracks
	Owner        string   `yaml:"owner,omitempty"`     // owner for task contracts / ops tracking
	DependsOn    []string `yaml:"depends_on,omitempty"`
	SnapshotKeys []string `yaml:"snapshot_keys,omitempty"` // optional fields to include in state snapshots
	Checks       []Check  `yaml:"checks"`

	// Preventive-only fields (for registry/auditability)
	Mechanism   string `yaml:"mechanism,omitempty"`
	Agent       string `yaml:"agent,omitempty"`
	Enforcement string `yaml:"enforcement,omitempty"`
}

// Check is a single check within a detective contract.
type Check struct {
	Name     string       `yaml:"name"`
	Command  *CmdCheck    `yaml:"command,omitempty"`
	Script   *ScriptCheck `yaml:"script,omitempty"`
	OnFail   FailAction   `yaml:"on_fail"`
	Severity string       `yaml:"severity,omitempty"`
	Category string       `yaml:"category,omitempty"`
	What     string       `yaml:"what,omitempty"`
	Verify   string       `yaml:"verify,omitempty"`
	Affects  []string     `yaml:"affects,omitempty"`
}

// CmdCheck: inline shell command. In the new unified schema the run command
// is self-contained (exit code signals pass/fail). The optional Test field
// is kept for backward compatibility with the old split run+test format.
type CmdCheck struct {
	Run      string `yaml:"run"`                 // shell command; exit code signals pass/fail
	Test     string `yaml:"test,omitempty"`      // old: test expression using $RESULT
	ExitCode int    `yaml:"exit_code,omitempty"` // expected exit code (default 0)
}

// ScriptCheck: external script.
type ScriptCheck struct {
	Path    string `yaml:"path"`
	Timeout string `yaml:"timeout"`
}

// FailAction defines what happens when a check fails.
// In the new schema it is a plain string; in the old schema it was an object.
// Both forms are supported via a custom YAML unmarshaler.
type FailAction struct {
	Action   string // halt_agents | halt_workers | kill_session | quarantine | alert
	Escalate string // agent name to receive escalation task (old object form only)
	Message  string // human-readable message (old object form only)
}

// UnmarshalYAML accepts both:
//
//	on_fail: halt_agents          (new: plain string)
//	on_fail:                      (old: object)
//	  action: halt_agents
//	  escalate: sysadmin
//	  message: "..."
func (f *FailAction) UnmarshalYAML(value *yaml.Node) error {
	// Plain scalar — new format
	if value.Kind == yaml.ScalarNode {
		f.Action = value.Value
		return nil
	}

	// Mapping — old format
	if value.Kind == yaml.MappingNode {
		type failActionAlias struct {
			Action   string `yaml:"action"`
			Escalate string `yaml:"escalate"`
			Message  string `yaml:"message"`
		}
		var alias failActionAlias
		if err := value.Decode(&alias); err != nil {
			return fmt.Errorf("on_fail: %w", err)
		}
		f.Action = alias.Action
		f.Escalate = alias.Escalate
		f.Message = alias.Message
		return nil
	}

	return fmt.Errorf("on_fail: unexpected YAML node kind %v", value.Kind)
}

// CheckResult captures the outcome of one check execution.
type CheckResult struct {
	ContractID   string
	CheckName    string
	Passed       bool
	Status       string // pass | fail | warn | unknown | exempt
	Output       string
	Evidence     string
	Error        error
	Duration     time.Duration
	OnFail       string
	Severity     string
	Category     string
	What         string
	Verify       string
	Affects      []string
	RunID        string
	Actor        string
	EvaluationID string
	Owner        string
	Class        string
}

// RunResult captures the outcome of a full healthcheck run.
type RunResult struct {
	Timestamp    time.Time
	Results      []CheckResult
	Passed       int
	Failed       int
	Warned       int
	Unknown      int
	Skipped      int // preventive contracts
	RunID        string
	Actor        string
	EvaluationID string
}

// Valid failure actions.
var validActions = map[string]bool{
	"halt_agents":       true,
	"halt_workers":      true,
	"kill_session":      true,
	"quarantine":        true,
	"alert":             true,
	"escalate":          true, // escalate to sysadmin inbox
	"fail":              true,
	"warn":              true,
	"require_exemption": true,
}
