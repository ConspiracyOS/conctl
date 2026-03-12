package bootstrap

import (
	"strings"
	"testing"

	"github.com/ConspiracyOS/conctl/internal/config"
)

func TestGenerateOnDemandUnits(t *testing.T) {
	agent := config.AgentConfig{
		Name: "concierge",
		Tier: "operator",
		Mode: "on-demand",
	}

	units := GenerateUnits(agent)

	// Should produce a .path unit
	pathUnit, ok := units["conos-concierge.path"]
	if !ok {
		t.Fatal("expected con-concierge.path unit")
	}
	if !strings.Contains(pathUnit, "PathChanged=/srv/conos/agents/concierge/inbox") {
		t.Error("path unit should watch agent inbox")
	}

	// Should produce a .service unit
	svcUnit, ok := units["conos-concierge.service"]
	if !ok {
		t.Fatal("expected con-concierge.service unit")
	}
	if !strings.Contains(svcUnit, "User=a-concierge") {
		t.Error("service should run as a-concierge")
	}
	if !strings.Contains(svcUnit, "ExecStart=/usr/local/bin/conctl run concierge") {
		t.Error("service should exec con run")
	}
}

func TestServiceHardeningWorker(t *testing.T) {
	agent := config.AgentConfig{
		Name:  "researcher",
		Tier:  "worker",
		Mode:  "on-demand",
		Roles: []string{"researcher"},
	}

	units := GenerateUnits(agent)
	svc := units["conos-researcher.service"]

	// Workers get full hardening including syscall filtering
	for _, directive := range []string{
		"NoNewPrivileges=yes",
		"ProtectSystem=strict",
		"PrivateTmp=yes",
		"PrivateDevices=yes",
		"ProtectHome=tmpfs",
		"UMask=0077",
		"SystemCallFilter=@system-service",
		"SystemCallFilter=~@mount",
		"SystemCallErrorNumber=EPERM",
	} {
		if !strings.Contains(svc, directive) {
			t.Errorf("worker service should contain %s", directive)
		}
	}
}

func TestServiceHardeningSysadmin(t *testing.T) {
	agent := config.AgentConfig{
		Name:  "sysadmin",
		Tier:  "operator",
		Mode:  "on-demand",
		Roles: []string{"sysadmin"},
	}

	units := GenerateUnits(agent)
	svc := units["conos-sysadmin.service"]

	// Sysadmin needs sudo — no NoNewPrivileges, ProtectSystem, or syscall filter
	if strings.Contains(svc, "NoNewPrivileges=yes") {
		t.Error("sysadmin service must NOT have NoNewPrivileges (breaks sudo)")
	}
	if strings.Contains(svc, "ProtectSystem=strict") {
		t.Error("sysadmin service must NOT have ProtectSystem=strict (breaks sudo writes to /etc)")
	}
	if strings.Contains(svc, "SystemCallFilter=") {
		t.Error("sysadmin service must NOT have SystemCallFilter (needs broad syscall access for sudo)")
	}

	// But should still have other hardening
	for _, directive := range []string{
		"PrivateTmp=yes",
		"PrivateDevices=yes",
		"UMask=0077",
	} {
		if !strings.Contains(svc, directive) {
			t.Errorf("sysadmin service should still contain %s", directive)
		}
	}
}

func TestServiceHardeningOperatorSyscalls(t *testing.T) {
	agent := config.AgentConfig{
		Name: "concierge",
		Tier: "operator",
		Mode: "on-demand",
	}

	units := GenerateUnits(agent)
	svc := units["conos-concierge.service"]

	// Operators get syscall filtering (same as workers)
	for _, directive := range []string{
		"SystemCallFilter=@system-service",
		"SystemCallFilter=~@mount",
		"SystemCallErrorNumber=EPERM",
	} {
		if !strings.Contains(svc, directive) {
			t.Errorf("operator service should contain %s", directive)
		}
	}
}

func TestAgentEnvLines_Empty(t *testing.T) {
	agent := config.AgentConfig{Name: "concierge"}
	if result := agentEnvLines(agent); result != "" {
		t.Errorf("expected empty string for agent with no env vars, got %q", result)
	}
}

func TestAgentEnvLines_WithVars(t *testing.T) {
	agent := config.AgentConfig{
		Name:        "researcher",
		Environment: []string{"API_KEY=secret123", "ENABLE_TOOLS=true"},
	}
	result := agentEnvLines(agent)
	if !strings.Contains(result, "Environment=") {
		t.Error("expected Environment= directives in output")
	}
	if !strings.Contains(result, "API_KEY=secret123") {
		t.Error("expected API_KEY in env lines")
	}
	if !strings.Contains(result, "ENABLE_TOOLS=true") {
		t.Error("expected ENABLE_TOOLS in env lines")
	}
}

func TestGenerateContinuousUnits(t *testing.T) {
	agent := config.AgentConfig{
		Name: "heartbeat",
		Tier: "worker",
		Mode: "continuous",
	}
	units := GenerateUnits(agent)

	svc, ok := units["conos-heartbeat.service"]
	if !ok {
		t.Fatal("expected conos-heartbeat.service unit")
	}
	if !strings.Contains(svc, "Type=simple") {
		t.Error("continuous service should use Type=simple")
	}
	if !strings.Contains(svc, "Restart=on-failure") {
		t.Error("continuous service should have Restart=on-failure")
	}
	if !strings.Contains(svc, "--continuous") {
		t.Error("continuous service should pass --continuous flag to conctl")
	}
	if _, ok := units["conos-heartbeat.path"]; ok {
		t.Error("continuous agent should not have a .path unit")
	}
}

func TestGenerateCronUnits(t *testing.T) {
	agent := config.AgentConfig{
		Name: "reporter",
		Tier: "worker",
		Mode: "cron",
		Cron: "*-*-* 09:00:00",
	}

	units := GenerateUnits(agent)

	// Should produce a .timer unit
	timerUnit, ok := units["conos-reporter.timer"]
	if !ok {
		t.Fatal("expected con-reporter.timer unit")
	}
	if !strings.Contains(timerUnit, "OnCalendar=*-*-* 09:00:00") {
		t.Error("timer should use cron expression")
	}

	// Should also produce a .path unit for on-demand tasks between scheduled runs
	pathUnit, ok := units["conos-reporter.path"]
	if !ok {
		t.Fatal("expected con-reporter.path unit for inbox watching")
	}
	if !strings.Contains(pathUnit, "PathChanged=/srv/conos/agents/reporter/inbox") {
		t.Error("path unit should watch agent inbox")
	}

	// Should produce a .service unit
	if _, ok := units["conos-reporter.service"]; !ok {
		t.Fatal("expected con-reporter.service unit")
	}
}
