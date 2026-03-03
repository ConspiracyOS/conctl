package assembler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Layers struct {
	OuterRoot          string   // /etc/conos/
	InnerRoot          string   // /srv/conos/config/ (may be empty)
	Groups             []string
	Roles              []string
	Scopes             []string
	AgentName          string
	InlineInstructions string // from conos.toml [[agents]] instructions field
}

// Assemble concatenates AGENTS.md files from all layers in order:
// base → groups → roles → scopes → agent → inline instructions.
// For each layer, outer is read first, then inner overlays.
func Assemble(l Layers) (string, error) {
	var parts []string

	// Helper: read AGENTS.md from a path, skip if not found
	read := func(path string) {
		data, err := os.ReadFile(filepath.Join(path, "AGENTS.md"))
		if err == nil {
			parts = append(parts, strings.TrimSpace(string(data)))
		}
	}

	// Base layer
	read(filepath.Join(l.OuterRoot, "base"))
	if l.InnerRoot != "" {
		read(filepath.Join(l.InnerRoot, "base"))
	}

	// Group layers
	for _, g := range l.Groups {
		read(filepath.Join(l.OuterRoot, "groups", g))
		if l.InnerRoot != "" {
			read(filepath.Join(l.InnerRoot, "groups", g))
		}
	}

	// Role layers
	for _, r := range l.Roles {
		read(filepath.Join(l.OuterRoot, "roles", r))
		if l.InnerRoot != "" {
			read(filepath.Join(l.InnerRoot, "roles", r))
		}
	}

	// Scope layers
	for _, s := range l.Scopes {
		read(filepath.Join(l.OuterRoot, "scopes", s))
		if l.InnerRoot != "" {
			read(filepath.Join(l.InnerRoot, "scopes", s))
		}
	}

	// Agent-specific layer
	read(filepath.Join(l.OuterRoot, "agents", l.AgentName))
	if l.InnerRoot != "" {
		read(filepath.Join(l.InnerRoot, "agents", l.AgentName))
	}

	// Inline instructions from conos.toml
	if l.InlineInstructions != "" {
		parts = append(parts, strings.TrimSpace(l.InlineInstructions))
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("no AGENTS.md content found for agent %q", l.AgentName)
	}

	return strings.Join(parts, "\n\n---\n\n") + "\n", nil
}
