package bootstrap

import (
	"fmt"
	"strings"

	"github.com/ConspiracyOS/conctl/internal/config"
)

// Manifest describes the expected system state for a ConspiracyOS instance.
// Generated from Config, consumed by ProvisionFromManifest and Verify.
type Manifest struct {
	Groups        []Group        `yaml:"groups"`
	Users         []User         `yaml:"users"`
	Directories   []Directory    `yaml:"directories"`
	Files         []File         `yaml:"files"`
	ACLs          []ACL          `yaml:"acls"`
	Units         []SystemdUnit  `yaml:"units"`
	SetupCommands []SetupCommand `yaml:"setup_commands"`
}

type Group struct {
	Name string `yaml:"name"`
}

type User struct {
	Name   string   `yaml:"name"`
	Home   string   `yaml:"home"`
	Shell  string   `yaml:"shell"`
	Groups []string `yaml:"groups"`
}

type Directory struct {
	Path  string `yaml:"path"`
	Mode  string `yaml:"mode"`
	Owner string `yaml:"owner"`
	Group string `yaml:"group"`
}

type File struct {
	Path    string `yaml:"path"`
	Mode    string `yaml:"mode"`
	Owner   string `yaml:"owner"`
	Group   string `yaml:"group"`
	Content string `yaml:"content,omitempty"` // empty = don't manage content, just metadata
}

type ACL struct {
	Path    string `yaml:"path"`
	User    string `yaml:"user,omitempty"`
	Group   string `yaml:"group,omitempty"`
	Perms   string `yaml:"perms"`             // rwx, rx, etc.
	Default bool   `yaml:"default,omitempty"` // -d flag (default ACL for new files)
}

type SystemdUnit struct {
	Name    string `yaml:"name"`
	Content string `yaml:"content"`
	Enabled bool   `yaml:"enabled"`
}

type SetupCommand struct {
	Description string `yaml:"description"`
	Cmd         string `yaml:"cmd"`
}

// BootstrapMode controls how bootstrap applies system state.
type BootstrapMode int

const (
	ModeContainer BootstrapMode = iota // default: owns the entire OS
	ModeSidecar                        // coexists with an existing OS
)

// BootstrapOptions configures bootstrap behavior independent of system config.
type BootstrapOptions struct {
	Mode BootstrapMode
}

const defaultArchwayMCPURL = "http://host.docker.internal:8893/mcp"

func isClaudeRunner(name string) bool {
	runner := strings.ToLower(name)
	return runner == "claude" || runner == "claude-code"
}

func requiresClaudeCodeCLI(cfg *config.Config) bool {
	for _, a := range cfg.Agents {
		resolved := cfg.ResolvedAgent(a.Name)
		if isClaudeRunner(resolved.Runner) {
			return true
		}
	}
	return false
}

func homeDirForUser(user string) string {
	if user == "root" {
		return "/root"
	}
	return "/home/" + user
}

func configureClaudeMCPCommand(user, group string) SetupCommand {
	settingsPath := homeDirForUser(user) + "/.claude.json"
	return SetupCommand{
		Description: fmt.Sprintf("configure Claude MCP for %s", user),
		Cmd: fmt.Sprintf(`python3 - <<'PY'
import json
from pathlib import Path

path = Path(%q)
data = {}
if path.exists():
    try:
        data = json.loads(path.read_text())
    except Exception:
        data = {}
if not isinstance(data, dict):
    data = {}
servers = data.get("mcpServers")
if not isinstance(servers, dict):
    servers = {}
servers["archway"] = {"type": "http", "url": %q}
data["mcpServers"] = servers
path.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n")
PY
chown %s:%s %s
chmod 600 %s`, settingsPath, defaultArchwayMCPURL, user, group, settingsPath, settingsPath),
	}
}

