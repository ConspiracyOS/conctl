package assembler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssembleBasic(t *testing.T) {
	root := t.TempDir()

	// base/AGENTS.md
	basedir := filepath.Join(root, "base")
	os.MkdirAll(basedir, 0755)
	os.WriteFile(filepath.Join(basedir, "AGENTS.md"), []byte("# Base\nYou are an agent.\n"), 0644)

	// roles/operator/AGENTS.md
	roledir := filepath.Join(root, "roles", "operator")
	os.MkdirAll(roledir, 0755)
	os.WriteFile(filepath.Join(roledir, "AGENTS.md"), []byte("# Operator\nYou operate systems.\n"), 0644)

	// agents/concierge/AGENTS.md
	agentdir := filepath.Join(root, "agents", "concierge")
	os.MkdirAll(agentdir, 0755)
	os.WriteFile(filepath.Join(agentdir, "AGENTS.md"), []byte("# Concierge\nYou route tasks.\n"), 0644)

	layers := Layers{
		OuterRoot:          root,
		InnerRoot:          "",
		Roles:              []string{"operator"},
		Groups:             []string{},
		Scopes:             []string{},
		AgentName:          "concierge",
		InlineInstructions: "Inline instructions here.",
	}

	result, err := Assemble(layers)
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	if !strings.Contains(result, "# Base") {
		t.Error("missing base layer")
	}
	if !strings.Contains(result, "# Operator") {
		t.Error("missing role layer")
	}
	if !strings.Contains(result, "# Concierge") {
		t.Error("missing agent layer")
	}
	if !strings.Contains(result, "Inline instructions") {
		t.Error("missing inline instructions")
	}
}

func TestAssembleInnerOverlay_WithGroupsRolesScopes(t *testing.T) {
	root := t.TempDir()
	inner := t.TempDir()

	// Outer base (required so parts is non-empty)
	os.MkdirAll(filepath.Join(root, "base"), 0755)
	os.WriteFile(filepath.Join(root, "base", "AGENTS.md"), []byte("Outer base.\n"), 0644)

	// Inner group overlay — covers the InnerRoot != "" branch inside the groups loop
	os.MkdirAll(filepath.Join(inner, "groups", "ops"), 0755)
	os.WriteFile(filepath.Join(inner, "groups", "ops", "AGENTS.md"), []byte("Inner group ops.\n"), 0644)

	// Inner role overlay — covers the InnerRoot != "" branch inside the roles loop
	os.MkdirAll(filepath.Join(inner, "roles", "admin"), 0755)
	os.WriteFile(filepath.Join(inner, "roles", "admin", "AGENTS.md"), []byte("Inner role admin.\n"), 0644)

	// Inner scope overlay — covers the InnerRoot != "" branch inside the scopes loop
	os.MkdirAll(filepath.Join(inner, "scopes", "prod"), 0755)
	os.WriteFile(filepath.Join(inner, "scopes", "prod", "AGENTS.md"), []byte("Inner scope prod.\n"), 0644)

	layers := Layers{
		OuterRoot: root,
		InnerRoot: inner,
		Groups:    []string{"ops"},
		Roles:     []string{"admin"},
		Scopes:    []string{"prod"},
		AgentName: "test",
	}

	result, err := Assemble(layers)
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	if !strings.Contains(result, "Inner group ops.") {
		t.Error("missing inner group overlay")
	}
	if !strings.Contains(result, "Inner role admin.") {
		t.Error("missing inner role overlay")
	}
	if !strings.Contains(result, "Inner scope prod.") {
		t.Error("missing inner scope overlay")
	}
}

func TestAssembleEmptyResult(t *testing.T) {
	// Empty temp dir — no AGENTS.md anywhere — should return an error.
	layers := Layers{
		OuterRoot: t.TempDir(),
		AgentName: "nobody",
	}
	_, err := Assemble(layers)
	if err == nil {
		t.Error("expected error when no AGENTS.md content found")
	}
}

func TestAssembleInnerOverlay(t *testing.T) {
	root := t.TempDir()
	inner := t.TempDir()

	// Outer base
	os.MkdirAll(filepath.Join(root, "base"), 0755)
	os.WriteFile(filepath.Join(root, "base", "AGENTS.md"), []byte("Outer base.\n"), 0644)

	// Inner base (should be appended after outer base)
	os.MkdirAll(filepath.Join(inner, "base"), 0755)
	os.WriteFile(filepath.Join(inner, "base", "AGENTS.md"), []byte("Inner base override.\n"), 0644)

	layers := Layers{
		OuterRoot: root,
		InnerRoot: inner,
		AgentName: "test",
	}

	result, err := Assemble(layers)
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	if !strings.Contains(result, "Outer base.") {
		t.Error("missing outer base")
	}
	if !strings.Contains(result, "Inner base override.") {
		t.Error("missing inner base overlay")
	}
}
