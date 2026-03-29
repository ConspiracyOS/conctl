package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ConspiracyOS/conctl/internal/config"
)

// ProvisionFromManifest generates shell commands from a Manifest.
// This covers groups, users, directories, files, ACLs, and systemd units.
func ProvisionFromManifest(m Manifest) []string {
	var cmds []string

	// Groups
	for _, g := range m.Groups {
		cmds = append(cmds, fmt.Sprintf("groupadd -f %s", g.Name))
	}

	// Files (pre-user, for things like env and signing key)
	for _, f := range m.Files {
		cmds = append(cmds, fmt.Sprintf("chattr -i %s 2>/dev/null || true", f.Path))
		if f.Content != "" {
			cmds = append(cmds, fmt.Sprintf("cat > %s << 'EOF'\n%sEOF", f.Path, f.Content))
		}
		cmds = append(cmds, fmt.Sprintf("chmod %s %s 2>/dev/null || true", f.Mode, f.Path))
		cmds = append(cmds, fmt.Sprintf("chown %s:%s %s 2>/dev/null || true", f.Owner, f.Group, f.Path))
	}

	// Users
	for _, u := range m.Users {
		if len(u.Groups) == 0 {
			continue
		}
		groupList := u.Groups[0]
		for _, g := range u.Groups[1:] {
			groupList += "," + g
		}
		shell := u.Shell
		if shell == "" {
			shell = "/bin/bash"
		}
		cmds = append(cmds, fmt.Sprintf(
			"useradd -r -m -d %s -s %s -g %s -G %s %s || true",
			u.Home, shell, u.Groups[0], groupList, u.Name,
		))
		cmds = append(cmds, fmt.Sprintf("install -d -o %s -g %s -m 700 %s", u.Name, u.Groups[0], u.Home))
	}

	// Directories
	for _, d := range m.Directories {
		if d.Owner != "" && d.Owner != "root" {
			cmds = append(cmds, fmt.Sprintf("install -d -o %s -g %s -m %s %s", d.Owner, d.Group, d.Mode, d.Path))
		} else if d.Group != "" && d.Group != "root" {
			cmds = append(cmds, fmt.Sprintf("install -d -o %s -g %s -m %s %s", d.Owner, d.Group, d.Mode, d.Path))
		} else {
			cmds = append(cmds, fmt.Sprintf("install -d -m %s %s", d.Mode, d.Path))
		}
	}

	// ACLs
	for _, acl := range m.ACLs {
		flag := ""
		if acl.Default {
			flag = "-d "
		}
		if acl.User != "" {
			cmds = append(cmds, fmt.Sprintf("setfacl %s-m u:%s:%s %s", flag, acl.User, acl.Perms, acl.Path))
		} else if acl.Group != "" {
			cmds = append(cmds, fmt.Sprintf("setfacl %s-m g:%s:%s %s", flag, acl.Group, acl.Perms, acl.Path))
		}
	}

	// Systemd units
	for _, u := range m.Units {
		path := "/etc/systemd/system/" + u.Name
		cmds = append(cmds, fmt.Sprintf("chattr -i %s 2>/dev/null || true", path))
		cmds = append(cmds, fmt.Sprintf("cat > %s << 'UNITEOF'\n%sUNITEOF", path, u.Content))
	}

	// Setup commands (imperative steps)
	for _, sc := range m.SetupCommands {
		cmds = append(cmds, sc.Cmd)
	}

	return cmds
}

// PruneCommands generates shell commands to remove agents, users, groups,
// and systemd units that exist on the system but are not in the current config.
func PruneCommands(cfg *config.Config) []string {
	declared := map[string]bool{}
	for _, a := range cfg.Agents {
		declared[a.Name] = true
	}

	var cmds []string

	// Find stale agent state directories
	agentsDir := "/srv/conos/agents"
	entries, err := os.ReadDir(agentsDir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() || declared[e.Name()] {
				continue
			}
			name := e.Name()
			user := "a-" + name

			// Stop and disable systemd units
			for _, suffix := range []string{".path", ".service", ".timer"} {
				unit := "conos-" + name + suffix
				cmds = append(cmds, fmt.Sprintf("systemctl disable --now %s 2>/dev/null || true", unit))
			}

			// Remove unit files
			unitGlob := filepath.Join("/etc/systemd/system", "conos-"+name+".*")
			matches, _ := filepath.Glob(unitGlob)
			for _, m := range matches {
				cmds = append(cmds, fmt.Sprintf("chattr -i %s 2>/dev/null || true && rm -f %s", m, m))
			}

			// Remove user (also removes from groups)
			cmds = append(cmds, fmt.Sprintf("userdel -r %s 2>/dev/null || true", user))

			// Remove task group
			cmds = append(cmds, fmt.Sprintf("groupdel can-task-%s 2>/dev/null || true", name))

			// Remove state directory
			cmds = append(cmds, fmt.Sprintf("rm -rf %s", filepath.Join(agentsDir, name)))
		}
	}

	// Find stale systemd units that match conos-* but don't belong to any declared agent
	unitDir := "/etc/systemd/system"
	unitEntries, err := os.ReadDir(unitDir)
	if err == nil {
		for _, e := range unitEntries {
			n := e.Name()
			if !strings.HasPrefix(n, "conos-") {
				continue
			}
			// Skip non-agent units
			base := strings.TrimPrefix(n, "conos-")
			for _, suffix := range []string{".path", ".service", ".timer"} {
				base = strings.TrimSuffix(base, suffix)
			}
			// Skip system units (healthcheck, outer-inbox, env, bootstrap)
			if base == "healthcheck" || base == "outer-inbox" || base == "env" || base == "bootstrap" {
				continue
			}
			if !declared[base] {
				cmds = append(cmds, fmt.Sprintf("systemctl disable --now %s 2>/dev/null || true", n))
				cmds = append(cmds, fmt.Sprintf("chattr -i %s 2>/dev/null || true && rm -f %s", filepath.Join(unitDir, n), filepath.Join(unitDir, n)))
			}
		}
	}

	if len(cmds) > 0 {
		cmds = append(cmds, "systemctl daemon-reload")
	}

	return cmds
}
