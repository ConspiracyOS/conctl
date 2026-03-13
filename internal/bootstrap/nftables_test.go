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

	// Should have per-agent allowlist
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
	if !strings.Contains(rules, `meta skuid "a-concierge" tcp dport 11434 accept`) {
		t.Error("expected concierge single port rule")
	}

	// Sysadmin: multiple ports with set syntax
	if !strings.Contains(rules, `meta skuid "a-sysadmin" tcp dport { 80, 443 } accept`) {
		t.Error("expected sysadmin multi-port rule")
	}

	// Researcher: no allowlist rule (fully isolated)
	if strings.Contains(rules, `meta skuid "a-researcher" tcp dport`) {
		t.Error("researcher should have no port allowlist")
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

	// Should NOT have an allow rule for researcher
	if strings.Contains(rules, `meta skuid "a-researcher" tcp dport`) {
		t.Error("agent with no allowed ports should not have a port allow rule")
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
	allowIdx := strings.Index(rules, "tcp dport")
	denyIdx := strings.Index(rules, "counter drop")

	if loIdx == -1 || estIdx == -1 || allowIdx == -1 || denyIdx == -1 {
		t.Fatal("missing expected rule sections")
	}
	if loIdx > estIdx {
		t.Error("loopback must come before established")
	}
	if estIdx > allowIdx {
		t.Error("established must come before per-agent allows")
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
