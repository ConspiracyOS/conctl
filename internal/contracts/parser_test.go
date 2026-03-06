package contracts

import (
	"os"
	"path/filepath"
	"testing"
)

const detectiveYAML = `id: CON-SYS-001
description: Disk free space check
type: detective
frequency: 60s
scope: system
checks:
  - name: disk_free
    command:
      run: "df /srv --output=pcent | tail -1 | tr -d ' %'"
      test: "[ $RESULT -ge 15 ]"
    on_fail:
      action: halt_agents
      escalate: sysadmin
      message: "disk below threshold"
`

const preventiveYAML = `id: CON-117
description: agent may only reach api.example.com
type: preventive
mechanism: nftables
agent: twitter-watcher
enforcement: |
  nft add rule inet filter output meta skuid a-twitter-watcher \
    ip daddr != { api.example.com } drop
`

const scriptCheckYAML = `id: CON-042
description: custom script check
type: detective
frequency: 300s
scope: agent:researcher
checks:
  - name: custom_check
    script:
      path: scripts/check.sh
      timeout: 30s
    on_fail:
      action: alert
      escalate: sysadmin
      message: "custom check failed"
`

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFile_Detective(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "CON-SYS-001.yaml", detectiveYAML)

	c, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "CON-SYS-001" {
		t.Errorf("ID = %q, want CON-SYS-001", c.ID)
	}
	if c.Type != "detective" {
		t.Errorf("Type = %q, want detective", c.Type)
	}
	if c.Scope != "system" {
		t.Errorf("Scope = %q, want system", c.Scope)
	}
	if len(c.Checks) != 1 {
		t.Fatalf("Checks len = %d, want 1", len(c.Checks))
	}
	ch := c.Checks[0]
	if ch.Name != "disk_free" {
		t.Errorf("Check.Name = %q, want disk_free", ch.Name)
	}
	if ch.Command == nil {
		t.Fatal("Check.Command is nil")
	}
	if ch.Command.Run == "" || ch.Command.Test == "" {
		t.Error("Command.Run or Command.Test is empty")
	}
	if ch.OnFail.Action != "halt_agents" {
		t.Errorf("OnFail.Action = %q, want halt_agents", ch.OnFail.Action)
	}
	if ch.OnFail.Escalate != "sysadmin" {
		t.Errorf("OnFail.Escalate = %q, want sysadmin", ch.OnFail.Escalate)
	}
}

func TestLoadFile_Preventive(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "CON-117.yaml", preventiveYAML)

	c, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Type != "preventive" {
		t.Errorf("Type = %q, want preventive", c.Type)
	}
	if c.Mechanism != "nftables" {
		t.Errorf("Mechanism = %q, want nftables", c.Mechanism)
	}
	if c.Agent != "twitter-watcher" {
		t.Errorf("Agent = %q, want twitter-watcher", c.Agent)
	}
	if c.Enforcement == "" {
		t.Error("Enforcement is empty")
	}
}

func TestLoadFile_ScriptCheck(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "CON-042.yaml", scriptCheckYAML)

	c, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Checks) != 1 {
		t.Fatalf("Checks len = %d, want 1", len(c.Checks))
	}
	ch := c.Checks[0]
	if ch.Script == nil {
		t.Fatal("Check.Script is nil")
	}
	if ch.Script.Path != "scripts/check.sh" {
		t.Errorf("Script.Path = %q", ch.Script.Path)
	}
	if ch.Script.Timeout != "30s" {
		t.Errorf("Script.Timeout = %q", ch.Script.Timeout)
	}
}

func TestLoadFile_ValidationError_NoChecks(t *testing.T) {
	yaml := `id: CON-BAD
description: missing checks
type: detective
scope: system
`
	dir := t.TempDir()
	path := writeTemp(t, dir, "bad.yaml", yaml)

	_, err := LoadFile(path)
	if err == nil {
		t.Error("expected validation error for detective with no checks")
	}
}

func TestLoadFile_MetaContractAllowsNoChecks(t *testing.T) {
	yaml := `id: CON-META-001
description: Meta contract
type: meta
scope: global
`
	dir := t.TempDir()
	path := writeTemp(t, dir, "meta.yaml", yaml)

	c, err := LoadFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Type != "meta" {
		t.Fatalf("type = %q, want meta", c.Type)
	}
}

func TestLoadFile_TaskContract(t *testing.T) {
	yaml := `id: CON-TASK-001
description: Task contract
type: task
scope: global
checks:
  - name: done_when_file_exists
    command:
      run: "test -f /tmp/done.flag"
      exit_code: 0
    on_fail: fail
`
	dir := t.TempDir()
	path := writeTemp(t, dir, "task.yaml", yaml)

	c, err := LoadFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Type != "task" {
		t.Fatalf("type = %q, want task", c.Type)
	}
}

func TestLoadFile_ValidationError_NoCommandOrScript(t *testing.T) {
	yaml := `id: CON-BAD2
description: check without command or script
type: detective
scope: system
checks:
  - name: empty_check
    on_fail:
      action: alert
      message: "bad"
`
	dir := t.TempDir()
	path := writeTemp(t, dir, "bad2.yaml", yaml)

	_, err := LoadFile(path)
	if err == nil {
		t.Error("expected validation error for check without command or script")
	}
}

