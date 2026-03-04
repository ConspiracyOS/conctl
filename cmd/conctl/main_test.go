package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ConspiracyOS/conctl/internal/config"
	"github.com/ConspiracyOS/conctl/internal/runner"
)

func TestLoadConfig_FromEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`[[agents]]
name = "test"
tier = "worker"
`), 0644)
	t.Setenv("CONOS_CONFIG", path)

	cfg := loadConfig()
	if len(cfg.Agents) != 1 || cfg.Agents[0].Name != "test" {
		t.Errorf("loadConfig returned unexpected config: %+v", cfg)
	}
}

func TestRunHealthcheck_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONOS_CONTRACTS_DIR", dir)
	runHealthcheck()
}

func TestHealthcheckIn_EmptyDir(t *testing.T) {
	err := healthcheckIn(t.TempDir(), filepath.Join(t.TempDir(), "contracts.log"), "")
	if err != nil {
		t.Errorf("expected nil for empty contracts dir, got: %v", err)
	}
}

func TestHealthcheckIn_BadDir(t *testing.T) {
	err := healthcheckIn("/nonexistent-xyz/contracts", "/dev/null", "")
	if err == nil {
		t.Error("expected error for nonexistent contracts dir")
	}
	if !strings.Contains(err.Error(), "loading contracts") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHealthcheckIn_PassingContract(t *testing.T) {
	contractsDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "contracts.log")

	os.WriteFile(filepath.Join(contractsDir, "test.yaml"), []byte(`
id: TEST-001
description: Always passing
type: detective
trigger: schedule
scope: global
checks:
  - name: always_pass
    command:
      run: "true"
      exit_code: 0
    on_fail: alert
`), 0644)

	err := healthcheckIn(contractsDir, logPath, "")
	if err != nil {
		t.Errorf("expected nil for passing contract, got: %v", err)
	}
	// Log file should have been written
	if _, statErr := os.Stat(logPath); os.IsNotExist(statErr) {
		t.Error("expected log file to be written")
	}
}

func TestHealthcheckIn_FailingContract(t *testing.T) {
	contractsDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "contracts.log")

	os.WriteFile(filepath.Join(contractsDir, "test.yaml"), []byte(`
id: TEST-002
description: Always failing
type: detective
trigger: schedule
scope: global
checks:
  - name: always_fail
    command:
      run: "false"
      exit_code: 0
    on_fail: alert
`), 0644)

	err := healthcheckIn(contractsDir, logPath, "")
	if err == nil {
		t.Fatal("expected error for failing contract")
	}
	if !strings.Contains(err.Error(), "contract(s) failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowLogs_NoLogs(t *testing.T) {
	showLogsFrom(t.TempDir(), &logOpts{n: 20})
}

func TestShowLogsFrom_TodayLog(t *testing.T) {
	auditDir := t.TempDir()
	today := time.Now().Format("2006-01-02")

	// Write a log with more than 20 lines — covers the tail branch
	var lines []string
	for i := 0; i < 25; i++ {
		lines = append(lines, strings.Repeat("x", 10))
	}
	os.WriteFile(filepath.Join(auditDir, today+".log"), []byte(strings.Join(lines, "\n")), 0644)

	showLogsFrom(auditDir, &logOpts{n: 20})
}

func TestShowLogsFrom_ContractsLogFallback(t *testing.T) {
	auditDir := t.TempDir()
	// No today.log, but contracts.log exists — should print it
	os.WriteFile(filepath.Join(auditDir, "contracts.log"), []byte("contract log line\n"), 0644)
	// No panic or error expected
	showLogsFrom(auditDir, &logOpts{n: 20})
}

func TestShowLogsFrom_LastN(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().Format("2006-01-02")
	logFile := filepath.Join(dir, today+".log")
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%02d", i)
	}
	if err := os.WriteFile(logFile, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Redirect stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	showLogsFrom(dir, &logOpts{n: 5, follow: false, agent: ""})

	w.Close()
	os.Stdout = old
	var buf strings.Builder
	io.Copy(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "line-29") {
		t.Errorf("expected last line (line-29), got: %s", out)
	}
	if strings.Contains(out, "line-00") {
		t.Errorf("should not contain early lines (line-00), got: %s", out)
	}
}

func TestDropTaskToAgent(t *testing.T) {
	dir := t.TempDir()
	agentInbox := filepath.Join(dir, "agents", "researcher", "inbox")
	if err := os.MkdirAll(agentInbox, 0755); err != nil {
		t.Fatal(err)
	}

	if err := dropTaskToAgent(filepath.Join(dir, "agents"), "researcher", "do research"); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(agentInbox)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 task file, got %d", len(files))
	}
	data, _ := os.ReadFile(filepath.Join(agentInbox, files[0].Name()))
	if string(data) != "do research" {
		t.Fatalf("unexpected task content: %q", string(data))
	}
}

func TestKillAgent_NoUnits(t *testing.T) {
	// killAgentUnits calls systemctl stop — expected to fail in test env (no systemd)
	// Just verify it doesn't panic.
	err := killAgentUnits("nonexistent-agent")
	_ = err // error expected in test env
}

func TestDropTaskTo_Success(t *testing.T) {
	inbox := t.TempDir()
	if err := dropTaskTo(inbox, "hello world"); err != nil {
		t.Fatalf("dropTaskTo failed: %v", err)
	}
	entries, _ := os.ReadDir(inbox)
	if len(entries) != 1 {
		t.Fatalf("expected 1 task file in inbox, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), ".task") {
		t.Errorf("expected .task file, got %q", entries[0].Name())
	}
	data, _ := os.ReadFile(filepath.Join(inbox, entries[0].Name()))
	if string(data) != "hello world" {
		t.Errorf("unexpected task content: %q", string(data))
	}
}

func TestDropTaskTo_Error(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root")
	}
	inbox := t.TempDir()
	os.Chmod(inbox, 0555)
	defer os.Chmod(inbox, 0755)

	if err := dropTaskTo(inbox, "test"); err == nil {
		t.Error("expected error writing to read-only inbox")
	}
}

