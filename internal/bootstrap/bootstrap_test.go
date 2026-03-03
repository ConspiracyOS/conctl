package bootstrap

import (
	"strings"
	"testing"

	"github.com/ConspiracyOS/conctl/internal/config"
)

func TestProvisionCommands(t *testing.T) {
	cfg := &config.Config{
		System: config.SystemConfig{Name: "test"},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
			{Name: "sysadmin", Tier: "operator", Mode: "on-demand"},
		},
	}

	cmds := PlanProvision(cfg)

	// Should create agents group
	found := false
	for _, c := range cmds {
		if strings.Contains(c, "groupadd") && strings.Contains(c, "agents") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected groupadd agents command")
	}

	// Should create user a-concierge
	found = false
	for _, c := range cmds {
		if strings.Contains(c, "useradd") && strings.Contains(c, "a-concierge") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected useradd a-concierge command")
	}

	// Should create /srv/conos/agents/concierge/inbox/
	found = false
	for _, c := range cmds {
		if strings.Contains(c, "/srv/conos/agents/concierge/inbox") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected inbox directory creation for concierge")
	}
}

func TestProvisionACLs(t *testing.T) {
	cfg := &config.Config{
		System: config.SystemConfig{Name: "test"},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
			{Name: "sysadmin", Tier: "operator", Mode: "on-demand"},
		},
	}

	cmds := PlanProvision(cfg)

	// Concierge should be able to write to sysadmin's inbox
	found := false
	for _, c := range cmds {
		if strings.Contains(c, "setfacl") && strings.Contains(c, "a-concierge") && strings.Contains(c, "sysadmin/inbox") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ACL granting concierge write to sysadmin inbox")
	}
}

func TestProvisionContractInstallation(t *testing.T) {
	cfg := &config.Config{
		System: config.SystemConfig{Name: "test"},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
		},
	}

	cmds := PlanProvision(cfg)

	found := false
	for _, c := range cmds {
		if strings.Contains(c, "cp /etc/conos/contracts/*.yaml /srv/conos/contracts/") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected contract file installation command")
	}
}

func TestGenerateHealthcheckUnits(t *testing.T) {
	units := GenerateHealthcheckUnits("60s")

	svc, ok := units["conos-healthcheck.service"]
	if !ok {
		t.Fatal("missing conos-healthcheck.service")
	}
	if !strings.Contains(svc, "conctl healthcheck") {
		t.Error("service should run 'conctl healthcheck'")
	}
	if !strings.Contains(svc, "Type=oneshot") {
		t.Error("service should be oneshot")
	}
	// Should NOT have a User= line (runs as root)
	if strings.Contains(svc, "User=") {
		t.Error("healthcheck service should not have User= (runs as root)")
	}

	timer, ok := units["conos-healthcheck.timer"]
	if !ok {
		t.Fatal("missing conos-healthcheck.timer")
	}
	if !strings.Contains(timer, "OnUnitActiveSec=60s") {
		t.Error("timer should use the provided interval")
	}
	if !strings.Contains(timer, "OnBootSec=30s") {
		t.Error("timer should have OnBootSec delay")
	}
}

func TestProvisionTrustedGroup(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
		},
	}
	cmds := PlanProvision(cfg)
	found := false
	for _, c := range cmds {
		if c == "groupadd -f trusted" {
			found = true
			break
		}
	}
	if !found {
		t.Error("PlanProvision should create trusted group")
	}
}

func TestProvisionSudoersFromProfile(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
			{Name: "sysadmin", Tier: "operator", Roles: []string{"sysadmin"}},
		},
	}
	cmds := PlanProvision(cfg)

	// Should copy sudoers from profile, not hardcode them
	foundCopy := false
	foundValidate := false
	for _, c := range cmds {
		if strings.Contains(c, "cp /etc/conos/sudoers.d/") && strings.Contains(c, "/etc/sudoers.d/") {
			foundCopy = true
		}
		if strings.Contains(c, "visudo -c") {
			foundValidate = true
		}
	}
	if !foundCopy {
		t.Error("expected sudoers copy from /etc/conos/sudoers.d/ to /etc/sudoers.d/")
	}
	if !foundValidate {
		t.Error("expected visudo -c validation after sudoers install")
	}

	// Should NOT contain hardcoded CONSPIRACY_OPS
	for _, c := range cmds {
		if strings.Contains(c, "Cmnd_Alias CONSPIRACY_OPS") {
			t.Error("sudoers should come from profile files, not be hardcoded in Go")
		}
	}
}

func TestDashboardDisabledStopsNginx(t *testing.T) {
	cfg := &config.Config{
		Dashboard: config.DashboardConfig{Enabled: false, Port: 8080, Bind: "0.0.0.0"},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
		},
	}
	cmds := PlanProvision(cfg)

	foundDisable := false
	for _, c := range cmds {
		if strings.Contains(c, "systemctl disable") && strings.Contains(c, "nginx") {
			foundDisable = true
		}
	}
	if !foundDisable {
		t.Error("expected nginx disable when dashboard is disabled")
	}

	// Should NOT enable nginx
	for _, c := range cmds {
		if strings.Contains(c, "systemctl enable") && strings.Contains(c, "nginx") {
			t.Error("should not enable nginx when dashboard is disabled")
		}
	}
}

