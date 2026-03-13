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

func TestGenerateHealthcheckUnits(t *testing.T) {
	units := GenerateHealthcheckUnits("300s")

	svc, ok := units["conos-healthcheck.service"]
	if !ok {
		t.Fatal("expected conos-healthcheck.service")
	}
	if !strings.Contains(svc, "Type=oneshot") {
		t.Error("healthcheck service should be oneshot")
	}
	if !strings.Contains(svc, "conctl healthcheck") {
		t.Error("healthcheck service should exec conctl healthcheck")
	}
	if !strings.Contains(svc, "conos-status-page") {
		t.Error("healthcheck service should run status page after check")
	}

	timer, ok := units["conos-healthcheck.timer"]
	if !ok {
		t.Fatal("expected conos-healthcheck.timer")
	}
	if !strings.Contains(timer, "OnUnitActiveSec=300s") {
		t.Error("timer should use configured interval")
	}
	if !strings.Contains(timer, "OnBootSec=30s") {
		t.Error("timer should run 30s after boot")
	}
	if !strings.Contains(timer, "WantedBy=timers.target") {
		t.Error("timer should be wanted by timers.target")
	}
}

func TestGenerateHealthcheckUnits_CustomInterval(t *testing.T) {
	units := GenerateHealthcheckUnits("60s")
	timer := units["conos-healthcheck.timer"]
	if !strings.Contains(timer, "OnUnitActiveSec=60s") {
		t.Error("timer should use the given interval")
	}
}

func TestHasSudo_Sysadmin(t *testing.T) {
	agent := config.AgentConfig{Name: "sysadmin", Roles: []string{"sysadmin"}}
	if !hasSudo(agent) {
		t.Error("sysadmin role should have sudo")
	}
}

func TestHasSudo_NonSysadmin(t *testing.T) {
	agent := config.AgentConfig{Name: "concierge", Roles: []string{"router"}}
	if hasSudo(agent) {
		t.Error("non-sysadmin role should not have sudo")
	}
}

func TestHasSudo_NoRoles(t *testing.T) {
	agent := config.AgentConfig{Name: "worker"}
	if hasSudo(agent) {
		t.Error("agent with no roles should not have sudo")
	}
}

func TestHasSudo_MultipleRolesIncludingSysadmin(t *testing.T) {
	agent := config.AgentConfig{Name: "sysadmin", Roles: []string{"operator", "sysadmin", "monitor"}}
	if !hasSudo(agent) {
		t.Error("agent with sysadmin among multiple roles should have sudo")
	}
}

func TestServiceHardeningOfficer(t *testing.T) {
	agent := config.AgentConfig{
		Name: "ceo",
		Tier: "officer",
		Mode: "on-demand",
	}

	units := GenerateUnits(agent)
	svc := units["conos-ceo.service"]

	// Officer gets same hardening as operator: NoNewPrivileges, ProtectSystem, syscall filter
	for _, directive := range []string{
		"NoNewPrivileges=yes",
		"ProtectSystem=strict",
		"ReadWritePaths=/srv/conos/agents",
		"ReadWritePaths=/srv/conos/artifacts",
		"ReadWritePaths=/srv/conos/logs/audit",
		"SystemCallFilter=@system-service",
		"SystemCallErrorNumber=EPERM",
	} {
		if !strings.Contains(svc, directive) {
			t.Errorf("officer service should contain %s", directive)
		}
	}

	// Officer should NOT have BindReadOnlyPaths (that's workers only)
	if strings.Contains(svc, "BindReadOnlyPaths=/srv/conos/agents") {
		t.Error("officer should NOT have BindReadOnlyPaths (needs write for delegation)")
	}
}

func TestServiceHardeningBindPaths(t *testing.T) {
	agent := config.AgentConfig{
		Name: "researcher",
		Tier: "worker",
		Mode: "on-demand",
	}

	units := GenerateUnits(agent)
	svc := units["conos-researcher.service"]

	// All agents get home and agent dir bind paths
	if !strings.Contains(svc, "BindPaths=/home/a-researcher") {
		t.Error("service should bind agent home dir")
	}
	if !strings.Contains(svc, "BindPaths=/srv/conos/agents/researcher") {
		t.Error("service should bind agent workspace dir")
	}
}

