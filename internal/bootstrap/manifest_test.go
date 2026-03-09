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
		t.Error("missing ACL: operator concierge -> operator sysadmin inbox")
	}
}

func TestFromConfig_ACLs_OfficerCanTaskAll(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "ceo", Tier: "officer"},
			{Name: "concierge", Tier: "operator"},
			{Name: "researcher", Tier: "worker"},
		},
	}
	m := FromConfig(cfg)

	targets := map[string]bool{"concierge": false, "researcher": false}
	for _, acl := range m.ACLs {
		if acl.User == "a-ceo" && strings.Contains(acl.Path, "/inbox") && acl.Perms == "rwx" {
			for name := range targets {
				if strings.Contains(acl.Path, "/"+name+"/") {
					targets[name] = true
				}
			}
		}
	}
	for name, found := range targets {
		if !found {
			t.Errorf("officer ceo should be able to task %s", name)
		}
	}
}

func TestFromConfig_ACLs_WorkerCannotTask(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
			{Name: "researcher", Tier: "worker"},
		},
	}
	m := FromConfig(cfg)

	for _, acl := range m.ACLs {
		if acl.User == "a-researcher" && strings.Contains(acl.Path, "/inbox") {
			t.Errorf("worker should not have inbox ACLs, got %+v", acl)
		}
	}
}

func TestFromConfig_ACLs_OperatorCanTaskNonOfficer(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "ceo", Tier: "officer"},
			{Name: "concierge", Tier: "operator"},
			{Name: "sysadmin", Tier: "operator"},
			{Name: "researcher", Tier: "worker"},
		},
	}
	m := FromConfig(cfg)

	canTask := map[string]bool{}
	for _, acl := range m.ACLs {
		if acl.User == "a-concierge" && strings.Contains(acl.Path, "/inbox") && acl.Perms == "rwx" {
			for _, a := range cfg.Agents {
				if strings.Contains(acl.Path, "/"+a.Name+"/") {
					canTask[a.Name] = true
				}
			}
		}
	}
	if !canTask["sysadmin"] {
		t.Error("operator concierge should task sysadmin")
	}
	if !canTask["researcher"] {
		t.Error("operator concierge should task researcher")
	}
	if canTask["ceo"] {
		t.Error("operator concierge should NOT task officer ceo")
	}
}

func TestFromConfig_ACLs_SysadminRoleGetsConfigAccess(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "sysadmin", Tier: "operator", Roles: []string{"sysadmin"}},
			{Name: "concierge", Tier: "operator", Roles: []string{"router"}},
		},
	}
	m := FromConfig(cfg)

	hasConfig := false
	hasContracts := false
	for _, acl := range m.ACLs {
		if acl.User == "a-sysadmin" && acl.Path == "/srv/conos/config/agents" {
			hasConfig = true
		}
		if acl.User == "a-sysadmin" && acl.Path == "/srv/conos/contracts" {
			hasContracts = true
		}
	}
	if !hasConfig {
		t.Error("sysadmin role should have config/agents write ACL")
	}
	if !hasContracts {
		t.Error("sysadmin role should have contracts write ACL")
	}

	for _, acl := range m.ACLs {
		if acl.User == "a-concierge" && acl.Path == "/srv/conos/config/agents" {
			t.Error("non-sysadmin should not have config/agents ACL")
		}
	}
}

func TestFromConfig_ACLs_CannotTaskSelf(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "ceo", Tier: "officer"},
		},
	}
	m := FromConfig(cfg)

	for _, acl := range m.ACLs {
		if acl.User == "a-ceo" && strings.Contains(acl.Path, "/ceo/") {
			t.Error("agent should not have ACL to task itself")
		}
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

func TestFromConfig_SetupCommands_SSHKeys(t *testing.T) {
	cfg := &config.Config{
		Infra: config.InfraConfig{
			SSHAuthorizedKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5 user@host"},
		},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
		},
	}
	m := FromConfig(cfg)

	hasSSH := false
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "authorized_keys") {
			hasSSH = true
		}
	}
	if !hasSSH {
		t.Error("expected SSH key setup command")
	}
}

func TestFromConfig_SetupCommands_Sudoers(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
		},
	}
	m := FromConfig(cfg)

	hasSudoers := false
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "sudoers") {
			hasSudoers = true
		}
	}
	if !hasSudoers {
		t.Error("expected sudoers setup command")
	}
}

func TestFromConfig_SetupCommands_ContractsCopy(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
		},
	}
	m := FromConfig(cfg)

	hasContracts := false
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "contracts") {
			hasContracts = true
		}
	}
	if !hasContracts {
		t.Error("expected contracts copy setup command")
	}
}

func TestFromConfig_SetupCommands_GitInit(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
		},
	}
	m := FromConfig(cfg)

	hasGit := false
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "git init") {
			hasGit = true
		}
	}
	if !hasGit {
		t.Error("expected git init setup command")
	}
}

func TestFromConfig_SetupCommands_SSHKeyRejectsSpecialChars(t *testing.T) {
	cfg := &config.Config{
		Infra: config.InfraConfig{
			SSHAuthorizedKeys: []string{"ssh-ed25519 AAAA user@host", "bad'key"},
		},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
		},
	}
	m := FromConfig(cfg)

	hasValidKey := false
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "ssh-ed25519 AAAA") {
			hasValidKey = true
		}
	}
	if !hasValidKey {
		t.Error("expected valid SSH key in setup commands")
	}
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "bad'key") {
			t.Error("special char key should be rejected")
		}
	}
}

