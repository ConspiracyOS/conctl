package bootstrap

import (
	"fmt"
	"strings"

	"github.com/ConspiracyOS/conctl/internal/config"
)

// agentEnvLines returns systemd Environment= directives for per-agent env vars.
// Each entry in agent.Environment must be "KEY=VALUE". Values with spaces are quoted.
func agentEnvLines(agent config.AgentConfig) string {
	if len(agent.Environment) == 0 {
		return ""
	}
	var b strings.Builder
	for _, kv := range agent.Environment {
		// Quote the value so systemd handles spaces and special chars correctly.
		fmt.Fprintf(&b, "Environment=%q\n", kv)
	}
	return b.String()
}

// GenerateHealthcheckUnits returns systemd units for the contract healthcheck timer.
func GenerateHealthcheckUnits(interval string) map[string]string {
	units := make(map[string]string)

	svc := `[Unit]
Description=ConspiracyOS contract healthcheck
After=network.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/conctl healthcheck
ExecStartPost=-/usr/local/bin/conos-status-page
`
	units["conos-healthcheck.service"] = svc

	timer := fmt.Sprintf(`[Unit]
Description=ConspiracyOS healthcheck timer

[Timer]
OnBootSec=30s
OnUnitActiveSec=%s
AccuracySec=1s

[Install]
WantedBy=timers.target
`, interval)
	units["conos-healthcheck.timer"] = timer

	return units
}

// hasSudo returns true if the agent has a role that grants sudoers access.
// These agents cannot use NoNewPrivileges or ProtectSystem=strict.
func hasSudo(agent config.AgentConfig) bool {
	for _, r := range agent.Roles {
		if r == "sysadmin" {
			return true
		}
	}
	return false
}

// serviceHardening returns systemd hardening directives for an agent.
// Three levels: sysadmin (broad write), officer (delegation write), worker (strict read-only).
// Inter-agent access control is handled by POSIX ACLs, not systemd namespacing.
func serviceHardening(agent config.AgentConfig) string {
	user := "a-" + agent.Name
	// Officers/operators use UMask=0027 so files they create (task routing,
	// audit entries) are group-readable by target agents. Workers keep 0077.
	umask := "0077"
	if agent.Tier != "worker" {
		umask = "0027"
	}
	base := fmt.Sprintf(`PrivateTmp=yes
PrivateDevices=yes
ProtectKernelTunables=yes
ProtectControlGroups=yes
ProtectHome=tmpfs
BindPaths=/home/%s
BindPaths=/srv/conos/agents/%s
UMask=%s
`, user, agent.Name, umask)

	// Syscall filter: @system-service is the systemd-recommended baseline for
	// normal services. We deny groups that agents should never need: kernel
	// module loading, mount manipulation, clock changes, raw I/O, reboot, swap,
	// and obsolete/debug syscalls. Sysadmin is excluded — it needs sudo which
	// requires broader syscall access.
	const syscallFilter = `SystemCallFilter=@system-service
SystemCallFilter=~@mount @clock @module @reboot @swap @raw-io @cpu-emulation @debug @obsolete
SystemCallErrorNumber=EPERM
`

	if hasSudo(agent) {
		// Sysadmin: broad write access for commissioning, config, systemd units.
		// No syscall filter — needs sudo and broad system access.
		base += `ReadWritePaths=/srv/conos/agents
ReadWritePaths=/srv/conos/config
ReadWritePaths=/srv/conos/contracts
ReadWritePaths=/srv/conos/logs
ReadWritePaths=/etc/conos
ReadWritePaths=/etc/sudoers.d
ReadWritePaths=/etc/systemd/system
`
	} else if agent.Tier == "worker" {
		// Workers: no new privileges, root filesystem write-protected.
		// ProtectSystem=strict makes /, /usr, /etc read-only for writes —
		// it does NOT prevent reads. Read access is controlled by POSIX
		// permissions (e.g. /etc/conos/env is mode 600 root:root).
		// Workers cannot task other agents — all routing goes through concierge.
		// Scopes are bind-mounted read-write for coding tasks.
		workerPaths := "BindReadOnlyPaths=/srv/conos/agents\n"
		for _, scope := range agent.Scopes {
			workerPaths += fmt.Sprintf("ReadWritePaths=%s\n", scope)
			workerPaths += fmt.Sprintf("BindPaths=%s\n", scope)
		}
		base += workerPaths + `NoNewPrivileges=yes
ProtectSystem=strict
` + syscallFilter
	} else {
		// Officers and operators: read-only root, but can write to agent inboxes
		// (for routing/delegation), produce artifacts, and write audit logs.
		// POSIX ACLs control which inboxes each agent can access.
		base += `NoNewPrivileges=yes
ProtectSystem=strict
ReadWritePaths=/srv/conos/agents
ReadWritePaths=/srv/conos/artifacts
ReadWritePaths=/srv/conos/logs/audit
ReadWritePaths=/srv/conos/policy
ReadWritePaths=/srv/conos/ledger
` + syscallFilter
	}
	return base
}