func TestServiceHardeningSysadminWritePaths(t *testing.T) {
	agent := config.AgentConfig{
		Name:  "sysadmin",
		Tier:  "operator",
		Mode:  "on-demand",
		Roles: []string{"sysadmin"},
	}

	units := GenerateUnits(agent)
	svc := units["conos-sysadmin.service"]

	// Sysadmin gets broad write paths for commissioning
	for _, path := range []string{
		"ReadWritePaths=/srv/conos/agents",
		"ReadWritePaths=/srv/conos/config",
		"ReadWritePaths=/srv/conos/contracts",
		"ReadWritePaths=/etc/conos",
		"ReadWritePaths=/etc/sudoers.d",
		"ReadWritePaths=/etc/systemd/system",
	} {
		if !strings.Contains(svc, path) {
			t.Errorf("sysadmin service should contain %s", path)
		}
	}
}

func TestServiceEnvironmentFile(t *testing.T) {
	agent := config.AgentConfig{
		Name: "concierge",
		Tier: "operator",
		Mode: "on-demand",
	}

	units := GenerateUnits(agent)
	svc := units["conos-concierge.service"]

	// EnvironmentFile with - prefix means optional (no error if missing)
	if !strings.Contains(svc, "EnvironmentFile=-/etc/conos/env") {
		t.Error("service should load environment from /etc/conos/env")
	}
}

func TestContinuousServiceWithEnvVars(t *testing.T) {
	agent := config.AgentConfig{
		Name:        "monitor",
		Tier:        "worker",
		Mode:        "continuous",
		Environment: []string{"POLL_INTERVAL=30s"},
	}

	units := GenerateUnits(agent)
	svc := units["conos-monitor.service"]

	if !strings.Contains(svc, "POLL_INTERVAL=30s") {
		t.Error("continuous service should include agent env vars")
	}
	if !strings.Contains(svc, "Type=simple") {
		t.Error("continuous service should be Type=simple")
	}
}

func TestOnDemandServiceIsOneshot(t *testing.T) {
	agent := config.AgentConfig{
		Name: "concierge",
		Tier: "operator",
		Mode: "on-demand",
	}

	units := GenerateUnits(agent)
	svc := units["conos-concierge.service"]

	if !strings.Contains(svc, "Type=oneshot") {
		t.Error("on-demand service should be Type=oneshot")
	}
}

func TestCronServiceIsOneshot(t *testing.T) {
	agent := config.AgentConfig{
		Name: "reporter",
		Tier: "worker",
		Mode: "cron",
		Cron: "daily",
	}

	units := GenerateUnits(agent)
	svc := units["conos-reporter.service"]

	if !strings.Contains(svc, "Type=oneshot") {
		t.Error("cron service should be Type=oneshot (timer triggers it)")
	}
}

func TestCronTimerPersistent(t *testing.T) {
	agent := config.AgentConfig{
		Name: "reporter",
		Tier: "worker",
		Mode: "cron",
		Cron: "*-*-* 09:00:00",
	}

	units := GenerateUnits(agent)
	timer := units["conos-reporter.timer"]

	if !strings.Contains(timer, "Persistent=true") {
		t.Error("cron timer should be persistent (catches up missed runs)")
	}
}

func TestGenerateUnitsCount_OnDemand(t *testing.T) {
	agent := config.AgentConfig{Name: "test", Tier: "worker", Mode: "on-demand"}
	units := GenerateUnits(agent)
	if len(units) != 2 {
		t.Errorf("on-demand agent should produce 2 units (.service + .path), got %d", len(units))
	}
}

func TestGenerateUnitsCount_Continuous(t *testing.T) {
	agent := config.AgentConfig{Name: "test", Tier: "worker", Mode: "continuous"}
	units := GenerateUnits(agent)
	if len(units) != 1 {
		t.Errorf("continuous agent should produce 1 unit (.service only), got %d", len(units))
	}
}

func TestGenerateUnitsCount_Cron(t *testing.T) {
	agent := config.AgentConfig{Name: "test", Tier: "worker", Mode: "cron", Cron: "daily"}
	units := GenerateUnits(agent)
	if len(units) != 3 {
		t.Errorf("cron agent should produce 3 units (.service + .timer + .path), got %d", len(units))
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
