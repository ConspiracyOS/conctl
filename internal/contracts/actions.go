package contracts

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// DispatchAction executes the failure action for a failed check.
// Returns the commands that were executed.
func DispatchAction(ctx context.Context, action FailAction, scope string, executor CommandExecutor) ([]string, error) {
	var cmds []string

	switch action.Action {
	case "halt_agents", "halt_workers":
		// v1: halt_workers = halt_agents (tier differentiation deferred)
		cmd := "systemctl stop 'conos-*.service'"
		cmds = append(cmds, cmd)
		if _, _, err := executor.Execute(ctx, cmd); err != nil {
			return cmds, fmt.Errorf("halt_agents: %w", err)
		}

	case "kill_session":
		agent := parseAgentFromScope(scope)
		if agent == "" {
			return nil, fmt.Errorf("kill_session: cannot determine agent from scope %q", scope)
		}
		cmd := fmt.Sprintf("pkill -u a-%s picoclaw", agent)
		cmds = append(cmds, cmd)
		if _, _, err := executor.Execute(ctx, cmd); err != nil {
			return cmds, fmt.Errorf("kill_session: %w", err)
		}

	case "quarantine":
		agent := parseAgentFromScope(scope)
		if agent == "" {
			return nil, fmt.Errorf("quarantine: cannot determine agent from scope %q", scope)
		}
		stopCmd := fmt.Sprintf("systemctl stop conos-%s.service", agent)
		aclCmd := fmt.Sprintf("setfacl -b /srv/conos/agents/%s/inbox/", agent)
		cmds = append(cmds, stopCmd, aclCmd)
		if _, _, err := executor.Execute(ctx, stopCmd); err != nil {
			return cmds, fmt.Errorf("quarantine stop: %w", err)
		}
		if _, _, err := executor.Execute(ctx, aclCmd); err != nil {
			return cmds, fmt.Errorf("quarantine acl: %w", err)
		}

	case "alert":
		// No OS action — log only

	case "escalate":
		if err := Escalate("sysadmin", action.Message); err != nil {
			return cmds, fmt.Errorf("escalate: %w", err)
		}

	default:
		return nil, fmt.Errorf("unknown action: %q", action.Action)
	}

	// Escalate if target specified
	if action.Escalate != "" {
		if err := Escalate(action.Escalate, action.Message); err != nil {
			return cmds, fmt.Errorf("escalation to %s: %w", action.Escalate, err)
		}
	}

	return cmds, nil
}

// Escalate writes a .task file to the target agent's inbox.
func Escalate(agentName string, message string) error {
	ts := time.Now().Format("20060102-150405")
	taskPath := fmt.Sprintf("/srv/conos/agents/%s/inbox/%s-healthcheck.task", agentName, ts)
	return os.WriteFile(taskPath, []byte(message), 0644)
}

// parseAgentFromScope extracts agent name from "agent:<name>" scope.
func parseAgentFromScope(scope string) string {
	if strings.HasPrefix(scope, "agent:") {
		return strings.TrimPrefix(scope, "agent:")
	}
	return ""
}
