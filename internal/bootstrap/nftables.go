package bootstrap

import (
	"fmt"
	"strings"

	"github.com/ConspiracyOS/conctl/internal/config"
)

// GenerateNftRules builds an nftables ruleset for per-agent outbound filtering.
// When outbound_filter is "strict", agents are denied all outbound traffic except
// explicitly allowed ports. When "permissive" or empty, no rules are generated.
//
// Rules use meta skuid to match packets by socket owner UID — kernel-enforced,
// no application-layer proxy needed.
func GenerateNftRules(cfg *config.Config) string {
	if cfg.Network.OutboundFilter != "strict" {
		return ""
	}
	if len(cfg.Agents) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString("table inet conos {\n")

	// Per-agent allowlist rules
	var agentRules []string
	for _, a := range cfg.Agents {
		user := "a-" + a.Name
		if len(a.AllowedPorts) == 0 {
			// No ports allowed — agent is network-isolated
			continue
		}
		ports := formatPorts(a.AllowedPorts)
		agentRules = append(agentRules,
			fmt.Sprintf("        meta skuid \"%s\" tcp dport %s accept", user, ports))
	}

	b.WriteString("    chain output {\n")
	b.WriteString("        type filter hook output priority 0; policy accept;\n\n")

	// Allow loopback (always)
	b.WriteString("        # Loopback is always allowed\n")
	b.WriteString("        oifname \"lo\" accept\n\n")

	// Allow established connections (return traffic for allowed outbound)
	b.WriteString("        # Allow return traffic for established connections\n")
	b.WriteString("        ct state established,related accept\n\n")

	// Per-agent port allowlists
	if len(agentRules) > 0 {
		b.WriteString("        # Per-agent outbound allowlists\n")
		for _, rule := range agentRules {
			b.WriteString(rule + "\n")
		}
		b.WriteString("\n")
	}

	// Default deny for all agent users
	b.WriteString("        # Deny all other outbound from agent users\n")
	for _, a := range cfg.Agents {
		user := "a-" + a.Name
		b.WriteString(fmt.Sprintf("        meta skuid \"%s\" counter drop\n", user))
	}

	b.WriteString("    }\n")
	b.WriteString("}\n")

	return b.String()
}

// formatPorts formats a port list for nftables syntax.
// Single port: "443", multiple ports: "{ 80, 443 }".
func formatPorts(ports []int) string {
	if len(ports) == 1 {
		return fmt.Sprintf("%d", ports[0])
	}
	parts := make([]string, len(ports))
	for i, p := range ports {
		parts[i] = fmt.Sprintf("%d", p)
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}