// FromConfig generates a Manifest from the current config.
func FromConfig(cfg *config.Config, opts BootstrapOptions) Manifest {
	var m Manifest

	// Groups
	for _, name := range []string{"agents", "officers", "operators", "workers", "trusted"} {
		m.Groups = append(m.Groups, Group{Name: name})
	}
	for _, a := range cfg.Agents {
		m.Groups = append(m.Groups, Group{Name: "can-task-" + a.Name})
	}

	// Users
	for _, a := range cfg.Agents {
		user := "a-" + a.Name
		groups := []string{"agents"}
		switch a.Tier {
		case "officer":
			groups = append(groups, "officers")
		case "operator":
			groups = append(groups, "operators")
		}
		m.Users = append(m.Users, User{
			Name:   user,
			Home:   "/home/" + user,
			Shell:  "/bin/bash",
			Groups: groups,
		})
	}

	// Top-level directories
	topDirs := []Directory{
		{Path: "/etc/conos", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/etc/conos/base", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/etc/conos/roles", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/etc/conos/groups", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/etc/conos/scopes", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/etc/conos/agents", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/srv/conos", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/srv/conos/inbox", Mode: "770", Owner: "root", Group: "agents"},
		{Path: "/srv/conos/artifacts", Mode: "775", Owner: "root", Group: "root"},
		{Path: "/srv/conos/config", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/srv/conos/config/agents", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/srv/conos/contracts", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/srv/conos/logs", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/srv/conos/logs/audit", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/srv/conos/status", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/srv/conos/scopes", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/srv/conos/policy", Mode: "755", Owner: "root", Group: "root"},
		{Path: "/srv/conos/ledger", Mode: "755", Owner: "root", Group: "root"},
	}
	m.Directories = append(m.Directories, topDirs...)

	// Per-agent directories
	for _, a := range cfg.Agents {
		user := "a-" + a.Name
		base := fmt.Sprintf("/srv/conos/agents/%s", a.Name)
		for _, sub := range []string{"", "/inbox", "/outbox", "/workspace", "/sessions", "/processed"} {
			m.Directories = append(m.Directories, Directory{
				Path: base + sub, Mode: "700", Owner: user, Group: "agents",
			})
		}
	}

	// Files
	m.Files = append(m.Files,
		File{Path: "/etc/conos/env", Mode: "600", Owner: "root", Group: "root"},
		File{Path: "/etc/conos/artifact-signing.key", Mode: "600", Owner: "root", Group: "root"},
	)

	// ACLs — derived from agent tiers.
	// Base ACL: agents group gets traverse on agent dirs so group members
	// (including vegard) can access them. Without this, setfacl's user entries
	// cause group::--- which blocks all group access via POSIX ACL precedence.
	for _, a := range cfg.Agents {
		base := fmt.Sprintf("/srv/conos/agents/%s", a.Name)
		m.ACLs = append(m.ACLs,
			ACL{Path: base, Group: "agents", Perms: "rx"},
		)
	}

	// Officers and operators can task any agent. Workers cannot task.
	for _, src := range cfg.Agents {
		srcUser := "a-" + src.Name

		for _, dst := range cfg.Agents {
			if src.Name == dst.Name {
				continue
			}

			canTask := false
			switch src.Tier {
			case "officer", "operator":
				canTask = true
			}

			if canTask {
				m.ACLs = append(m.ACLs,
					ACL{Path: fmt.Sprintf("/srv/conos/agents/%s", dst.Name), User: srcUser, Perms: "x"},
					ACL{Path: fmt.Sprintf("/srv/conos/agents/%s/inbox", dst.Name), User: srcUser, Perms: "rwx"},
				)
			}
		}

		// Role-based: sysadmin role gets config + contracts write access
		for _, role := range src.Roles {
			if role == "sysadmin" {
				m.ACLs = append(m.ACLs,
					ACL{Path: "/srv/conos/config/agents", User: srcUser, Perms: "rwx"},
					ACL{Path: "/srv/conos/contracts", User: srcUser, Perms: "rwx"},
				)
			}
		}
	}

	// Shared dirs: all agents can write audit logs and ledger entries
	m.ACLs = append(m.ACLs,
		ACL{Path: "/srv/conos/logs/audit", Group: "agents", Perms: "rwx"},
		ACL{Path: "/srv/conos/logs/audit", Group: "agents", Perms: "rw", Default: true},
		ACL{Path: "/srv/conos/ledger", Group: "agents", Perms: "rwx"},
		ACL{Path: "/srv/conos/ledger", Group: "agents", Perms: "rw", Default: true},
	)

	// Systemd units — agent units
	for _, a := range cfg.Agents {
		resolved := cfg.ResolvedAgent(a.Name)
		agentUnits := GenerateUnits(resolved)
		for name, content := range agentUnits {
			enabled := true
			m.Units = append(m.Units, SystemdUnit{Name: name, Content: content, Enabled: enabled})
		}
	}

	// Healthcheck units
	interval := cfg.Contracts.System.HealthcheckInterval
	if interval == "" {
		interval = "60s"
	}
	for name, content := range GenerateHealthcheckUnits(interval) {
		m.Units = append(m.Units, SystemdUnit{Name: name, Content: content, Enabled: true})
	}

	// Outer inbox watcher
	m.Units = append(m.Units, SystemdUnit{
		Name: "conos-outer-inbox.path",
		Content: `[Unit]
Description=ConspiracyOS outer inbox watcher

[Path]
PathChanged=/srv/conos/inbox
MakeDirectory=yes

[Install]
WantedBy=multi-user.target
`,
		Enabled: true,
	})
	m.Units = append(m.Units, SystemdUnit{
		Name: "conos-outer-inbox.service",
		Content: `[Unit]
Description=ConspiracyOS outer inbox -> concierge inbox

[Service]
Type=oneshot
User=a-concierge
ExecStart=/usr/local/bin/conctl route-inbox
EnvironmentFile=-/etc/conos/env
`,
		Enabled: false, // activated by .path, not enabled directly
	})

	// Setup commands — imperative steps that don't fit the declarative model.

	// Clear immutable bits from previous bootstrap on top-level config files.
	// This must happen before any writes to these paths.
	m.SetupCommands = append(m.SetupCommands,
		SetupCommand{Description: "clear immutable: conos.toml", Cmd: "chattr -i /etc/conos/conos.toml 2>/dev/null || true"},
		SetupCommand{Description: "clear immutable: env", Cmd: "chattr -i /etc/conos/env 2>/dev/null || true"},
		SetupCommand{Description: "clear immutable: signing key", Cmd: "chattr -i /etc/conos/artifact-signing.key 2>/dev/null || true"},
		SetupCommand{Description: "clear immutable: conctl binary", Cmd: "chattr -i /usr/local/bin/conctl 2>/dev/null || true"},
	)

	// SSH authorized keys
	if len(cfg.Infra.SSHAuthorizedKeys) > 0 {
		m.SetupCommands = append(m.SetupCommands,
			SetupCommand{Description: "create SSH dir", Cmd: "install -d -m 700 /root/.ssh"},
		)
		for _, key := range cfg.Infra.SSHAuthorizedKeys {
			if strings.ContainsAny(key, "'\\") {
				continue
			}
			m.SetupCommands = append(m.SetupCommands, SetupCommand{
				Description: "add SSH key",
				Cmd: fmt.Sprintf(
					`grep -qxF '%s' /root/.ssh/authorized_keys 2>/dev/null || echo '%s' >> /root/.ssh/authorized_keys`,
					key, key,
				),
			})
		}
		m.SetupCommands = append(m.SetupCommands,
			SetupCommand{Description: "fix SSH key perms", Cmd: "chmod 600 /root/.ssh/authorized_keys"},
		)
	}

	// Sudoers — clear immutable bits from previous runs before overwriting
	validateCmd := "visudo -c || echo 'warn: sudoers validation failed'"
	if opts.Mode == ModeSidecar {
		validateCmd = "visudo -c || exit 1"
	}
	m.SetupCommands = append(m.SetupCommands,
		SetupCommand{Description: "clear immutable: sudoers", Cmd: "find /etc/sudoers.d -name 'conos-*' -exec chattr -i {} + 2>/dev/null || true"},
		SetupCommand{Description: "install sudoers", Cmd: "cp /etc/conos/sudoers.d/* /etc/sudoers.d/ 2>/dev/null || true"},
		SetupCommand{Description: "fix sudoers perms", Cmd: "chmod 440 /etc/sudoers.d/conos-* 2>/dev/null || true"},
		SetupCommand{Description: "validate sudoers", Cmd: validateCmd},
	)

	// Copy system contracts — clear immutable bits from previous runs
	m.SetupCommands = append(m.SetupCommands,
		SetupCommand{Description: "clear immutable: contracts", Cmd: "find /srv/conos/contracts -name '*.yaml' -exec chattr -i {} + 2>/dev/null || true"},
		SetupCommand{Description: "copy contracts", Cmd: "cp /etc/conos/contracts/*.yaml /srv/conos/contracts/ 2>/dev/null || true"},
		SetupCommand{Description: "copy contract scripts", Cmd: "cp -r /etc/conos/contracts/scripts/ /srv/conos/contracts/scripts/ 2>/dev/null || true"},
	)

	// Initialize /srv/conos as git repo
	m.SetupCommands = append(m.SetupCommands, SetupCommand{
		Description: "git init /srv/conos",
		Cmd: `git config --system --add safe.directory /srv/conos
cd /srv/conos && git init && git config user.name 'conos' && git config user.email 'conos@localhost' && git config core.sharedRepository group && cat > .gitignore << 'GITIGNORE'
agents/*/workspace/
artifacts/
*.env
*.pem
*.key
GITIGNORE
git add -A && git commit -m 'initial state' --allow-empty || true
chown -R root:agents /srv/conos/.git && chmod -R g+w /srv/conos/.git`,
	})

	// Signing key generation
	m.SetupCommands = append(m.SetupCommands, SetupCommand{
		Description: "generate artifact signing key",
		Cmd:         "test -f /etc/conos/artifact-signing.key || (umask 077 && od -An -tx1 -N32 /dev/urandom | tr -d ' \\n' > /etc/conos/artifact-signing.key)",
	})

	// Enable auditd
	m.SetupCommands = append(m.SetupCommands,
		SetupCommand{Description: "enable auditd", Cmd: "systemctl enable --now auditd 2>/dev/null || true"},
	)

	// nftables: per-agent outbound filtering
	if nftRules := GenerateNftRules(cfg); nftRules != "" {
		if opts.Mode == ModeSidecar {
			// Sidecar: write to conos-owned file and load additively.
			// The generated rules use a self-contained "table inet conos"
			// so they don't interfere with existing firewall rules.
			m.SetupCommands = append(m.SetupCommands,
				SetupCommand{
					Description: "write nftables rules",
					Cmd:         fmt.Sprintf("cat > /etc/conos/nftables.conf << 'NFTEOF'\n%sNFTEOF", nftRules),
				},
				SetupCommand{
					Description: "apply nftables rules (additive)",
					Cmd:         "nft -f /etc/conos/nftables.conf",
				},
			)
		} else {
			m.SetupCommands = append(m.SetupCommands,
				SetupCommand{
					Description: "write nftables rules",
					Cmd:         fmt.Sprintf("cat > /etc/nftables.conf << 'NFTEOF'\n%sNFTEOF", nftRules),
				},
				SetupCommand{
					Description: "apply nftables rules",
					Cmd:         "nft -f /etc/nftables.conf",
				},
				SetupCommand{
					Description: "enable nftables",
					Cmd:         "systemctl enable nftables 2>/dev/null || true",
				},
			)
		}
	}

	// Tailscale
	if cfg.Infra.TailscaleHostname != "" {
		loginServerFlag := ""
		if cfg.Infra.TailscaleLoginServer != "" {
			loginServerFlag = fmt.Sprintf(" --login-server=%s", cfg.Infra.TailscaleLoginServer)
		}
		m.SetupCommands = append(m.SetupCommands, SetupCommand{
			Description: "tailscale setup",
			Cmd: fmt.Sprintf(`if [ -f /var/lib/tailscale-persist/tailscaled.state ]; then
    mkdir -p /var/lib/tailscale
    cp /var/lib/tailscale-persist/tailscaled.state /var/lib/tailscale/tailscaled.state
    echo "tailscale: restored state from persistent volume"
fi
systemctl restart tailscaled 2>/dev/null || true
sleep 3
TS_STATUS=$(tailscale status --json 2>/dev/null | jq -r '.BackendState // "NoState"' 2>/dev/null || echo "NoState")
if [ "$TS_STATUS" = "Running" ]; then
    echo "tailscale: already connected ($(tailscale ip -4))"
else
    TSKEY="${TS_AUTHKEY:-$TS_AUTH_KEY}"
    if [ -n "$TSKEY" ]; then
        tailscale up --hostname=%s --authkey="$TSKEY"%s --accept-routes
        echo "tailscale: $(tailscale ip -4)"
    else
        echo "warn: tailscale not connected and no TS_AUTHKEY"
    fi
fi
if [ -d /var/lib/tailscale-persist ] && [ -f /var/lib/tailscale/tailscaled.state ]; then
    cp /var/lib/tailscale/tailscaled.state /var/lib/tailscale-persist/tailscaled.state
    echo "tailscale: state persisted to volume"
fi`, cfg.Infra.TailscaleHostname, loginServerFlag),
		})
	}

	// Dashboard (nginx)
	if cfg.Dashboard.Enabled {
		nginxConf := fmt.Sprintf(`server {
    listen %s:%d;
    root /srv/conos/status;
    index index.html;
    location / {
        limit_except GET HEAD { deny all; }
    }
}`, cfg.Dashboard.Bind, cfg.Dashboard.Port)
		m.SetupCommands = append(m.SetupCommands,
			SetupCommand{Description: "write nginx config", Cmd: fmt.Sprintf("cat > /etc/nginx/sites-available/conspiracyos << 'EOF'\n%s\nEOF", nginxConf)},
			SetupCommand{Description: "enable nginx site", Cmd: "ln -sf /etc/nginx/sites-available/conspiracyos /etc/nginx/sites-enabled/conspiracyos"},
		)
		if cfg.Infra.TailscaleHostname != "" {
			m.SetupCommands = append(m.SetupCommands, SetupCommand{
				Description: "bind nginx to tailscale IP",
				Cmd: `TSIP=$(tailscale ip -4 2>/dev/null || true)
if [ -n "$TSIP" ]; then
    sed -i "s/listen .*/listen $TSIP:80;/" /etc/nginx/sites-enabled/conspiracyos
fi`,
			})
		}
		m.SetupCommands = append(m.SetupCommands,
			SetupCommand{Description: "start nginx", Cmd: "systemctl enable --now nginx"},
			SetupCommand{Description: "generate initial status page", Cmd: "/usr/local/bin/conos-status-page"},
		)
	} else {
		m.SetupCommands = append(m.SetupCommands,
			SetupCommand{Description: "disable nginx", Cmd: "systemctl disable --now nginx 2>/dev/null || true"},
			SetupCommand{Description: "remove nginx site", Cmd: "rm -f /etc/nginx/sites-enabled/conspiracyos"},
		)
	}

	// Remove stale systemd units for agents no longer in config.
	// Build the set of expected unit filenames, then disable+remove any
	// conos-*.{service,path,timer} files that aren't in the set.
	expectedUnits := map[string]bool{}
	for _, u := range m.Units {
		expectedUnits[u.Name] = true
	}
	// Also keep outer-inbox units (not agent-specific)
	expectedUnits["conos-outer-inbox.path"] = true
	expectedUnits["conos-outer-inbox.service"] = true
	var keepList []string
	for name := range expectedUnits {
		keepList = append(keepList, name)
	}
	m.SetupCommands = append(m.SetupCommands, SetupCommand{
		Description: "remove stale agent units",
		Cmd: fmt.Sprintf(`for unit in /etc/systemd/system/conos-*.service /etc/systemd/system/conos-*.path /etc/systemd/system/conos-*.timer; do
  [ -f "$unit" ] || continue
  name=$(basename "$unit")
  case "$name" in %s) continue ;; esac
  systemctl disable --now "$name" 2>/dev/null || true
  rm -f "$unit"
  echo "removed stale unit: $name"
done`, strings.Join(keepList, "|")),
	})

	// Systemd reload and unit enablement
	m.SetupCommands = append(m.SetupCommands,
		SetupCommand{Description: "reload systemd", Cmd: "systemctl daemon-reload"},
		// Only enable healthcheck timer if it wasn't explicitly disabled.
		// On first bootstrap (unit doesn't exist yet), is-enabled returns "not-found"
		// which is not "disabled", so this will enable it.
		SetupCommand{Description: "enable healthcheck timer", Cmd: `if [ "$(systemctl is-enabled conos-healthcheck.timer 2>/dev/null)" != "disabled" ]; then systemctl enable --now conos-healthcheck.timer; fi`},
	)
	for _, a := range cfg.Agents {
		switch a.Mode {
		case "on-demand", "":
			m.SetupCommands = append(m.SetupCommands, SetupCommand{
				Description: fmt.Sprintf("enable %s inbox watcher", a.Name),
				Cmd:         fmt.Sprintf("systemctl enable --now conos-%s.path", a.Name),
			})
		case "continuous":
			m.SetupCommands = append(m.SetupCommands, SetupCommand{
				Description: fmt.Sprintf("enable %s service", a.Name),
				Cmd:         fmt.Sprintf("systemctl enable --now conos-%s.service", a.Name),
			})
		case "cron":
			m.SetupCommands = append(m.SetupCommands, SetupCommand{
				Description: fmt.Sprintf("enable %s timer", a.Name),
				Cmd:         fmt.Sprintf("systemctl enable --now conos-%s.timer", a.Name),
			})
		}
	}

	// Enable outer inbox watcher
	m.SetupCommands = append(m.SetupCommands,
		SetupCommand{Description: "enable outer inbox watcher", Cmd: "systemctl enable --now conos-outer-inbox.path"},
	)

	// Auditd watches on critical paths
	m.SetupCommands = append(m.SetupCommands,
		SetupCommand{Description: "auditd watch /etc/conos", Cmd: "auditctl -w /etc/conos/ -p wa -k conos_config_tamper 2>/dev/null || true"},
		SetupCommand{Description: "auditd watch /etc/sudoers.d", Cmd: "auditctl -w /etc/sudoers.d/ -p wa -k conos_sudoers_tamper 2>/dev/null || true"},
		SetupCommand{Description: "auditd watch systemd units", Cmd: "auditctl -w /etc/systemd/system/ -p wa -k conos_systemd_tamper 2>/dev/null || true"},
		SetupCommand{Description: "auditd watch conctl binary", Cmd: "auditctl -w /usr/local/bin/conctl -p wa -k conos_binary_tamper 2>/dev/null || true"},
	)

	// Install agent packages declared in config (before skills, which may depend on them)
	var allPkgs []string
	for _, a := range cfg.Agents {
		allPkgs = append(allPkgs, a.Packages...)
	}
	// Note: Claude Code CLI is installed as a native binary (not via npm)
	// to avoid Node.js version incompatibilities (mono#17).
	if len(allPkgs) > 0 {
		// Deduplicate
		seen := map[string]bool{}
		var unique []string
		for _, p := range allPkgs {
			if !seen[p] {
				seen[p] = true
				unique = append(unique, p)
			}
		}
		if opts.Mode == ModeSidecar {
			m.SetupCommands = append(m.SetupCommands, SetupCommand{
				Description: "list required packages",
				Cmd:         fmt.Sprintf("echo 'sidecar: install these packages manually: %s'", strings.Join(unique, " ")),
			})
		} else {
			m.SetupCommands = append(m.SetupCommands, SetupCommand{
				Description: "install agent packages",
				Cmd:         fmt.Sprintf("apt-get install -y --no-install-recommends %s", strings.Join(unique, " ")),
			})
		}
	}
	if requiresClaudeCodeCLI(cfg) {
		if opts.Mode == ModeSidecar {
			m.SetupCommands = append(m.SetupCommands, SetupCommand{
				Description: "install Claude Code CLI manually",
				Cmd:         "echo 'sidecar: install Claude Code CLI: curl -fsSL https://console.anthropic.com/install.sh | sh && cp ~/.claude/local/claude /usr/local/bin/claude && ln -sf /usr/local/bin/claude /usr/local/bin/claude-code'",
			})
		} else {
			// Install native binary — avoids Node.js version issues (mono#17).
			// The install script places the binary under ~/.claude/local/claude (as root).
			// We copy it to a system path so agent users can execute it.
			m.SetupCommands = append(m.SetupCommands,
				SetupCommand{
					Description: "install Claude Code CLI (native)",
					Cmd: `if ! command -v claude >/dev/null 2>&1 || ! claude --version >/dev/null 2>&1; then
    curl -fsSL https://console.anthropic.com/install.sh | sh
    cp /root/.claude/local/claude /usr/local/bin/claude
    chmod 755 /usr/local/bin/claude
    ln -sf /usr/local/bin/claude /usr/local/bin/claude-code
fi`,
				},
			)
		}

		m.SetupCommands = append(m.SetupCommands, configureClaudeMCPCommand("root", "root"))
		for _, a := range cfg.Agents {
			user := "a-" + a.Name
			m.SetupCommands = append(m.SetupCommands, configureClaudeMCPCommand(user, "agents"))
		}
	}

	// AGENTS.md ownership fix and skill deployment for each agent
	for _, a := range cfg.Agents {
		user := "a-" + a.Name

		// AGENTS.md assembly is handled in main.go (needs runner package).
		// Here we just fix ownership after assembly. Clear immutable first for reruns.
		m.SetupCommands = append(m.SetupCommands, SetupCommand{
			Description: fmt.Sprintf("clear immutable: AGENTS.md for %s", a.Name),
			Cmd:         fmt.Sprintf("chattr -i /home/%s/AGENTS.md 2>/dev/null || true", user),
		}, SetupCommand{
			Description: fmt.Sprintf("fix AGENTS.md ownership for %s", a.Name),
			Cmd:         fmt.Sprintf("chown root:root /home/%s/AGENTS.md 2>/dev/null || true && chmod 0444 /home/%s/AGENTS.md 2>/dev/null || true", user, user),
		})

		// Deploy skills from roles and agent-specific dirs — clear immutable first for reruns
		skillsDir := fmt.Sprintf("/srv/conos/agents/%s/workspace/skills", a.Name)
		cpCmd := fmt.Sprintf("find /etc/conos/roles -name '*.md' -exec chattr -i {} + 2>/dev/null || true && mkdir -p %s", skillsDir)
		for _, r := range a.Roles {
			cpCmd += fmt.Sprintf(" && cp /etc/conos/roles/%s/skills/*.md %s/ 2>/dev/null || true", r, skillsDir)
		}
		cpCmd += fmt.Sprintf(" && cp /etc/conos/agents/%s/skills/*.md %s/ 2>/dev/null || true", a.Name, skillsDir)
		cpCmd += fmt.Sprintf(" && chown -R %s:agents %s", user, skillsDir)

		m.SetupCommands = append(m.SetupCommands, SetupCommand{
			Description: fmt.Sprintf("deploy skills for %s", a.Name),
			Cmd:         cpCmd,
		})
	}

	// Immutable bit on critical files — last step after all provisioning.
	// chattr +i prevents modification even by root without CAP_LINUX_IMMUTABLE.
	// Skipped in sidecar mode to avoid surprising the host admin.
	if opts.Mode == ModeSidecar {
		return m
	}
	m.SetupCommands = append(m.SetupCommands,
		SetupCommand{Description: "immutable: config", Cmd: "chattr +i /etc/conos/conos.toml 2>/dev/null || true"},
		SetupCommand{Description: "immutable: env", Cmd: "chattr +i /etc/conos/env 2>/dev/null || true"},
		SetupCommand{Description: "immutable: signing key", Cmd: "chattr +i /etc/conos/artifact-signing.key 2>/dev/null || true"},
		SetupCommand{Description: "immutable: skills", Cmd: "find /etc/conos/roles -name '*.md' -exec chattr +i {} + 2>/dev/null || true"},
		SetupCommand{Description: "immutable: agent instructions", Cmd: "find /etc/conos/agents -name 'AGENTS.md' -exec chattr +i {} + 2>/dev/null || true"},
		SetupCommand{Description: "immutable: contracts", Cmd: "find /etc/conos/contracts -name '*.yaml' -exec chattr +i {} + 2>/dev/null || true"},
		SetupCommand{Description: "immutable: sudoers", Cmd: "find /etc/sudoers.d -name 'conos-*' -exec chattr +i {} + 2>/dev/null || true"},
		SetupCommand{Description: "immutable: systemd units", Cmd: "find /etc/systemd/system -name 'conos-*' -exec chattr +i {} + 2>/dev/null || true"},
		SetupCommand{Description: "immutable: conctl binary", Cmd: "chattr +i /usr/local/bin/conctl 2>/dev/null || true"},
	)

	return m
}
