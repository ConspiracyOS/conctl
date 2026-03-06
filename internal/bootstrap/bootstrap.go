package bootstrap

import (
	"fmt"
	"strings"

	"github.com/ConspiracyOS/conctl/internal/config"
)

// PlanProvision returns the shell commands needed to provision the conspiracy.
// This is a "dry run" — no commands are executed. Use Execute() to run them.
func PlanProvision(cfg *config.Config) []string {
	var cmds []string

	// 1. Create groups
	cmds = append(cmds, "groupadd -f agents")
	cmds = append(cmds, "groupadd -f officers")
	cmds = append(cmds, "groupadd -f operators")
	cmds = append(cmds, "groupadd -f workers")
	cmds = append(cmds, "groupadd -f trusted")

	// Lock down /etc/conos/env — only root should read it.
	// Agents receive env vars via systemd EnvironmentFile= injection.
	cmds = append(cmds, "chmod 600 /etc/conos/env 2>/dev/null || true")
	cmds = append(cmds, "chown root:root /etc/conos/env 2>/dev/null || true")
	cmds = append(cmds, "test -f /etc/conos/artifact-signing.key || (umask 077 && head -c 32 /dev/urandom | xxd -p -c 32 > /etc/conos/artifact-signing.key)")
	cmds = append(cmds, "chmod 600 /etc/conos/artifact-signing.key 2>/dev/null || true")
	cmds = append(cmds, "chown root:root /etc/conos/artifact-signing.key 2>/dev/null || true")

	// Can-task groups (who can write to whose inbox)
	for _, a := range cfg.Agents {
		cmds = append(cmds, fmt.Sprintf("groupadd -f can-task-%s", a.Name))
	}

	// 2. Create users
	for _, a := range cfg.Agents {
		user := "a-" + a.Name
		groups := "agents"
		switch a.Tier {
		case "officer":
			groups += ",officers"
		case "operator":
			groups += ",operators"
		}
		cmds = append(cmds, fmt.Sprintf(
			"useradd -r -m -d /home/%s -s /bin/bash -g agents -G %s %s || true",
			user, groups, user,
		))
		// Ensure home dir exists even if user was pre-created (useradd -m only works on new users)
		cmds = append(cmds, fmt.Sprintf("install -d -o %s -g agents -m 700 /home/%s", user, user))
	}

	// 3. Create directory structure
	// Top-level dirs
	cmds = append(cmds, "install -d -m 755 /etc/conos")
	cmds = append(cmds, "install -d -m 755 /etc/conos/base")
	cmds = append(cmds, "install -d -m 755 /etc/conos/roles")
	cmds = append(cmds, "install -d -m 755 /etc/conos/groups")
	cmds = append(cmds, "install -d -m 755 /etc/conos/scopes")
	cmds = append(cmds, "install -d -m 755 /etc/conos/agents")

	// /srv/conos/
	cmds = append(cmds, "install -d -m 755 /srv/conos")
	cmds = append(cmds, "install -d -o root -g agents -m 0770 /srv/conos/inbox") // root writes, agents group routes
	cmds = append(cmds, "install -d -m 775 /srv/conos/artifacts")
	cmds = append(cmds, "install -d -m 755 /srv/conos/config")
	cmds = append(cmds, "install -d -m 755 /srv/conos/config/agents")
	cmds = append(cmds, "install -d -m 755 /srv/conos/contracts")
	cmds = append(cmds, "install -d -m 755 /srv/conos/logs")
	cmds = append(cmds, "install -d -m 755 /srv/conos/logs/audit")
	cmds = append(cmds, "install -d -m 755 /srv/conos/status")
	cmds = append(cmds, "install -d -m 755 /srv/conos/scopes")
	cmds = append(cmds, "install -d -m 755 /srv/conos/policy")
	cmds = append(cmds, "install -d -m 755 /srv/conos/ledger")

	// Per-agent dirs
	for _, a := range cfg.Agents {
		user := "a-" + a.Name
		base := fmt.Sprintf("/srv/conos/agents/%s", a.Name)
		cmds = append(cmds,
			fmt.Sprintf("install -d -o %s -g agents -m 700 %s", user, base),
			fmt.Sprintf("install -d -o %s -g agents -m 700 %s/inbox", user, base),
			fmt.Sprintf("install -d -o %s -g agents -m 700 %s/outbox", user, base),
			fmt.Sprintf("install -d -o %s -g agents -m 700 %s/workspace", user, base),
			fmt.Sprintf("install -d -o %s -g agents -m 700 %s/sessions", user, base),
			fmt.Sprintf("install -d -o %s -g agents -m 700 %s/processed", user, base),
		)
	}

	// 4. ACLs — default tasking permissions for minimal install
	// With mode 700, agents need explicit traverse (--x) on each other's base dirs
	// to reach the inbox subdirectory.
	// Concierge can task sysadmin: traverse base + rwx inbox
	cmds = append(cmds, "setfacl -m u:a-concierge:x /srv/conos/agents/sysadmin/")
	cmds = append(cmds, "setfacl -m u:a-concierge:rwx /srv/conos/agents/sysadmin/inbox/")
	// Sysadmin can task concierge: traverse base + rwx inbox
	cmds = append(cmds, "setfacl -m u:a-sysadmin:x /srv/conos/agents/concierge/")
	cmds = append(cmds, "setfacl -m u:a-sysadmin:rwx /srv/conos/agents/concierge/inbox/")

	// Sysadmin write access to inner config and contracts (for commissioning)
	cmds = append(cmds, "setfacl -m u:a-sysadmin:rwx /srv/conos/config/agents/")
	cmds = append(cmds, "setfacl -m u:a-sysadmin:rwx /srv/conos/contracts/")
	// All agents can write audit logs and ledger entries (append date-based entries).
	// The -d flag sets default ACLs so new files inherit group-write regardless of
	// each agent's UMask=0077 (which would otherwise strip group bits on creation).
	cmds = append(cmds, "setfacl -m g:agents:rwx -d -m g:agents:rw /srv/conos/logs/audit/")
	cmds = append(cmds, "setfacl -m g:agents:rwx -d -m g:agents:rw /srv/conos/ledger/")

	// 5. SSH authorized keys (for make apply, SSH access)
	if len(cfg.Infra.SSHAuthorizedKeys) > 0 {
		cmds = append(cmds, "install -d -m 700 /root/.ssh")
		for _, key := range cfg.Infra.SSHAuthorizedKeys {
			if strings.ContainsAny(key, "'\\") {
				cmds = append(cmds, "echo 'warn: skipping SSH key with special characters'")
				continue
			}
			cmds = append(cmds, fmt.Sprintf(
				`grep -qxF '%s' /root/.ssh/authorized_keys 2>/dev/null || echo '%s' >> /root/.ssh/authorized_keys`,
				key, key,
			))
		}
		cmds = append(cmds, "chmod 600 /root/.ssh/authorized_keys")
	}

	// 6. Sudoers — install from profile (not hardcoded)
	cmds = append(cmds, "cp /etc/conos/sudoers.d/* /etc/sudoers.d/ 2>/dev/null || true")
	cmds = append(cmds, "chmod 440 /etc/sudoers.d/conos-* 2>/dev/null || true")
	cmds = append(cmds, "visudo -c || echo 'warn: sudoers validation failed'")

	// 7. Install system contracts from outer config
	cmds = append(cmds, "cp /etc/conos/contracts/*.yaml /srv/conos/contracts/ 2>/dev/null || true")
	cmds = append(cmds, "cp -r /etc/conos/contracts/scripts/ /srv/conos/contracts/scripts/ 2>/dev/null || true")

	// 7. Initialize /srv/conos/ as git repo with .gitignore
	// Allow all users (agents run as a-<name>) to use the repo owned by root.
	cmds = append(cmds, "git config --system --add safe.directory /srv/conos")
	cmds = append(cmds, `cd /srv/conos && git init && git config user.name 'conos' && git config user.email 'conos@localhost' && cat > .gitignore << 'GITIGNORE'
agents/*/workspace/
artifacts/
*.env
*.pem
*.key
GITIGNORE
git add -A && git commit -m 'initial state' --allow-empty || true
chown -R root:agents /srv/conos/.git && chmod -R g+w /srv/conos/.git`)

	// 8. Outer inbox watcher — triggers concierge when files land in /srv/conos/inbox
	cmds = append(cmds, `cat > /etc/systemd/system/conos-outer-inbox.path << 'EOF'
[Unit]
Description=ConspiracyOS outer inbox watcher

[Path]
PathChanged=/srv/conos/inbox
MakeDirectory=yes

[Install]
WantedBy=multi-user.target
EOF`)

	cmds = append(cmds, `cat > /etc/systemd/system/conos-outer-inbox.service << 'EOF'
[Unit]
Description=ConspiracyOS outer inbox -> concierge inbox

[Service]
Type=oneshot
User=a-concierge
ExecStart=/usr/local/bin/conctl route-inbox
EnvironmentFile=-/etc/conos/env
EOF`)

	cmds = append(cmds, "systemctl enable --now conos-outer-inbox.path")

	// 9. Ensure Linux audit logging is active.
	cmds = append(cmds, "systemctl enable --now auditd 2>/dev/null || true")

	// 10. Tailscale (if configured — persisted state from volume mount is preferred)
	if cfg.Infra.TailscaleHostname != "" {
		loginServerFlag := ""
		if cfg.Infra.TailscaleLoginServer != "" {
			loginServerFlag = fmt.Sprintf(" --login-server=%s", cfg.Infra.TailscaleLoginServer)
		}
		cmds = append(cmds, fmt.Sprintf(`# Restore persisted Tailscale state (volume mounted at /var/lib/tailscale-persist)
if [ -f /var/lib/tailscale-persist/tailscaled.state ]; then
    mkdir -p /var/lib/tailscale
    cp /var/lib/tailscale-persist/tailscaled.state /var/lib/tailscale/tailscaled.state
    echo "tailscale: restored state from persistent volume"
fi
# Start tailscaled via systemd
systemctl restart tailscaled 2>/dev/null || true
sleep 3
# Check if already authenticated (restored state)
TS_STATUS=$(tailscale status --json 2>/dev/null | jq -r '.BackendState // "NoState"' 2>/dev/null || echo "NoState")
if [ "$TS_STATUS" = "Running" ]; then
    echo "tailscale: already connected ($(tailscale ip -4))"
else
    TSKEY="${TS_AUTHKEY:-$TS_AUTH_KEY}"
    if [ -n "$TSKEY" ]; then
        tailscale up --hostname=%s --authkey="$TSKEY"%s --accept-routes
        echo "tailscale: $(tailscale ip -4)"
    else
        echo "warn: tailscale not connected and no TS_AUTHKEY — provide key or mount /var/lib/tailscale-persist"
    fi
fi
# Persist state back to volume for next restart
if [ -d /var/lib/tailscale-persist ] && [ -f /var/lib/tailscale/tailscaled.state ]; then
    cp /var/lib/tailscale/tailscaled.state /var/lib/tailscale-persist/tailscaled.state
    echo "tailscale: state persisted to volume"
fi`, cfg.Infra.TailscaleHostname, loginServerFlag))
	}

	// 11. Status page (nginx serves static HTML generated by healthcheck)
	if cfg.Dashboard.Enabled {
		nginxConf := fmt.Sprintf(`server {
    listen %s:%d;
    root /srv/conos/status;
    index index.html;
    location / {
        limit_except GET HEAD { deny all; }
    }
}`, cfg.Dashboard.Bind, cfg.Dashboard.Port)
		cmds = append(cmds, fmt.Sprintf("cat > /etc/nginx/sites-available/conspiracyos << 'EOF'\n%s\nEOF", nginxConf))
		cmds = append(cmds, "ln -sf /etc/nginx/sites-available/conspiracyos /etc/nginx/sites-enabled/conspiracyos")

		// If tailscale is active, override nginx to bind to tailscale IP on port 80
		if cfg.Infra.TailscaleHostname != "" {
			cmds = append(cmds, `TSIP=$(tailscale ip -4 2>/dev/null || true)
if [ -n "$TSIP" ]; then
    sed -i "s/listen .*/listen $TSIP:80;/" /etc/nginx/sites-enabled/conspiracyos
fi`)
		}

		cmds = append(cmds, "systemctl enable --now nginx")

		// Generate initial status page so it's available immediately
		cmds = append(cmds, "/usr/local/bin/conos-status-page")
	} else {
		// Dashboard disabled — ensure nginx is stopped
		cmds = append(cmds, "systemctl disable --now nginx 2>/dev/null || true")
		cmds = append(cmds, "rm -f /etc/nginx/sites-enabled/conspiracyos")
	}

	return cmds
}
