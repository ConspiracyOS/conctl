package bootstrap

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ConspiracyOS/conctl/internal/config"
)

func TestGenerateNftRules_Permissive(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{OutboundFilter: "permissive"},
		Agents:  []config.AgentConfig{{Name: "concierge"}},
	}
	if rules := GenerateNftRules(cfg); rules != "" {
		t.Error("permissive mode should generate no rules")
	}
}

func TestGenerateNftRules_Empty(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{OutboundFilter: ""},
	}
	if rules := GenerateNftRules(cfg); rules != "" {
		t.Error("empty outbound_filter should generate no rules")
	}
}

func TestGenerateNftRules_StrictNoAgents(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{OutboundFilter: "strict"},
	}
	if rules := GenerateNftRules(cfg); rules != "" {
		t.Error("strict with no agents should generate no rules")
	}
}

func TestGenerateNftRules_FlushBeforeReload(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{OutboundFilter: "strict"},
		Agents:  []config.AgentConfig{{Name: "concierge"}},
	}
	rules := GenerateNftRules(cfg)
	// Must create then delete the table before re-creating, for idempotent reloads
	if !strings.Contains(rules, "table inet conos {}\ndelete table inet conos\n") {
		t.Error("expected create-then-delete prefix for idempotent reload")
	}
}

func TestGenerateNftRules_StrictSingleAgent(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{OutboundFilter: "strict"},
		Agents: []config.AgentConfig{
			{Name: "concierge", AllowedPorts: []int{11434}},
		},
	}
	rules := GenerateNftRules(cfg)

	// Should have the table and chain
	if !strings.Contains(rules, "table inet conos") {
		t.Error("expected table inet conos")
	}
	if !strings.Contains(rules, "chain output") {
		t.Error("expected chain output")
	}

	// Should have loopback and established
	if !strings.Contains(rules, `oifname "lo" accept`) {
		t.Error("expected loopback accept")
	}
	if !strings.Contains(rules, "ct state established,related accept") {
		t.Error("expected established accept")
	}

	// Should have per-agent DNS allowlist scoped to local resolver + drop for external DNS
	if !strings.Contains(rules, `meta skuid "a-concierge" udp dport 53 ip daddr 127.0.0.53 accept`) {
		t.Error("expected concierge udp dns scoped to 127.0.0.53")
	}
	if !strings.Contains(rules, `meta skuid "a-concierge" udp dport 53 drop`) {
		t.Error("expected concierge udp dns external drop")
	}
	if !strings.Contains(rules, `meta skuid "a-concierge" tcp dport 53 ip daddr 127.0.0.53 accept`) {
		t.Error("expected concierge tcp dns scoped to 127.0.0.53")
	}
	if !strings.Contains(rules, `meta skuid "a-concierge" tcp dport 53 drop`) {
		t.Error("expected concierge tcp dns external drop")
	}
	if !strings.Contains(rules, `meta skuid "a-concierge" tcp dport 11434 accept`) {
		t.Error("expected concierge port 11434 allowlist")
	}

	// Should have default deny
	if !strings.Contains(rules, `meta skuid "a-concierge" counter drop`) {
		t.Error("expected concierge default deny")
	}
}

func TestGenerateNftRules_StrictMultipleAgents(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{OutboundFilter: "strict"},
		Agents: []config.AgentConfig{
			{Name: "concierge", AllowedPorts: []int{11434}},
			{Name: "sysadmin", AllowedPorts: []int{80, 443}},
			{Name: "researcher"}, // no ports = fully isolated
		},
	}
	rules := GenerateNftRules(cfg)

	// Concierge: single port
	if !strings.Contains(rules, `meta skuid "a-concierge" udp dport 53 ip daddr 127.0.0.53 accept`) {
		t.Error("expected concierge udp dns scoped to 127.0.0.53")
	}
	if !strings.Contains(rules, `meta skuid "a-concierge" udp dport 53 drop`) {
		t.Error("expected concierge udp dns external drop")
	}
	if !strings.Contains(rules, `meta skuid "a-concierge" tcp dport 53 ip daddr 127.0.0.53 accept`) {
		t.Error("expected concierge tcp dns scoped to 127.0.0.53")
	}
	if !strings.Contains(rules, `meta skuid "a-concierge" tcp dport 53 drop`) {
		t.Error("expected concierge tcp dns external drop")
	}
	if !strings.Contains(rules, `meta skuid "a-concierge" tcp dport 11434 accept`) {
		t.Error("expected concierge single port rule")
	}

	// Sysadmin: multiple ports with set syntax
	if !strings.Contains(rules, `meta skuid "a-sysadmin" udp dport 53 ip daddr 127.0.0.53 accept`) {
		t.Error("expected sysadmin udp dns scoped to 127.0.0.53")
	}
	if !strings.Contains(rules, `meta skuid "a-sysadmin" udp dport 53 drop`) {
		t.Error("expected sysadmin udp dns external drop")
	}
	if !strings.Contains(rules, `meta skuid "a-sysadmin" tcp dport 53 ip daddr 127.0.0.53 accept`) {
		t.Error("expected sysadmin tcp dns scoped to 127.0.0.53")
	}
	if !strings.Contains(rules, `meta skuid "a-sysadmin" tcp dport 53 drop`) {
		t.Error("expected sysadmin tcp dns external drop")
	}
	if !strings.Contains(rules, `meta skuid "a-sysadmin" tcp dport { 80, 443 } accept`) {
		t.Error("expected sysadmin multi-port rule")
	}

	// Researcher: only DNS, no additional allowlist rules.
	if !strings.Contains(rules, `meta skuid "a-researcher" udp dport 53 ip daddr 127.0.0.53 accept`) {
		t.Error("expected researcher udp dns scoped to 127.0.0.53")
	}
	if !strings.Contains(rules, `meta skuid "a-researcher" udp dport 53 drop`) {
		t.Error("expected researcher udp dns external drop")
	}
	if !strings.Contains(rules, `meta skuid "a-researcher" tcp dport 53 ip daddr 127.0.0.53 accept`) {
		t.Error("expected researcher tcp dns scoped to 127.0.0.53")
	}
	if !strings.Contains(rules, `meta skuid "a-researcher" tcp dport 53 drop`) {
		t.Error("expected researcher tcp dns external drop")
	}
	if strings.Contains(rules, `meta skuid "a-researcher" tcp dport {`) || strings.Contains(rules, `meta skuid "a-researcher" tcp dport 80 accept`) || strings.Contains(rules, `meta skuid "a-researcher" tcp dport 443 accept`) {
		t.Error("researcher should not have non-dns port allow rules")
	}

	// All three should have deny rules
	for _, agent := range []string{"a-concierge", "a-sysadmin", "a-researcher"} {
		if !strings.Contains(rules, fmt.Sprintf(`meta skuid "%s" counter drop`, agent)) {
			t.Errorf("expected deny rule for %s", agent)
		}
	}
}