func TestProvisionFromManifest_SetupCommands(t *testing.T) {
	m := Manifest{
		SetupCommands: []SetupCommand{
			{Description: "test", Cmd: "echo hello"},
		},
	}
	cmds := ProvisionFromManifest(m)

	hasEcho := false
	for _, c := range cmds {
		if c == "echo hello" {
			hasEcho = true
		}
	}
	if !hasEcho {
		t.Error("expected setup command in provision output")
	}
}

func TestFromConfig_SetupCommands_Tailscale(t *testing.T) {
	cfg := &config.Config{
		Infra:  config.InfraConfig{TailscaleHostname: "myhost"},
		Agents: []config.AgentConfig{{Name: "concierge", Tier: "operator", Mode: "on-demand"}},
	}
	m := FromConfig(cfg)
	hasTailscale := false
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "tailscale") {
			hasTailscale = true
		}
	}
	if !hasTailscale {
		t.Error("expected Tailscale setup command when hostname configured")
	}
}

func TestFromConfig_SetupCommands_NoTailscale(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{{Name: "concierge", Tier: "operator", Mode: "on-demand"}},
	}
	m := FromConfig(cfg)
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "tailscale up") {
			t.Error("should not have Tailscale command when hostname empty")
		}
	}
}

func TestFromConfig_SetupCommands_Dashboard(t *testing.T) {
	cfg := &config.Config{
		Dashboard: config.DashboardConfig{Enabled: true, Port: 8080, Bind: "127.0.0.1"},
		Agents:    []config.AgentConfig{{Name: "concierge", Tier: "operator", Mode: "on-demand"}},
	}
	m := FromConfig(cfg)
	hasNginx := false
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "nginx") {
			hasNginx = true
		}
	}
	if !hasNginx {
		t.Error("expected nginx setup command when dashboard enabled")
	}
}

func TestFromConfig_SetupCommands_DashboardDisabled(t *testing.T) {
	cfg := &config.Config{
		Dashboard: config.DashboardConfig{Enabled: false},
		Agents:    []config.AgentConfig{{Name: "concierge", Tier: "operator", Mode: "on-demand"}},
	}
	m := FromConfig(cfg)
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "enable --now nginx") {
			t.Error("should not enable nginx when dashboard disabled")
		}
	}
}

func TestFromConfig_SetupCommands_SystemdReload(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{{Name: "concierge", Tier: "operator", Mode: "on-demand"}},
	}
	m := FromConfig(cfg)
	hasDaemonReload := false
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "daemon-reload") {
			hasDaemonReload = true
		}
	}
	if !hasDaemonReload {
		t.Error("expected systemctl daemon-reload setup command")
	}
}

func TestFromConfig_SetupCommands_UnitEnable(t *testing.T) {
	cfg := &config.Config{
		Agents:    []config.AgentConfig{{Name: "concierge", Tier: "operator", Mode: "on-demand"}},
		Contracts: config.ContractsConfig{System: config.SystemContracts{HealthcheckInterval: "60s"}},
	}
	m := FromConfig(cfg)
	hasPathEnable := false
	hasHCEnable := false
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "conos-concierge.path") {
			hasPathEnable = true
		}
		if strings.Contains(sc.Cmd, "conos-healthcheck.timer") {
			hasHCEnable = true
		}
	}
	if !hasPathEnable {
		t.Error("expected concierge path unit enable")
	}
	if !hasHCEnable {
		t.Error("expected healthcheck timer enable")
	}
}

func TestFromConfig_SetupCommands_SkillDeploy(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{{Name: "concierge", Tier: "operator", Mode: "on-demand", Roles: []string{"router"}}},
	}
	m := FromConfig(cfg)
	hasSkills := false
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "workspace/skills") {
			hasSkills = true
		}
	}
	if !hasSkills {
		t.Error("expected skill deployment setup commands")
	}
}

func TestFromConfig_SetupCommands_ContinuousMode(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{{Name: "worker", Tier: "worker", Mode: "continuous"}},
	}
	m := FromConfig(cfg)
	hasSvcEnable := false
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "conos-worker.service") && strings.Contains(sc.Cmd, "enable") {
			hasSvcEnable = true
		}
	}
	if !hasSvcEnable {
		t.Error("expected continuous mode agent to enable .service unit")
	}
}

func TestFromConfig_SetupCommands_CronMode(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{{Name: "reporter", Tier: "worker", Mode: "cron"}},
	}
	m := FromConfig(cfg)
	hasTimerEnable := false
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "conos-reporter.timer") && strings.Contains(sc.Cmd, "enable") {
			hasTimerEnable = true
		}
	}
	if !hasTimerEnable {
		t.Error("expected cron mode agent to enable .timer unit")
	}
}

func TestFromConfig_SetupCommands_OuterInboxEnable(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{{Name: "concierge", Tier: "operator", Mode: "on-demand"}},
	}
	m := FromConfig(cfg)
	hasOuterInbox := false
	for _, sc := range m.SetupCommands {
		if strings.Contains(sc.Cmd, "conos-outer-inbox.path") && strings.Contains(sc.Cmd, "enable") {
			hasOuterInbox = true
		}
	}
	if !hasOuterInbox {
		t.Error("expected outer inbox path unit enable")
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
	if len(m2.SetupCommands) != len(m.SetupCommands) {
		t.Errorf("setup_commands: expected %d, got %d", len(m.SetupCommands), len(m2.SetupCommands))
	}
}