func TestShowResponsesIn_Empty(t *testing.T) {
	// No agents dir entries — just returns without output
	if err := showResponsesIn(t.TempDir()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowResponsesIn_BadDir(t *testing.T) {
	err := showResponsesIn("/nonexistent-xyz/agents")
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
	if !strings.Contains(err.Error(), "cannot read agents dir") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowResponsesIn_WithResponse(t *testing.T) {
	agentsDir := t.TempDir()
	outbox := filepath.Join(agentsDir, "concierge", "outbox")
	os.MkdirAll(outbox, 0755)
	os.WriteFile(filepath.Join(outbox, "20260101-120000-task.response"), []byte("response content"), 0644)
	// Plain file in agentsDir covers the !e.IsDir() continue branch
	os.WriteFile(filepath.Join(agentsDir, "notadir.txt"), []byte("ignored"), 0644)

	if err := showResponsesIn(agentsDir); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowResponsesIn_Truncation(t *testing.T) {
	agentsDir := t.TempDir()
	outbox := filepath.Join(agentsDir, "sysadmin", "outbox")
	os.MkdirAll(outbox, 0755)
	// Content > 500 bytes should be truncated
	big := strings.Repeat("a", 600)
	os.WriteFile(filepath.Join(outbox, "20260101-120000-task.response"), []byte(big), 0644)
	if err := showResponsesIn(agentsDir); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowStatusIn(t *testing.T) {
	agentsDir := t.TempDir()
	// Create a fake agent with two tasks pending
	inboxDir := filepath.Join(agentsDir, "concierge", "inbox")
	os.MkdirAll(inboxDir, 0755)
	os.WriteFile(filepath.Join(inboxDir, "001.task"), []byte("task"), 0644)
	os.WriteFile(filepath.Join(inboxDir, "002.task"), []byte("task"), 0644)
	os.WriteFile(filepath.Join(inboxDir, "readme.txt"), []byte("ignored"), 0644)
	// Plain file in agentsDir covers the !e.IsDir() continue branch
	os.WriteFile(filepath.Join(agentsDir, "notadir.txt"), []byte("ignored"), 0644)

	// systemctl will return non-zero (not installed in test env) → state = "inactive"
	// No panic or error expected; output goes to stdout
	if err := showStatusIn(agentsDir); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowResponsesIn_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root")
	}
	agentsDir := t.TempDir()
	outbox := filepath.Join(agentsDir, "test-agent", "outbox")
	os.MkdirAll(outbox, 0755)
	f := filepath.Join(outbox, "20260101-120000.response")
	os.WriteFile(f, []byte("content"), 0000) // mode 0: unreadable
	defer os.Chmod(f, 0644)

	if err := showResponsesIn(agentsDir); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowStatusIn_BadDir(t *testing.T) {
	err := showStatusIn("/nonexistent-xyz/agents")
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
	if !strings.Contains(err.Error(), "cannot read agents dir") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunAgentLoop_DrainInbox(t *testing.T) {
	agentName := "test-agent"
	stateBase := t.TempDir()
	homeBase := t.TempDir()
	agentDir := filepath.Join(stateBase, "agents", agentName)
	os.MkdirAll(filepath.Join(agentDir, "inbox"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "outbox"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "processed"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "workspace"), 0755)
	agentHome := filepath.Join(homeBase, "a-"+agentName)
	os.MkdirAll(agentHome, 0755)
	os.WriteFile(filepath.Join(agentHome, "AGENTS.md"), []byte("# Agent\n"), 0644)

	// Two tasks — both processed by cat, then inbox drained → return nil
	os.WriteFile(filepath.Join(agentDir, "inbox", "001.task"), []byte("first"), 0644)
	os.WriteFile(filepath.Join(agentDir, "inbox", "002.task"), []byte("second"), 0644)

	cfg := &config.Config{Agents: []config.AgentConfig{{Name: agentName, Runner: "cat"}}}
	dirs := runner.Dirs{HomeBase: homeBase, StateBase: stateBase, ConfigBase: t.TempDir()}

	if err := runAgentLoop(agentName, cfg, dirs); err != nil {
		t.Fatalf("runAgentLoop failed: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(agentDir, "outbox"))
	if len(entries) != 2 {
		t.Errorf("expected 2 responses in outbox, got %d", len(entries))
	}
}

func TestRunAgentLoop_RuntimeError(t *testing.T) {
	agentName := "test-agent"
	stateBase := t.TempDir()
	homeBase := t.TempDir()
	agentDir := filepath.Join(stateBase, "agents", agentName)
	os.MkdirAll(filepath.Join(agentDir, "inbox"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "outbox"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "processed"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "workspace"), 0755)
	agentHome := filepath.Join(homeBase, "a-"+agentName)
	os.MkdirAll(agentHome, 0755)
	os.WriteFile(filepath.Join(agentHome, "AGENTS.md"), []byte("# Agent\n"), 0644)
	os.WriteFile(filepath.Join(agentDir, "inbox", "001.task"), []byte("fail"), 0644)

	// "false" exits 1 with no output → runner.Run returns "runtime returned no output"
	cfg := &config.Config{Agents: []config.AgentConfig{{Name: agentName, Runner: "false"}}}
	dirs := runner.Dirs{HomeBase: homeBase, StateBase: stateBase, ConfigBase: t.TempDir()}

	err := runAgentLoop(agentName, cfg, dirs)
	if err == nil {
		t.Fatal("expected error from failing runtime")
	}
}