// GenerateUnits returns a map of filename → unit file content for a given agent.
func GenerateUnits(agent config.AgentConfig) map[string]string {
	units := make(map[string]string)
	user := "a-" + agent.Name
	svcName := "conos-" + agent.Name
	hardening := serviceHardening(agent)

	// Service unit (always generated)
	// EnvironmentFile loads API keys from /etc/conos/env (written at container start)
	svc := fmt.Sprintf(`[Unit]
Description=ConspiracyOS agent: %s
After=network.target

[Service]
Type=oneshot
User=%s
Group=agents
ExecStart=/usr/local/bin/conctl run %s
WorkingDirectory=/srv/conos/agents/%s/workspace
Environment=HOME=/home/%s
EnvironmentFile=-/etc/conos/env
%s%s
[Install]
WantedBy=multi-user.target
`, agent.Name, user, agent.Name, agent.Name, user, agentEnvLines(agent), hardening)

	units[svcName+".service"] = svc

	switch agent.Mode {
	case "on-demand":
		// Path unit watches inbox
		path := fmt.Sprintf(`[Unit]
Description=ConspiracyOS inbox watcher: %s

[Path]
PathChanged=/srv/conos/agents/%s/inbox
MakeDirectory=yes

[Install]
WantedBy=multi-user.target
`, agent.Name, agent.Name)
		units[svcName+".path"] = path

	case "continuous":
		// Override service to be long-running
		svc = fmt.Sprintf(`[Unit]
Description=ConspiracyOS agent: %s
After=network.target

[Service]
Type=simple
User=%s
Group=agents
ExecStart=/usr/local/bin/conctl run %s --continuous
WorkingDirectory=/srv/conos/agents/%s/workspace
Environment=HOME=/home/%s
EnvironmentFile=-/etc/conos/env
Restart=on-failure
RestartSec=5
%s%s
[Install]
WantedBy=multi-user.target
`, agent.Name, user, agent.Name, agent.Name, user, agentEnvLines(agent), hardening)
		units[svcName+".service"] = svc

	case "cron":
		timer := fmt.Sprintf(`[Unit]
Description=ConspiracyOS timer: %s

[Timer]
OnCalendar=%s
Persistent=true

[Install]
WantedBy=timers.target
`, agent.Name, agent.Cron)
		units[svcName+".timer"] = timer

		// Also watch inbox for on-demand tasks between scheduled runs
		path := fmt.Sprintf(`[Unit]
Description=ConspiracyOS inbox watcher: %s

[Path]
PathChanged=/srv/conos/agents/%s/inbox
MakeDirectory=yes

[Install]
WantedBy=multi-user.target
`, agent.Name, agent.Name)
		units[svcName+".path"] = path
	}

	return units
}
