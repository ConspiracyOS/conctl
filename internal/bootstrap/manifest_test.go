package bootstrap

import (
	"strings"
	"testing"

	"github.com/ConspiracyOS/conctl/internal/config"
	"gopkg.in/yaml.v3"
)

func TestManifestHasGroups(t *testing.T) {
	m := Manifest{
		Groups: []Group{{Name: "agents"}, {Name: "officers"}},
	}
	if len(m.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(m.Groups))
	}
}

func TestManifestHasUsers(t *testing.T) {
	m := Manifest{
		Users: []User{{Name: "a-concierge", Home: "/home/a-concierge", Groups: []string{"agents", "operators"}}},
	}
	if m.Users[0].Name != "a-concierge" {
		t.Fatal("expected user name")
	}
}

func TestManifestHasDirs(t *testing.T) {
	m := Manifest{
		Directories: []Directory{{Path: "/srv/conos", Mode: "755", Owner: "root", Group: "root"}},
	}
	if m.Directories[0].Path != "/srv/conos" {
		t.Fatal("expected dir path")
	}
}

func TestManifestHasACLs(t *testing.T) {
	m := Manifest{
		ACLs: []ACL{{Path: "/srv/conos/agents/sysadmin/inbox", User: "a-concierge", Perms: "rwx"}},
	}
	if m.ACLs[0].User != "a-concierge" {
		t.Fatal("expected ACL user")
	}
}

func TestManifestHasUnits(t *testing.T) {
	m := Manifest{
		Units: []SystemdUnit{{Name: "conos-concierge.service", Content: "[Service]\nType=oneshot"}},
	}
	if m.Units[0].Name != "conos-concierge.service" {
		t.Fatal("expected unit name")
	}
}

func TestManifestHasFiles(t *testing.T) {
	m := Manifest{
		Files: []File{{Path: "/etc/conos/artifact-signing.key", Mode: "600", Owner: "root", Group: "root"}},
	}
	if m.Files[0].Mode != "600" {
		t.Fatal("expected file mode")
	}
}

func TestFromConfig_Groups(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
			{Name: "sysadmin", Tier: "operator"},
		},
	}
	m := FromConfig(cfg)

	names := map[string]bool{}
	for _, g := range m.Groups {
		names[g.Name] = true
	}
	for _, expected := range []string{"agents", "officers", "operators", "workers", "trusted", "can-task-concierge", "can-task-sysadmin"} {
		if !names[expected] {
			t.Errorf("missing group %q", expected)
		}
	}
}

func TestFromConfig_Users(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
			{Name: "ceo", Tier: "officer"},
		},
	}
	m := FromConfig(cfg)

	users := map[string]User{}
	for _, u := range m.Users {
		users[u.Name] = u
	}
	conc, ok := users["a-concierge"]
	if !ok {
		t.Fatal("missing a-concierge user")
	}
	if conc.Home != "/home/a-concierge" {
		t.Errorf("expected home /home/a-concierge, got %q", conc.Home)
	}

	ceo, ok := users["a-ceo"]
	if !ok {
		t.Fatal("missing a-ceo user")
	}
	hasOfficers := false
	for _, g := range ceo.Groups {
		if g == "officers" {
			hasOfficers = true
		}
	}
	if !hasOfficers {
		t.Error("officer tier should have officers group")
	}
}

func TestFromConfig_Directories(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
		},
	}
	m := FromConfig(cfg)

	paths := map[string]Directory{}
	for _, d := range m.Directories {
		paths[d.Path] = d
	}

	if _, ok := paths["/srv/conos"]; !ok {
		t.Error("missing /srv/conos directory")
	}
	inbox, ok := paths["/srv/conos/agents/concierge/inbox"]
	if !ok {
		t.Error("missing concierge inbox directory")
	}
	if inbox.Owner != "a-concierge" {
		t.Errorf("expected inbox owner a-concierge, got %q", inbox.Owner)
	}
	if inbox.Mode != "700" {
		t.Errorf("expected inbox mode 700, got %q", inbox.Mode)
	}
}

func TestFromConfig_ACLs(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
			{Name: "sysadmin", Tier: "operator"},
		},
	}
	m := FromConfig(cfg)

	found := false
	for _, acl := range m.ACLs {
		if acl.User == "a-concierge" && acl.Path == "/srv/conos/agents/sysadmin/inbox" && acl.Perms == "rwx" {
			found = true
		}
	}
	if !found {
		t.Error("missing ACL: concierge -> sysadmin inbox")
	}
}

func TestFromConfig_Files(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
		},
	}
	m := FromConfig(cfg)

	paths := map[string]File{}
	for _, f := range m.Files {
		paths[f.Path] = f
	}
	key, ok := paths["/etc/conos/artifact-signing.key"]
	if !ok {
		t.Error("missing artifact signing key")
	}
	if key.Mode != "600" {
		t.Errorf("expected signing key mode 600, got %q", key.Mode)
	}
}