func TestGenerateNftRules_StrictAgentNoPortsGetsDenyOnly(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{OutboundFilter: "strict"},
		Agents: []config.AgentConfig{
			{Name: "researcher"}, // no AllowedPorts
		},
	}
	rules := GenerateNftRules(cfg)

	// Should still get DNS allow rules scoped to local resolver
	if !strings.Contains(rules, `meta skuid "a-researcher" udp dport 53 ip daddr 127.0.0.53 accept`) {
		t.Error("agent with no allowed ports should still have udp dns allow rule (scoped)")
	}
	if !strings.Contains(rules, `meta skuid "a-researcher" udp dport 53 drop`) {
		t.Error("agent with no allowed ports should have udp dns external drop")
	}
	if !strings.Contains(rules, `meta skuid "a-researcher" tcp dport 53 ip daddr 127.0.0.53 accept`) {
		t.Error("agent with no allowed ports should still have tcp dns allow rule (scoped)")
	}
	if !strings.Contains(rules, `meta skuid "a-researcher" tcp dport 53 drop`) {
		t.Error("agent with no allowed ports should have tcp dns external drop")
	}

	// Should still have a deny rule
	if !strings.Contains(rules, `meta skuid "a-researcher" counter drop`) {
		t.Error("agent with no allowed ports should still have a deny rule")
	}
}

func TestGenerateNftRules_RuleOrdering(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{OutboundFilter: "strict"},
		Agents: []config.AgentConfig{
			{Name: "concierge", AllowedPorts: []int{443}},
		},
	}
	rules := GenerateNftRules(cfg)

	loIdx := strings.Index(rules, `oifname "lo"`)
	estIdx := strings.Index(rules, "ct state established")
	dnsIdx := strings.Index(rules, "udp dport 53")
	allowIdx := strings.LastIndex(rules, "tcp dport")
	denyIdx := strings.Index(rules, "counter drop")

	if loIdx == -1 || estIdx == -1 || dnsIdx == -1 || allowIdx == -1 || denyIdx == -1 {
		t.Fatal("missing expected rule sections")
	}
	if loIdx > estIdx {
		t.Error("loopback must come before established")
	}
	if estIdx > dnsIdx {
		t.Error("established must come before dns allows")
	}
	if dnsIdx > allowIdx {
		t.Error("dns allows must come before additional per-agent allows")
	}
	if allowIdx > denyIdx {
		t.Error("per-agent allows must come before deny")
	}
}

func TestGenerateNftRules_PolicyAccept(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{OutboundFilter: "strict"},
		Agents:  []config.AgentConfig{{Name: "test"}},
	}
	rules := GenerateNftRules(cfg)

	// Default policy must be accept (non-agent traffic should pass)
	if !strings.Contains(rules, "policy accept") {
		t.Error("chain policy must be accept (only agent UIDs are filtered)")
	}
}

func TestGenerateNftRules_DNSScoped(t *testing.T) {
	cfg := &config.Config{
		Network: config.NetworkConfig{OutboundFilter: "strict"},
		Agents:  []config.AgentConfig{{Name: "concierge", AllowedPorts: []int{443}}},
	}
	rules := GenerateNftRules(cfg)

	// DNS must be scoped to 127.0.0.53 — external DNS tunneling must be blocked
	if !strings.Contains(rules, `ip daddr 127.0.0.53 accept`) {
		t.Error("DNS must be scoped to 127.0.0.53")
	}
	if !strings.Contains(rules, `udp dport 53 drop`) {
		t.Error("external UDP DNS must be dropped")
	}
	if !strings.Contains(rules, `tcp dport 53 drop`) {
		t.Error("external TCP DNS must be dropped")
	}
	// The old blanket accept must not exist
	if strings.Contains(rules, "udp dport 53 accept\n") || strings.Contains(rules, "tcp dport 53 accept\n") {
		t.Error("blanket DNS accept must not be present — scoped accept only")
	}
}

func TestFormatPorts_Single(t *testing.T) {
	if got := formatPorts([]int{443}); got != "443" {
		t.Errorf("expected 443, got %s", got)
	}
}

func TestFormatPorts_Multiple(t *testing.T) {
	got := formatPorts([]int{80, 443, 8080})
	if got != "{ 80, 443, 8080 }" {
		t.Errorf("expected { 80, 443, 8080 }, got %s", got)
	}
}