func TestProvisionOfficerTier(t *testing.T) {
	cfg := &config.Config{
		Agents: []config.AgentConfig{
			{Name: "ceo", Tier: "officer", Mode: "on-demand"},
		},
	}
	cmds := PlanProvision(cfg)

	found := false
	for _, c := range cmds {
		if strings.Contains(c, "useradd") && strings.Contains(c, "a-ceo") && strings.Contains(c, "officers") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected useradd for officer tier to include officers group")
	}
}

func TestProvisionSSHAuthorizedKeys(t *testing.T) {
	cfg := &config.Config{
		Infra: config.InfraConfig{
			SSHAuthorizedKeys: []string{"ssh-rsa AAAA user@host", "ssh-rsa BBBB admin@host"},
		},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
		},
	}
	cmds := PlanProvision(cfg)

	foundSSHDir := false
	keyCount := 0
	for _, c := range cmds {
		if strings.Contains(c, "install -d -m 700 /root/.ssh") {
			foundSSHDir = true
		}
		if strings.Contains(c, "authorized_keys") && strings.Contains(c, "ssh-rsa") {
			keyCount++
		}
	}
	if !foundSSHDir {
		t.Error("expected /root/.ssh directory creation")
	}
	if keyCount != 2 {
		t.Errorf("expected 2 authorized_keys entries, got %d", keyCount)
	}
}

func TestProvisionTailscaleWithLoginServer(t *testing.T) {
	cfg := &config.Config{
		Infra: config.InfraConfig{
			TailscaleHostname:    "myhost",
			TailscaleLoginServer: "http://192.168.1.1:8080",
		},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
		},
	}
	cmds := PlanProvision(cfg)

	found := false
	for _, c := range cmds {
		if strings.Contains(c, "tailscale") && strings.Contains(c, "myhost") {
			if strings.Contains(c, "--login-server=http://192.168.1.1:8080") {
				found = true
			}
			break
		}
	}
	if !found {
		t.Error("expected tailscale command with --login-server flag")
	}
}

func TestProvisionTailscaleWithoutLoginServer(t *testing.T) {
	cfg := &config.Config{
		Infra: config.InfraConfig{
			TailscaleHostname: "myhost",
		},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
		},
	}
	cmds := PlanProvision(cfg)

	found := false
	for _, c := range cmds {
		if strings.Contains(c, "tailscale") && strings.Contains(c, "myhost") {
			found = true
			if strings.Contains(c, "--login-server") {
				t.Error("tailscale command should not include --login-server when not configured")
			}
			break
		}
	}
	if !found {
		t.Error("expected tailscale command in provision output")
	}
}

func TestDashboardEnabledStartsNginx(t *testing.T) {
	cfg := &config.Config{
		Dashboard: config.DashboardConfig{Enabled: true, Port: 8080, Bind: "0.0.0.0"},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
		},
	}
	cmds := PlanProvision(cfg)

	found := false
	for _, c := range cmds {
		if strings.Contains(c, "systemctl enable") && strings.Contains(c, "nginx") {
			found = true
		}
	}
	if !found {
		t.Error("expected nginx enable when dashboard is enabled")
	}
	for _, c := range cmds {
		if strings.Contains(c, "systemctl disable") && strings.Contains(c, "nginx") {
			t.Error("should not disable nginx when dashboard is enabled")
		}
	}
}

func TestDashboardEnabledWithTailscale(t *testing.T) {
	cfg := &config.Config{
		Infra: config.InfraConfig{
			TailscaleHostname: "myhost",
		},
		Dashboard: config.DashboardConfig{Enabled: true, Port: 8080, Bind: "0.0.0.0"},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
		},
	}
	cmds := PlanProvision(cfg)

	found := false
	for _, c := range cmds {
		if strings.Contains(c, "TSIP=$(tailscale ip") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected nginx bind override for Tailscale IP when dashboard+Tailscale both configured")
	}
}

func TestOuterInboxWatcher(t *testing.T) {
	cfg := &config.Config{
		System: config.SystemConfig{Name: "test"},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator", Mode: "on-demand"},
		},
	}

	cmds := PlanProvision(cfg)

	// Should create conos-outer-inbox.path unit
	foundPath := false
	for _, c := range cmds {
		if strings.Contains(c, "conos-outer-inbox.path") && strings.Contains(c, "PathChanged=/srv/conos/inbox") {
			foundPath = true
			break
		}
	}
	if !foundPath {
		t.Error("expected outer inbox .path unit watching /srv/conos/inbox")
	}

	// Should create conos-outer-inbox.service unit
	foundSvc := false
	for _, c := range cmds {
		if strings.Contains(c, "conos-outer-inbox.service") && strings.Contains(c, "conctl route-inbox") {
			foundSvc = true
			break
		}
	}
	if !foundSvc {
		t.Error("expected outer inbox .service unit running conctl route-inbox")
	}
}