func TestFromConfig_Units(t *testing.T) {
	cfg := &config.Config{
		Contracts: config.ContractsConfig{
			System: config.SystemContracts{HealthcheckInterval: "60s"},
		},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
		},
	}
	m := FromConfig(cfg)

	names := map[string]bool{}
	for _, u := range m.Units {
		names[u.Name] = true
	}
	if !names["conos-concierge.service"] {
		t.Error("missing concierge service unit")
	}
	if !names["conos-concierge.path"] {
		t.Error("missing concierge path unit")
	}
	if !names["conos-healthcheck.service"] {
		t.Error("missing healthcheck service unit")
	}
	if !names["conos-healthcheck.timer"] {
		t.Error("missing healthcheck timer unit")
	}
	if !names["conos-outer-inbox.path"] {
		t.Error("missing outer inbox path unit")
	}
}

func TestProvisionFromManifest_Groups(t *testing.T) {
	cfg := &config.Config{
		System: config.SystemConfig{Name: "test"},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
			{Name: "sysadmin", Tier: "operator", Mode: "on-demand"},
		},
	}
	m := FromConfig(cfg)
	cmds := ProvisionFromManifest(m)

	// Must create all groups
	groupCount := 0
	for _, c := range cmds {
		if strings.Contains(c, "groupadd") {
			groupCount++
		}
	}
	if groupCount < 7 { // 5 base + 2 can-task
		t.Errorf("expected at least 7 groupadd commands, got %d", groupCount)
	}
}

func TestProvisionFromManifest_Users(t *testing.T) {
	cfg := &config.Config{
		System: config.SystemConfig{Name: "test"},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
		},
	}
	m := FromConfig(cfg)
	cmds := ProvisionFromManifest(m)

	hasUser := false
	for _, c := range cmds {
		if strings.Contains(c, "useradd") && strings.Contains(c, "a-concierge") {
			hasUser = true
		}
	}
	if !hasUser {
		t.Error("expected useradd for a-concierge")
	}
}

func TestProvisionFromManifest_Dirs(t *testing.T) {
	cfg := &config.Config{
		System: config.SystemConfig{Name: "test"},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
		},
	}
	m := FromConfig(cfg)
	cmds := ProvisionFromManifest(m)

	hasInbox := false
	for _, c := range cmds {
		if strings.Contains(c, "install -d") && strings.Contains(c, "/srv/conos/agents/concierge/inbox") {
			hasInbox = true
		}
	}
	if !hasInbox {
		t.Error("expected inbox dir creation")
	}
}

func TestProvisionFromManifest_ACLs(t *testing.T) {
	cfg := &config.Config{
		System: config.SystemConfig{Name: "test"},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
			{Name: "sysadmin", Tier: "operator", Mode: "on-demand"},
		},
	}
	m := FromConfig(cfg)
	cmds := ProvisionFromManifest(m)

	hasACL := false
	for _, c := range cmds {
		if strings.Contains(c, "setfacl") && strings.Contains(c, "a-concierge") {
			hasACL = true
		}
	}
	if !hasACL {
		t.Error("expected setfacl for concierge")
	}
}

func TestProvisionFromManifest_Units(t *testing.T) {
	cfg := &config.Config{
		Contracts: config.ContractsConfig{
			System: config.SystemContracts{HealthcheckInterval: "60s"},
		},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
		},
	}
	m := FromConfig(cfg)
	cmds := ProvisionFromManifest(m)

	hasUnit := false
	for _, c := range cmds {
		if strings.Contains(c, "cat > /etc/systemd/system/") && strings.Contains(c, "conos-concierge") {
			hasUnit = true
		}
	}
	if !hasUnit {
		t.Error("expected systemd unit file write for concierge")
	}
}

func TestManifestYAMLRoundtrip(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
		},
		Contracts: config.ContractsConfig{
			System: config.SystemContracts{HealthcheckInterval: "60s"},
		},
	}
	m := FromConfig(cfg)

	data, err := yaml.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m2 Manifest
	if err := yaml.Unmarshal(data, &m2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(m2.Groups) != len(m.Groups) {
		t.Errorf("groups: expected %d, got %d", len(m.Groups), len(m2.Groups))
	}
	if len(m2.Users) != len(m.Users) {
		t.Errorf("users: expected %d, got %d", len(m.Users), len(m2.Users))
	}
	if len(m2.Directories) != len(m.Directories) {
		t.Errorf("dirs: expected %d, got %d", len(m.Directories), len(m2.Directories))
	}
	if len(m2.Files) != len(m.Files) {
		t.Errorf("files: expected %d, got %d", len(m.Files), len(m2.Files))
	}
	if len(m2.ACLs) != len(m.ACLs) {
		t.Errorf("acls: expected %d, got %d", len(m.ACLs), len(m2.ACLs))
	}
	if len(m2.Units) != len(m.Units) {
		t.Errorf("units: expected %d, got %d", len(m.Units), len(m2.Units))
	}
}
