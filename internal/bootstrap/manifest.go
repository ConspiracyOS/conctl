package bootstrap

import (
	"fmt"

	"github.com/ConspiracyOS/conctl/internal/config"
)

// Manifest describes the expected system state for a ConspiracyOS instance.
// Generated from Config, consumed by PlanProvision and Verify.
type Manifest struct {
	Groups      []Group       `yaml:"groups"`
	Users       []User        `yaml:"users"`
	Directories []Directory   `yaml:"directories"`
	Files       []File        `yaml:"files"`
	ACLs        []ACL         `yaml:"acls"`
	Units       []SystemdUnit `yaml:"units"`
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

// FromConfig generates a Manifest from the current config.
func FromConfig(cfg *config.Config) Manifest {
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

	// ACLs — cross-agent tasking + shared dirs.
	// NOTE: concierge↔sysadmin ACLs are hardcoded to match the legacy PlanProvision
	// behavior. These assume both agents exist. When PlanProvision is retired,
	// derive cross-agent ACLs from the actual agent list instead.
	m.ACLs = append(m.ACLs,
		ACL{Path: "/srv/conos/agents/sysadmin", User: "a-concierge", Perms: "x"},
		ACL{Path: "/srv/conos/agents/sysadmin/inbox", User: "a-concierge", Perms: "rwx"},
		ACL{Path: "/srv/conos/agents/concierge", User: "a-sysadmin", Perms: "x"},
		ACL{Path: "/srv/conos/agents/concierge/inbox", User: "a-sysadmin", Perms: "rwx"},
		ACL{Path: "/srv/conos/config/agents", User: "a-sysadmin", Perms: "rwx"},
		ACL{Path: "/srv/conos/contracts", User: "a-sysadmin", Perms: "rwx"},
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

	return m
}
