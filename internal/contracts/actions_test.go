package contracts

import (
	"context"
	"strings"
	"testing"
)

func TestDispatchAction_HaltAgents(t *testing.T) {
	exec := &MockExecutor{ExitCode: 0}
	action := FailAction{Action: "halt_agents", Message: "test"}

	cmds, err := DispatchAction(context.Background(), action, "system", exec)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("cmds = %d, want 1", len(cmds))
	}
	if !strings.Contains(cmds[0], "systemctl stop") {
		t.Errorf("expected systemctl stop, got: %s", cmds[0])
	}
}

func TestDispatchAction_HaltWorkers(t *testing.T) {
	exec := &MockExecutor{ExitCode: 0}
	action := FailAction{Action: "halt_workers", Message: "test"}

	cmds, err := DispatchAction(context.Background(), action, "system", exec)
	if err != nil {
		t.Fatal(err)
	}
	// v1: halt_workers = halt_agents
	if len(cmds) != 1 {
		t.Fatalf("cmds = %d, want 1", len(cmds))
	}
}

func TestDispatchAction_KillSession(t *testing.T) {
	exec := &MockExecutor{ExitCode: 0}
	action := FailAction{Action: "kill_session", Message: "session too long"}

	cmds, err := DispatchAction(context.Background(), action, "agent:researcher", exec)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("cmds = %d, want 1", len(cmds))
	}
	if !strings.Contains(cmds[0], "pkill -u a-researcher picoclaw") {
		t.Errorf("expected pkill for researcher, got: %s", cmds[0])
	}
}

func TestDispatchAction_KillSession_NoAgent(t *testing.T) {
	exec := &MockExecutor{ExitCode: 0}
	action := FailAction{Action: "kill_session", Message: "test"}

	_, err := DispatchAction(context.Background(), action, "system", exec)
	if err == nil {
		t.Error("expected error for kill_session without agent scope")
	}
}

func TestDispatchAction_Quarantine(t *testing.T) {
	exec := &MockExecutor{ExitCode: 0}
	action := FailAction{Action: "quarantine", Message: "compromised"}

	cmds, err := DispatchAction(context.Background(), action, "agent:badagent", exec)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 2 {
		t.Fatalf("cmds = %d, want 2", len(cmds))
	}
	if !strings.Contains(cmds[0], "systemctl stop con-badagent") {
		t.Errorf("first cmd should stop service, got: %s", cmds[0])
	}
	if !strings.Contains(cmds[1], "setfacl -b") {
		t.Errorf("second cmd should revoke ACLs, got: %s", cmds[1])
	}
}

func TestDispatchAction_Alert(t *testing.T) {
	exec := &MockExecutor{ExitCode: 0}
	action := FailAction{Action: "alert", Message: "info only"}

	cmds, err := DispatchAction(context.Background(), action, "system", exec)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 0 {
		t.Errorf("alert should execute no commands, got %d", len(cmds))
	}
	if len(exec.Calls) != 0 {
		t.Errorf("alert should not call executor, got %d calls", len(exec.Calls))
	}
}

func TestEscalate(t *testing.T) {
	// Escalate writes to a hardcoded /srv/conos/agents/<name>/inbox/ path.
	// In test environments this path does not exist, so we verify the error path.
	err := Escalate("nonexistent-test-agent-xyz", "alert: something failed")
	if err == nil {
		t.Error("expected error when agent inbox does not exist")
	}
}

func TestDispatchAction_WithEscalation(t *testing.T) {
	exec := &MockExecutor{ExitCode: 0}
	// alert executes no OS command but does trigger escalation when Escalate is set.
	action := FailAction{Action: "alert", Escalate: "sysadmin", Message: "disk low"}

	_, err := DispatchAction(context.Background(), action, "system", exec)
	// Escalate will fail (no inbox on test host) — verify the error is surfaced.
	if err == nil {
		t.Error("expected error when escalation inbox does not exist")
	}
	if !strings.Contains(err.Error(), "escalation") {
		t.Errorf("error should mention escalation, got: %v", err)
	}
}

func TestParseAgentFromScope(t *testing.T) {
	cases := []struct {
		scope string
		want  string
	}{
		{"agent:sysadmin", "sysadmin"},
		{"agent:concierge", "concierge"},
		{"system", ""},
		{"global", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := parseAgentFromScope(tc.scope); got != tc.want {
			t.Errorf("parseAgentFromScope(%q) = %q, want %q", tc.scope, got, tc.want)
		}
	}
}

func TestDispatchAction_HaltAgents_ExecutorError(t *testing.T) {
	exec := &ErrorExecutor{}
	action := FailAction{Action: "halt_agents"}

	_, err := DispatchAction(context.Background(), action, "system", exec)
	if err == nil {
		t.Error("expected error when executor fails")
	}
	if !strings.Contains(err.Error(), "halt_agents") {
		t.Errorf("error should mention halt_agents, got: %v", err)
	}
}

func TestDispatchAction_Quarantine_ACLError(t *testing.T) {
	// First command (systemctl stop) succeeds, second (setfacl) fails.
	calls := 0
	exec := &mockOnNthCallError{failOn: 2}
	action := FailAction{Action: "quarantine"}

	_, err := DispatchAction(context.Background(), action, "agent:target", exec)
	if err == nil {
		t.Error("expected error when ACL command fails")
	}
	if !strings.Contains(err.Error(), "quarantine acl") {
		t.Errorf("error should mention quarantine acl, got: %v", err)
	}
	_ = calls
}

func TestDispatchAction_UnknownAction(t *testing.T) {
	exec := &MockExecutor{ExitCode: 0}
	action := FailAction{Action: "destroy_everything", Message: "bad"}

	_, err := DispatchAction(context.Background(), action, "system", exec)
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestDispatchAction_KillSession_ExecutorError(t *testing.T) {
	exec := &ErrorExecutor{}
	action := FailAction{Action: "kill_session"}

	_, err := DispatchAction(context.Background(), action, "agent:target", exec)
	if err == nil {
		t.Error("expected error when executor fails")
	}
	if !strings.Contains(err.Error(), "kill_session") {
		t.Errorf("error should mention kill_session, got: %v", err)
	}
}

func TestDispatchAction_Quarantine_NoAgent(t *testing.T) {
	exec := &MockExecutor{ExitCode: 0}
	action := FailAction{Action: "quarantine"}

	_, err := DispatchAction(context.Background(), action, "system", exec)
	if err == nil {
		t.Error("expected error for quarantine without agent scope")
	}
	if !strings.Contains(err.Error(), "quarantine") {
		t.Errorf("error should mention quarantine, got: %v", err)
	}
}

func TestDispatchAction_Quarantine_StopError(t *testing.T) {
	// First call (systemctl stop) fails — should surface "quarantine stop" error.
	exec := &mockOnNthCallError{failOn: 1}
	action := FailAction{Action: "quarantine"}

	_, err := DispatchAction(context.Background(), action, "agent:target", exec)
	if err == nil {
		t.Error("expected error when stop command fails")
	}
	if !strings.Contains(err.Error(), "quarantine stop") {
		t.Errorf("error should mention quarantine stop, got: %v", err)
	}
}