func TestLoadFile_ValidationError_InvalidAction(t *testing.T) {
	yaml := `id: CON-BAD3
description: invalid action
type: detective
scope: system
checks:
  - name: check
    command:
      run: "echo 1"
      test: "[ 1 -eq 1 ]"
    on_fail:
      action: destroy_everything
      message: "bad"
`
	dir := t.TempDir()
	path := writeTemp(t, dir, "bad3.yaml", yaml)

	_, err := LoadFile(path)
	if err == nil {
		t.Error("expected validation error for invalid action")
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "CON-SYS-001.yaml", detectiveYAML)
	writeTemp(t, dir, "CON-117.yaml", preventiveYAML)
	writeTemp(t, dir, "readme.txt", "not a yaml")

	contracts, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 2 {
		t.Errorf("LoadDir returned %d contracts, want 2", len(contracts))
	}
}

func TestLoadDir_Empty(t *testing.T) {
	dir := t.TempDir()
	contracts, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 0 {
		t.Errorf("LoadDir returned %d contracts, want 0", len(contracts))
	}
}

func TestLoadDir_NonExistentDir(t *testing.T) {
	_, err := LoadDir("/nonexistent/path/contracts-xyz")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestLoadDir_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	// A yaml file that fails validation (missing ID) causes LoadDir to abort.
	writeTemp(t, dir, "bad.yaml", "type: detective\nscope: system\nchecks:\n  - name: c\n    command:\n      run: echo 1\n")

	_, err := LoadDir(dir)
	if err == nil {
		t.Error("expected error when a contract file in the directory fails validation")
	}
}

func TestLoadFile_NonExistent(t *testing.T) {
	_, err := LoadFile("/nonexistent/contract-xyz.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadFile_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "bad.yaml", "{ this is: [not valid yaml")
	_, err := LoadFile(path)
	if err == nil {
		t.Error("expected error for malformed YAML")
	}
}

func TestValidate_MissingID(t *testing.T) {
	yaml := `description: no id here
type: detective
scope: system
checks:
  - name: check
    command:
      run: "echo 1"
`
	dir := t.TempDir()
	_, err := LoadFile(writeTemp(t, dir, "noid.yaml", yaml))
	if err == nil {
		t.Error("expected error for contract without ID")
	}
}

func TestValidate_InvalidType(t *testing.T) {
	yaml := `id: CON-BAD4
type: superdetective
scope: system
checks:
  - name: check
    command:
      run: "echo 1"
`
	dir := t.TempDir()
	_, err := LoadFile(writeTemp(t, dir, "badtype.yaml", yaml))
	if err == nil {
		t.Error("expected error for invalid contract type")
	}
}

func TestValidate_BothCommandAndScript(t *testing.T) {
	yaml := `id: CON-BOTH
type: detective
scope: system
checks:
  - name: check
    command:
      run: "echo 1"
    script:
      path: script.sh
      timeout: 30s
    on_fail:
      action: alert
      message: bad
`
	dir := t.TempDir()
	_, err := LoadFile(writeTemp(t, dir, "both.yaml", yaml))
	if err == nil {
		t.Error("expected error for check with both command and script")
	}
}

func TestUnmarshalYAML_MappingDecodeError(t *testing.T) {
	// on_fail is a mapping but action has a sequence value — Decode into string fails.
	yaml := `id: CON-DEC
type: detective
scope: system
checks:
  - name: check
    command:
      run: "echo 1"
    on_fail:
      action:
        - halt_agents
`
	dir := t.TempDir()
	_, err := LoadFile(writeTemp(t, dir, "dec.yaml", yaml))
	if err == nil {
		t.Error("expected error when on_fail.action is a sequence")
	}
}

func TestUnmarshalYAML_SequenceNode(t *testing.T) {
	// A YAML sequence for on_fail is an unexpected node kind — should error.
	yaml := `id: CON-SEQ
type: detective
scope: system
checks:
  - name: check
    command:
      run: "echo 1"
    on_fail:
      - alert
      - halt_agents
`
	dir := t.TempDir()
	_, err := LoadFile(writeTemp(t, dir, "seq.yaml", yaml))
	if err == nil {
		t.Error("expected error for on_fail with sequence YAML node")
	}
}

func TestHasSchedule(t *testing.T) {
	cases := []struct {
		name string
		c    Contract
		want bool
	}{
		{"trigger:schedule", Contract{Trigger: "schedule"}, true},
		{"frequency set", Contract{Frequency: "60s"}, true},
		{"both set", Contract{Trigger: "schedule", Frequency: "60s"}, true},
		{"neither set", Contract{}, false},
		{"other trigger", Contract{Trigger: "event"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.HasSchedule(); got != tc.want {
				t.Errorf("HasSchedule() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsGlobal(t *testing.T) {
	cases := []struct {
		name string
		c    Contract
		want bool
	}{
		{"global scope", Contract{Scope: "global"}, true},
		{"system scope (legacy)", Contract{Scope: "system"}, true},
		{"agent scope", Contract{Scope: "agent:concierge"}, false},
		{"empty scope", Contract{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.IsGlobal(); got != tc.want {
				t.Errorf("IsGlobal() = %v, want %v", got, tc.want)
			}
		})
	}
}
